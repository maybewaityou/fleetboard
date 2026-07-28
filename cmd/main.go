// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command fleetboard is the AI coding-plan usage dashboard TUI.
//
// This is the assembly: it wires the concrete adapters (yaml config store,
// glm/minimax/kimi/deepseek providers, services.Aggregator) into the tview TUI and exposes
// manual refresh keys — r re-fetches the selected account (FetchOne) and R
// re-fetches every account (FetchAll). (Background auto-refresh was removed by
// request — refresh manually with r/R.) Refresh callbacks hand the fresh
// dataset to TUI.Render, which marshals the repaint onto the tview main loop so
// the write stays race-free.
//
// Tokens never enter main: each provider reads its own token from the env var
// named by the account's TokenEnv field. Errors from FetchAll/FetchOne are
// passed through untouched in ProviderUsage.Err — the UI marks the row red but
// still renders whatever dimensions the provider returned.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/maybewaityou/fleetboard/internal/adapters/config/yaml"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/deepseek"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/glm"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/kimi"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/minimax"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/newapi"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/siliconflow"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/sub2api"
	"github.com/maybewaityou/fleetboard/internal/adapters/ui"
	"github.com/maybewaityou/fleetboard/internal/app"
	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/services"
	"github.com/maybewaityou/fleetboard/internal/logger"
	"github.com/maybewaityou/fleetboard/internal/tz"
)

var (
	version   = "develop"
	gitCommit = "unknown"
)

func main() {
	os.Exit(realMain())
}

