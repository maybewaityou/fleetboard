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
// glm/minimax providers, services.Aggregator) into the tview TUI and exposes
// manual refresh keys — r re-fetches the selected account (FetchOne) and R
// re-fetches every account (FetchAll). (Background auto-refresh was removed by
// request — refresh manually with r/R.) Refresh callbacks hand the fresh
// dataset to TUI.Render, which marshals the repaint onto the tview main loop so
// the write stays race-free.
//
// Tokens never enter main: each provider reads its own token from the env var
// named by the account's TokenEnv field. Errors from FetchAll/FetchOne are
// passed through untouched in VendorUsage.Err — the UI marks the row red but
// still renders whatever dimensions the provider returned.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/maybewaityou/fleetboard/internal/adapters/config/yaml"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/glm"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/minimax"
	"github.com/maybewaityou/fleetboard/internal/adapters/ui"
	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/services"
	"github.com/maybewaityou/fleetboard/internal/logger"
)

var (
	version   = "develop"
	gitCommit = "unknown"
)

func main() {
	sugar, err := logger.New("FLEETBOARD")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fleetboard: init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = sugar.Sync() }()

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
		os.Exit(1)
	}
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
	reg := providers.NewRegistry(glm.New(), minimax.New())
	agg := services.NewAggregator(reg)

	ctx, cancel := context.WithCancel(context.Background())

	// usageCache holds the last full per-account dataset. Both refresh callbacks
	// and the background refresher write through it so r (refresh-selected) can
	// fold a single FetchOne result back into the rest and return the whole
	// dataset the TUI re-renders against (Render replaces allCache wholesale, so
	// a selected-refresh must hand back the full set, not just the one account).
	// The mutex serializes replace/snapshot/updateOne so a user-driven R and a
	// background tick cannot tear the slice header; returned slices are copies so
	// the TUI owns its snapshot.
	cache := &usageCache{}

	// Initial data: fetch every configured account so the first frame already
	// shows live usage. Per-account errors land in VendorUsage.Err and are passed
	// through untouched (task-7 err-transparency contract).
	initial := agg.FetchAll(ctx, cfg.Accounts)
	cache.replaceAll(initial)

	refreshAll := func() []domain.VendorUsage {
		usages := agg.FetchAll(ctx, cfg.Accounts)
		cache.replaceAll(usages)
		return cache.snapshot()
	}
	refreshSelected := func(accountID string) []domain.VendorUsage {
		acc, ok := findAccount(cfg.Accounts, accountID)
		if !ok {
			// Selection points at an account no longer in config (or is empty on
			// first run before the user has moved the cursor). Return nil so the
			// TUI leaves the view untouched instead of collapsing to empty.
			return nil
		}
		cache.updateOne(agg.FetchOne(ctx, acc))
		return cache.snapshot()
	}

	// CRUD 回调（a/e/d）：mutate cfg.Accounts → store.Save → refreshAll。
	// cfg 是闭包按引用捕获的局部变量，append/remove/edit 后 refreshAll 下次读到新 Accounts。
	onSaveAccount := func(acc domain.Account) []domain.VendorUsage {
		cfg.Accounts = append(cfg.Accounts, acc)
		if err := store.Save(cfg); err != nil {
			sugar.Warnw("save config (add) failed", "error", err)
		}
		return refreshAll()
	}
	onDeleteAccount := func(id string) []domain.VendorUsage {
		cfg.Accounts = removeAccount(cfg.Accounts, id)
		if err := store.Save(cfg); err != nil {
			sugar.Warnw("save config (delete) failed", "error", err)
		}
		return refreshAll()
	}
	onEditAccount := func(id string, acc domain.Account) []domain.VendorUsage {
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
		return findAccount(cfg.Accounts, id)
	}
	onTogglePin := func(id string) []domain.VendorUsage {
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
		cache.setPinned(id, pinned)
		return cache.snapshot()
	}

	t := ui.NewTUI(ui.Config{
		Logger:          sugar,
		Version:         version,
		Commit:          gitCommit,
		InitialData:     cache.snapshot(),
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

// usageCache is the in-process snapshot of the latest per-account usage. It is
// shared between the initial fetch, the r/R callbacks, and the background
// refresher. The mutex makes every accessor safe to call from any goroutine.
type usageCache struct {
	mu      sync.Mutex
	current []domain.VendorUsage
}

// replaceAll swaps the cached dataset. Callers must not retain aliases into the
// slice they hand over (snapshot returns a copy for that).
func (c *usageCache) replaceAll(usages []domain.VendorUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = usages
}

// snapshot returns a shallow copy of the current dataset. Callers own the
// returned slice — mutating it does not affect the cache, which matters because
// the TUI's Render hands it to queueDraw and a later tick must not mutate what
// the main loop is still painting.
func (c *usageCache) snapshot() []domain.VendorUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]domain.VendorUsage, len(c.current))
	copy(out, c.current)
	return out
}

// updateOne replaces the cache entry whose AccountID matches u.AccountID, or
// appends u when no such entry exists. Used by refresh-selected to fold a single
// FetchOne result back into the full dataset without disturbing the others.
func (c *usageCache) updateOne(u domain.VendorUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.current {
		if c.current[i].AccountID == u.AccountID {
			c.current[i] = u
			return
		}
	}
	// Defensive append: selection should always point at an existing row, but if
	// it does not we keep the cache complete rather than silently dropping the
	// freshly-fetched account.
	c.current = append(c.current, u)
}

// setPinned flips the Pinned flag on the cached entry for id, without refetching.
// Used by the pin-toggle callback so toggling pin does not trigger a network
// refresh — only the metadata changes, and the UI re-renders from the snapshot.
func (c *usageCache) setPinned(id string, pinned bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.current {
		if c.current[i].AccountID == id {
			c.current[i].Pinned = pinned
			return
		}
	}
}

// findAccount resolves an AccountID back to its full config (FetchOne needs the
// vendor/token_env/base_url fields, not just the id).
func findAccount(accs []domain.Account, id string) (domain.Account, bool) {
	for _, a := range accs {
		if a.ID == id {
			return a, true
		}
	}
	return domain.Account{}, false
}

// removeAccount 返回不含 id 的新切片（不改原切片），供删除账号使用。
func removeAccount(accs []domain.Account, id string) []domain.Account {
	out := make([]domain.Account, 0, len(accs))
	for _, a := range accs {
		if a.ID != id {
			out = append(out, a)
		}
	}
	return out
}

// (background auto-refresh + parseRefreshInterval + startBackgroundRefresher
// removed by request — users refresh manually with r/R.)