// realMain 承载初始化与退出码。defer sugar.Sync() 在 return 路径正常执行；
// os.Exit 会跳过 defer，故退出码逻辑从 main 移到这里的 return。
func realMain() int {
	sugar, err := logger.New("FLEETBOARD")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fleetboard: init logger: %v\n", err)
		return 1
	}
	defer func() { _ = sugar.Sync() }()

	// Resolve the real local timezone before anything formats a time. Without
	// this, Termux/Android silently falls back to UTC (its zoneinfo lives under a
	// non-standard $PREFIX path Go's LoadLocation never searches), so every
	// .Local() timestamp in Details renders 8h off on a CST device. tz.Init embeds
	// the tzdb and rebinds time.Local from $TZ / Android getprop.
	if name := tz.Init(); name != "" {
		sugar.Infow("timezone resolved", "zone", name)
	}

	root := &cobra.Command{
		Use:   "fleetboard",
		Short: "AI coding plan usage dashboard TUI",
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(sugar)
		},
	}
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// run wires the assembly: load config, build registry/aggregator, fetch initial
// usage, construct the TUI with refresh + CRUD callbacks, then block on the TUI
// main loop. ctx backs the r/R + CRUD callbacks and is cancelled on exit.
func run(sugar *zap.SugaredLogger) error {
	// Config path: ~/.fleetboard/config.yaml. A missing file is first-run; the
	// yaml store returns an empty Config rather than erroring, so the UI shows
	// its "no accounts configured" empty state instead of crashing.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	cfgPath := filepath.Join(home, ".fleetboard", "config.yaml")
	store := yaml.NewStore(cfgPath)
	cfg, err := store.Load()
	if err != nil {
		// Degrade rather than abort: a malformed YAML should not prevent the user
		// from seeing the dashboard at all. They get the empty state and can fix
		// the file out-of-band.
		sugar.Warnw("load config failed; using empty config", "path", cfgPath, "error", err)
		cfg = domain.Config{}
	}
	sugar.Infow("config loaded", "path", cfgPath, "accounts", len(cfg.Accounts))

	// Hexagonal wiring: the registry holds the concrete adapters; the aggregator
	// depends only on ports.ProviderLookup, which *providers.Registry satisfies.
	reg := providers.NewRegistry(glm.New(), minimax.New(), kimi.New(), deepseek.New(), sub2api.New(), newapi.New(), siliconflow.New())
	agg := services.NewAggregator(reg)
	// per-account 兜底超时：从 config.refresh.timeout 解析；空/非法→默认 15s。
	// 时序契约：必须 > adapter 的 http.Client.Timeout(10s)。刷新超时非致命，非法值静默回退默认。
	fetchTimeout := services.DefaultFetchTimeout
	if cfg.Refresh.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Refresh.Timeout); err == nil && d > 0 {
			fetchTimeout = d
		}
	}
	agg.WithTimeout(fetchTimeout)

	ctx, cancel := context.WithCancel(context.Background())

	// app.Cache holds the last full per-account dataset. Both refresh callbacks
	// and the CRUD callbacks write through it so r (refresh-selected) can fold a
	// single FetchOne result back into the rest and return the whole dataset the
	// TUI re-renders against (Render replaces allCache wholesale, so a
	// selected-refresh must hand back the full set, not just the one account).
	// The cache serializes ReplaceAll/Snapshot/UpdateOne internally; returned
	// slices are copies so the TUI owns its snapshot.
	cache := app.NewCache()

	// Initial data is fetched asynchronously by the TUI's loading screen
	// (Config.LoadInitial below) — the same fetch+cache+snapshot logic as R, so
	// we reuse refreshAll as the boot callback. Until it returns, the user sees a
	// loading splash instead of a frozen terminal. Per-account errors land in
	// ProviderUsage.Err and are passed through untouched (task-7 err-transparency
	// contract).
	refreshAll := func() []domain.ProviderUsage {
		usages := agg.FetchAll(ctx, cfg.Accounts)
		cache.ReplaceAll(usages)
		return cache.Snapshot()
	}
	refreshSelected := func(accountID string) []domain.ProviderUsage {
		acc, ok := app.FindAccount(cfg.Accounts, accountID)
		if !ok {
			// Selection points at an account no longer in config (or is empty on
			// first run before the user has moved the cursor). Return nil so the
			// TUI leaves the view untouched instead of collapsing to empty.
			return nil
		}
		cache.UpdateOne(agg.FetchOne(ctx, acc))
		return cache.Snapshot()
	}

	// CRUD 回调（a/e/d）：mutate cfg.Accounts → store.Save → refreshAll。
	// cfg 是闭包按引用捕获的局部变量，append/remove/edit 后 refreshAll 下次读到新 Accounts。
	onSaveAccount := func(acc domain.Account) []domain.ProviderUsage {
		acc.ID = domain.GenerateAccountID() // ID 自动生成，不由用户/表单提供
		cfg.Accounts = append(cfg.Accounts, acc)
		if err := store.Save(cfg); err != nil {
			sugar.Warnw("save config (add) failed", "error", err)
		}
		return refreshAll()
	}
	onDeleteAccount := func(id string) []domain.ProviderUsage {
		cfg.Accounts = app.RemoveAccounts(cfg.Accounts, id)
		if err := store.Save(cfg); err != nil {
			sugar.Warnw("save config (delete) failed", "error", err)
		}
		return refreshAll()
	}
	onEditAccount := func(id string, acc domain.Account) []domain.ProviderUsage {
		for i := range cfg.Accounts {
			if cfg.Accounts[i].ID == id {
				acc.ID = id // 保留原 ID（form 提交的 ID 可能被改动）
				cfg.Accounts[i] = acc
				break
			}
		}
		if err := store.Save(cfg); err != nil {
			sugar.Warnw("save config (edit) failed", "error", err)
		}
		return refreshAll()
	}
	onLoadAccount := func(id string) (domain.Account, bool) {
		return app.FindAccount(cfg.Accounts, id)
	}
	onTogglePin := func(id string) []domain.ProviderUsage {
		pinned := false
		for i := range cfg.Accounts {
			if cfg.Accounts[i].ID == id {
				cfg.Accounts[i].Pinned = !cfg.Accounts[i].Pinned
				pinned = cfg.Accounts[i].Pinned
				break
			}
		}
		if err := store.Save(cfg); err != nil {
			sugar.Warnw("save config (pin) failed", "error", err)
		}
		// 不重新拉取：pin 只改元数据，就地把缓存里对应条目的 Pinned 同步翻转即可。
		cache.SetPinned(id, pinned)
		return cache.Snapshot()
	}

	t := ui.NewTUI(ui.Config{
		Logger:          sugar,
		Version:         version,
		Commit:          gitCommit,
		UIConfig:        cfg.UI,     // 透传颜色阈值等 UI 配置
		LoadInitial:     refreshAll, // boot — async first fetch via the loading screen
		RefreshSelected: refreshSelected,
		RefreshAll:      refreshAll,
		OnSaveAccount:   onSaveAccount,
		OnDeleteAccount: onDeleteAccount,
		OnEditAccount:   onEditAccount,
		OnLoadAccount:   onLoadAccount,
		OnTogglePin:     onTogglePin,
	})

	// ctx backs the r/R + CRUD callbacks (FetchAll/FetchOne) for the lifetime of
	// the TUI. Background auto-refresh was removed by request — refresh manually
	// with r/R — so cancel only on exit.
	defer cancel()

	if err := t.Run(); err != nil {
		return fmt.Errorf("tui run: %w", err)
	}
	return nil
}
