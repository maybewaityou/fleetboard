# fleetboard Implementation Plan (P0 + P1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个 TUI 仪表盘，聚合多家 AI Coding 厂商账号的订阅/额度用量（首批 GLM + MiniMax），数据全部来自服务端接口。

**Architecture:** 六边形（ports & adapters），移植自作者自己的 `lazytmux`/`lazyssh`。`UsageProvider` 是唯一对外数据端口；新增厂商 = 加一个 adapter。UI 用 `tview` + Tokyo Night，布局严格复刻 lazytmux（header / [左:search+list | 右:details 3:2] / statusbar）。

**Tech Stack:** Go 1.24 · cobra · tview/tcell · zap · gopkg.in/yaml.v3

**Reference spec:** `docs/superpowers/specs/2026-07-27-fleetboard-design.md`

## Global Constraints

- **module path**: `github.com/maybewaityou/fleetboard`
- **Go 版本**: `go 1.24.6`（go.mod 声明）
- **依赖版本**（与 lazytmux 对齐）: `cobra v1.10.2` · `tview v0.42.0` · `tcell/v2 v2.13.10` · `zap v1.27.0` · `gopkg.in/yaml.v3 v3.0.1`
- **状态目录**: `~/.fleetboard/`（config.yaml / cache.json / fleetboard.log）
- **配置文件权限**: `0600`；写入用临时文件 + rename（原子），保留滚动备份 ≤10 份
- **token 不明文落盘**：config 里只存 `token_env`（环境变量名），运行时 `os.Getenv` 取值
- **日志脱敏**：token / Authorization 永不打屏
- **代码风格**: `gofumpt` + `go vet`；每个 `.go` 文件顶部 Apache 2.0 license header（见 Task 1 模板）
- **提交**: 语义化 `type(scope): 简短描述`；每个 Task 末尾 commit
- **测试**: `go test -race -cover ./...`；TDD（先红后绿）
- **UI 布局比例严格 3:2**；footer 右侧不显示刷新时间；两种刷新 `r`(选中)/`R`(全部)

---

## File Structure

```
fleetboard/
├── cmd/main.go                              # cobra root，装配依赖
├── go.mod / go.sum
├── makefile                                 # 移植 lazytmux（BINARY=fleetboard）
├── .gitignore
├── internal/
│   ├── logger/logger.go                     # zap，env=FLEETBOARD
│   ├── core/
│   │   ├── domain/
│   │   │   ├── account.go                   # Account
│   │   │   ├── vendor_usage.go              # VendorUsage / UsageDimension / ResetPolicy
│   │   │   └── config.go                    # Config / RefreshConfig / UIConfig
│   │   ├── ports/
│   │   │   ├── usage_provider.go            # UsageProvider 接口
│   │   │   ├── config_store.go              # ConfigStore 接口
│   │   │   └── view.go                      # View 接口
│   │   └── services/
│   │       └── aggregator.go                # 并发拉取 + 选主维度
│   └── adapters/
│       ├── config/yaml/store.go             # ~/.fleetboard/config.yaml
│       ├── providers/
│       │   ├── registry.go                  # vendor→adapter 注册表
│       │   ├── mock/mock.go                 # 测试用 stub
│       │   ├── glm/glm.go                   # GLM adapter
│       │   └── minimax/minimax.go           # MiniMax adapter
│       └── ui/
│           ├── theme.go / const.go          # Tokyo Night + vendorColor
│           ├── header.go / search_bar.go
│           ├── account_list.go              # formatAccountLine（平台彩 tag + 主维度% + 状态点）
│           ├── account_details.go           # 全部维度进度条
│           ├── status_bar.go                # r/R 两种刷新，无刷新时间
│           ├── account_form.go / help.go / sort.go
│           └── tui.go                       # buildLayout（3:2）
```

**职责边界**：`domain` 零依赖；`ports` 只依赖 `domain`；`services` 依赖 `ports`+`domain`；`adapters` 实现 `ports`。UI 只消费 `domain.VendorUsage`，不碰任何厂商接口。

---
---

# P0 — 骨架（不依赖外部接口，用 mock provider 跑通 UI）

## Task 1: 项目脚手架

**Files:**
- Create: `go.mod` · `cmd/main.go` · `makefile` · `.gitignore`

**Interfaces:** Produces module `github.com/maybewaityou/fleetboard`、可 `go run`/`make build` 的空壳。

- [ ] **Step 1: 初始化 module + 依赖**

```bash
go mod init github.com/maybewaityou/fleetboard
go get github.com/spf13/cobra@v1.10.2
go get github.com/gdamore/tcell/v2@v2.13.10
go get github.com/rivo/tview@v0.42.0
go get go.uber.org/zap@v1.27.0
go get gopkg.in/yaml.v3@v3.0.1
```

- [ ] **Step 2: 写 `cmd/main.go`（最小可运行空壳）**

每个 `.go` 文件顶部加 Apache 2.0 header（后续任务简称 "license header"）：
```go
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
```

`cmd/main.go`：
```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version   = "develop"
	gitCommit = "unknown"
)

func main() {
	root := &cobra.Command{
		Use:   "fleetboard",
		Short: "AI Coding plan usage dashboard TUI",
		RunE: func(*cobra.Command, []string) error {
			fmt.Printf("fleetboard %s (%s) — scaffold\n", version, gitCommit)
			return nil
		},
	}
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		fmt.Println(err)
	}
}
```

- [ ] **Step 3: 写 `makefile`（移植 lazytmux，改 BINARY）**

```makefile
BINARY   := fleetboard
VERSION  ?= v0.1.0
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  := -X main.version=$(VERSION) -X main.gitCommit=$(COMMIT)

.PHONY: run build test test-verbose lint fmt quality build-all

run:
	go run -ldflags "$(LDFLAGS)" ./cmd

build: quality
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd

test:
	go test -race -cover ./...

test-verbose:
	go test -race -v -cover ./...

fmt:
	gofumpt -w -l . 2>/dev/null || true
	go fmt ./...

lint:
	golangci-lint run ./...

quality: fmt
	go vet ./...

build-all:
	goreleaser build --snapshot --clean
```

- [ ] **Step 4: 写 `.gitignore`**

```
bin/
coverage.out
*.tar.gz
.DS_Store
```

- [ ] **Step 5: 验证可运行**

```bash
make build && ./bin/fleetboard
```
Expected: 输出 `fleetboard v0.1.0 (...) — scaffold`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: scaffold fleetboard module and cobra root"
```

---

## Task 2: logger 包

**Files:**
- Create: `internal/logger/logger.go` · Test: `internal/logger/logger_test.go`
**Interfaces:** Produces `logger.New(env string) (*zap.SugaredLogger, error)`，日志写 `~/.fleetboard/fleetboard.log`。

参照 `../lazytmux/internal/logger/`（移植）。关键：目录不存在则创建；`zap.NewProduction` 配置；返回 `*zap.SugaredLogger`。

- [ ] **Step 1: 写失败测试** — 验证 `New` 返回非 nil logger 且在 `~/.fleetboard/` 创建日志文件。
```go
package logger_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/logger"
)

func TestNewCreatesLogFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	log, err := logger.New("FLEETBOARD")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if log == nil {
		t.Fatal("nil logger")
	}
	log.Info("hello")
	_ = log.Sync()
	if _, err := os.Stat(filepath.Join(t.TempDir(), ".fleetboard", "fleetboard.log")); err == nil {
		// TempDir 不同实例——改为检查真实写入路径见下
	}
}
```
（注：`t.TempDir()` 每次调用返回不同目录；实现里用 `os.UserHomeDir()`，测试用 `t.Setenv("HOME", dir)` 后 `dir` 内应出现 `.fleetboard/fleetboard.log`。修正测试用同一变量。）
修正版关键断言：
```go
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	log, err := logger.New("FLEETBOARD")
	...
	_ = log.Sync()
	want := filepath.Join(dir, ".fleetboard", "fleetboard.log")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
```

- [ ] **Step 2: 运行验证失败** — `go test ./internal/logger/` → FAIL（package not found）

- [ ] **Step 3: 实现 `internal/logger/logger.go`** — 移植 lazytmux，`env` 用于命名（FLEETBOARD），日志路径 `~/.fleetboard/fleetboard.log`，`MkdirAll` 目录。

- [ ] **Step 4: 运行验证通过** — `go test -race ./internal/logger/` → PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(logger): add zap logger writing to ~/.fleetboard"`

---

## Task 3: domain 模型

**Files:**
- Create: `internal/core/domain/account.go` · `vendor_usage.go` · `config.go`
**Interfaces:** Produces `Account` / `VendorUsage` / `UsageDimension` / `ResetPolicy` / `Config`。无外部依赖。

- [ ] **Step 1: 写 `account.go`**
```go
package domain

// Account 是一个被监控的厂商账号配置。
type Account struct {
	ID       string `yaml:"id"`
	Vendor   string `yaml:"vendor"`            // glm | minimax | kimi | ...
	Label    string `yaml:"label"`
	BaseURL  string `yaml:"base_url,omitempty"` // 可选，覆盖默认
	TokenEnv string `yaml:"token_env"`          // 环境变量名，token 从此读
}
```

- [ ] **Step 2: 写 `vendor_usage.go`**（spec §6 多维度模型，逐字落实）
```go
package domain

import "time"

type ResetPolicy string

const (
	ResetRolling5h ResetPolicy = "rolling5h"
	ResetDaily     ResetPolicy = "daily"
	ResetMonthly   ResetPolicy = "monthly"
	ResetCustom    ResetPolicy = "custom"
)

// VendorUsage 是一次拉取的结果。一个账号可有多个额度维度。
type VendorUsage struct {
	AccountID  string
	Vendor     string
	Label      string
	Dimensions []UsageDimension
	Primary    *UsageDimension
	FetchedAt  time.Time
	Err        error
}

// UsageDimension 是单个额度维度（一个窗口/一档配额）。
type UsageDimension struct {
	Name        string
	Used        int64
	Limit       int64
	PercentUsed float64 // -1 = N/A
	Remaining   int64
	ResetsAt    time.Time
	Unit        string
	Source      string
}

// SelectPrimary 把 PercentUsed 最大的有效维度设为 Primary（最值得警惕的一档）。
func (u *VendorUsage) SelectPrimary() {
	var best *UsageDimension
	for i := range u.Dimensions {
		d := &u.Dimensions[i]
		if d.PercentUsed < 0 {
			continue
		}
		if best == nil || d.PercentUsed > best.PercentUsed {
			best = d
		}
	}
	u.Primary = best
}
```

- [ ] **Step 3: 写 `config.go`**
```go
package domain

type Config struct {
	Accounts []Account     `yaml:"accounts"`
	Refresh  RefreshConfig `yaml:"refresh"`
	UI       UIConfig      `yaml:"ui"`
}

type RefreshConfig struct {
	OnStart  bool   `yaml:"on_start"`
	Interval string `yaml:"interval"` // "5m"
}

type UIConfig struct {
	Theme string `yaml:"theme"` // tokyo-night
}
```

- [ ] **Step 4: 写测试 `domain_test.go`** — `SelectPrimary` 在多维度里选出 PercentUsed 最大者；全为 -1 时 Primary 为 nil。
```go
func TestSelectPrimaryPicksMaxPercent(t *testing.T) {
	u := &domain.VendorUsage{Dimensions: []domain.UsageDimension{
		{Name: "5h", PercentUsed: 30},
		{Name: "weekly", PercentUsed: 88},
		{Name: "mcp", PercentUsed: -1},
	}}
	u.SelectPrimary()
	if u.Primary == nil || u.Primary.Name != "weekly" {
		t.Fatalf("want weekly, got %+v", u.Primary)
	}
}
```

- [ ] **Step 5: 验证通过** — `go test -race ./internal/core/domain/` → PASS

- [ ] **Step 6: Commit** — `git commit -m "feat(domain): add account/usage/config models"`

---

## Task 4: ports 接口

**Files:**
- Create: `internal/core/ports/usage_provider.go` · `config_store.go` · `view.go`
**Interfaces:** Produces 三个端口；Consumes `domain`。

- [ ] **Step 1: `usage_provider.go`**
```go
package ports

import (
	"context"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// UsageProvider 是单个厂商的用量查询适配器。
type UsageProvider interface {
	Vendor() string
	FetchUsage(ctx context.Context, acc domain.Account) (domain.VendorUsage, error)
}
```

- [ ] **Step 2: `config_store.go`**
```go
package ports

import "github.com/maybewaityou/fleetboard/internal/core/domain"

type ConfigStore interface {
	Load() (domain.Config, error)
	Save(domain.Config) error
}
```

- [ ] **Step 3: `view.go`**
```go
package ports

import "github.com/maybewaityou/fleetboard/internal/core/domain"

// View 是 TUI 抽象（便于测试 service 时不拉起真实 tview）。
type View interface {
	Run() error
	Render(usages []domain.VendorUsage)
}
```

- [ ] **Step 4: 验证编译** — `go build ./internal/core/ports/` → OK

- [ ] **Step 5: Commit** — `git commit -m "feat(ports): add UsageProvider/ConfigStore/View ports"`

---

## Task 5: config yaml adapter

**Files:**
- Create: `internal/adapters/config/yaml/store.go` · Test: `store_test.go`
**Interfaces:** Consumes `ports.ConfigStore`；Produces `yaml.NewStore(path string)`。

学 lazyssh 的非破坏性写：原子（临时文件 + rename）+ 滚动备份 ≤10。

- [ ] **Step 1: 写失败测试** — `Load` 读不到文件返回零值不报错或明确错误（定：文件不存在返回空 Config + nil，方便首次启动）；`Save` 后 `Load` 回来等值；备份文件 ≤10。
```go
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	s := yaml.NewStore(path)
	orig := domain.Config{Accounts: []domain.Account{{ID: "g", Vendor: "glm", Label: "智谱", TokenEnv: "GLM_API_KEY"}}}
	if err := s.Save(orig); err != nil { t.Fatal(err) }
	got, err := s.Load()
	if err != nil { t.Fatal(err) }
	if len(got.Accounts) != 1 || got.Accounts[0].ID != "g" { t.Fatalf("roundtrip mismatch: %+v", got) }
	// 权限 0600
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 { t.Fatalf("perm = %o, want 0600", fi.Mode().Perm()) }
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	s := yaml.NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	got, err := s.Load()
	if err != nil || len(got.Accounts) != 0 { t.Fatalf("want empty+nil, got %+v %v", got, err) }
}
```

- [ ] **Step 2: 验证失败** — `go test ./internal/adapters/config/yaml/` → FAIL

- [ ] **Step 3: 实现 `store.go`** — yaml.v3 marshal/unmarshal；`Save` 写临时文件 `path+".tmp"`，`os.Chmod(0600)`，`os.Rename`；保留备份（`config-<n>.bak`，截断到 10）。参考 `../lazyssh/internal/adapters/` 的备份逻辑。

- [ ] **Step 4: 验证通过** — `go test -race ./internal/adapters/config/yaml/` → PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(config): yaml store with atomic writes and backups"`

---

## Task 6: theme + const（vendorColor）

**Files:**
- Create: `internal/adapters/ui/theme.go` · `const.go`
**Interfaces:** Produces Tokyo Night 色值常量 + `vendorColor(vendor) (bg, fg string)` + `statusColor(percent) string`。

移植 `../lazytmux/internal/adapters/ui/theme.go` 的 Tokyo Night 调色板（colorCyan/colorPrimary/colorBorder/colorSelected/colorTitle 等），新增 vendor 配色。

- [ ] **Step 1: `const.go`** — 复制 lazytmux 的颜色常量（`colorCyan`、`colorPrimary`、`colorBorder`、`colorSelected`、`colorTitle`、`colorGreen`/`colorYellow`/`colorRed`/`colorGray` 等，逐字对齐 lazytmux 取值）。

- [ ] **Step 2: `theme.go`** — `initializeTheme()` 返回 `*tview.Application` 并应用 Tokyo Night（移植 lazytmux）；`vendorColor` map：
```go
var vendorColor = map[string][2]string{ // {bg, fg}
	"glm":       {"#7C3AED", "#FFFFFF"},
	"minimax":   {"#EF4444", "#FFFFFF"},
	"kimi":      {"#06B6D4", "#001016"},
	"anthropic": {"#D97757", "#FFFFFF"},
	"openai":    {"#10A37F", "#FFFFFF"},
	"cursor":    {"#6366F1", "#FFFFFF"},
	"copilot":   {"#0969DA", "#FFFFFF"},
}
func VendorTag(vendor string) (bg, fg string) {
	if c, ok := vendorColor[vendor]; ok { return c[0], c[1] }
	return "#6B7280", "#FFFFFF" // 未知厂商灰
}
// statusColor: <70 green, 70-90 yellow, >90 red, <0 gray
func StatusColor(percent float64) string { /* ... */ }
```

- [ ] **Step 3: 写测试** — `VendorTag("glm")` 返回紫底；未知厂商返回灰；`StatusColor` 边界 69/70/89/90/-1。
- [ ] **Step 4: 验证通过** — `go test -race ./internal/adapters/ui/`
- [ ] **Step 5: Commit** — `git commit -m "feat(ui): tokyo-night theme + vendor color map"`

---

## Task 7: mock provider + registry

**Files:**
- Create: `internal/adapters/providers/registry.go` · `mock/mock.go` · tests
**Interfaces:** Produces `*Registry`（`Get(vendor) (UsageProvider, bool)`、`Register(p)`）；`mock.New(dimensions, err)`。

- [ ] **Step 1: `registry.go`**
```go
package providers

import "github.com/maybewaityou/fleetboard/internal/core/ports"

type Registry struct { byVendor map[string]ports.UsageProvider }

func NewRegistry(ps ...ports.UsageProvider) *Registry {
	r := &Registry{byVendor: map[string]ports.UsageProvider{}}
	for _, p := range ps { r.Register(p) }
	return r
}
func (r *Registry) Register(p ports.UsageProvider) { r.byVendor[p.Vendor()] = p }
func (r *Registry) Get(vendor string) (ports.UsageProvider, bool) { p, ok := r.byVendor[vendor]; return p, ok }
```

- [ ] **Step 2: `mock/mock.go`**
```go
package mock

type Provider struct {
	VendorName string
	Dims       []domain.UsageDimension
	Err        error
}
func New(vendor string, dims []domain.UsageDimension, err error) *Provider { return &Provider{vendor, dims, err} }
func (p *Provider) Vendor() string { return p.VendorName }
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.VendorUsage, error) {
	u := domain.VendorUsage{AccountID: acc.ID, Vendor: acc.Vendor, Label: acc.Label, Dimensions: p.Dims, FetchedAt: time.Now(), Err: p.Err}
	u.SelectPrimary()
	return u, p.Err
}
```

- [ ] **Step 3: 测试** — registry Register/Get；mock 返回的 VendorUsage.Primary 正确。
- [ ] **Step 4: 验证通过**
- [ ] **Step 5: Commit** — `git commit -m "feat(providers): registry + mock provider"`

---

## Task 8: UI 壳（移植 lazytmux 布局，mock 数据跑通）

**Files:**
- Create: `internal/adapters/ui/{tui,header,search_bar,account_list,account_details,status_bar}.go` · tests
**Interfaces:** Consumes `ports.View`、`domain.VendorUsage`；Produces `ui.NewTUI(...)`。

**移植来源**（逐文件参照 lazytmux 同名文件，把 `Session`→`Account`、`tmux` 语义→`usage` 语义）：
- `tui.go` 的 `buildLayout`：**逐字保留 3:2 比例**（`left,0,3` / `right,0,2`）。
- `header.go` / `search_bar.go` / `status_bar.go` / `help.go` / `sort.go`：直接移植结构，改文案。
- `account_list.go`（← `session_list.go`）、`account_details.go`（← `session_details.go`）：重写内容渲染。

- [ ] **Step 1: `status_bar.go`**（footer，核心要求：两种刷新、无刷新时间）
```go
package ui

import "github.com/rivo/tview"

type StatusBar struct{ *tview.TextView }

func NewStatusBar() *StatusBar {
	sb := &StatusBar{TextView: tview.NewTextView()}
	sb.SetDynamicColors(true).SetTextAlign(tview.AlignCenter).SetBackgroundColor(tcell.ColorDefault)
	sb.SetText(defaultHints())
	return sb
}
func (s *StatusBar) SetStatus(msg string) { s.SetText(msg) }
func (s *StatusBar) ResetHints()          { s.SetText(defaultHints()) }

func defaultHints() string {
	k := colorCyan
	return "[" + k + "]↑↓[-] Navigate  • " +
		"[" + k + "]r[-] Refresh  • " +      // 刷新选中
		"[" + k + "]R[-] Refresh All  • " +   // 刷新全部
		"[" + k + "]a[-] New  • " +
		"[" + k + "]e[-] Edit  • " +
		"[" + k + "]d[-] Delete  • " +
		"[" + k + "]/[-] Search  • " +
		"[" + k + "]s[-] Sort  • " +
		"[" + k + "]?[-] Help  • " +
		"[" + k + "]q[-] Quit"
}
// 注意：footer 不含上次刷新时间（移入 account_details 的"拉取"行）。
```

- [ ] **Step 2: `account_list.go`** — `formatAccountLine(u)` 产出单行 tview 字符串：
```
<label>  [bg:vendorColor] vendor [-]  <percent>%  <状态点>
```
关键渲染（tview `[fg:bg]` 双色语法做平台彩 tag，前景色做状态点）：
```go
func formatAccountLine(u domain.VendorUsage) string {
	bg, _ := VendorTag(u.Vendor)
	pct, dot := "N/A", "○"
	if u.Primary != nil {
		pct = fmt.Sprintf("%d%%", int(u.Primary.PercentUsed))
		dot = "●"
	}
	dotCol := StatusColor(u.PrimaryPercent())
	return fmt.Sprintf("%s  [%s:%s] %s [-]  %s  [%s]%s[-]",
		u.Label, "white", bg, u.Vendor, padRight(pct, 4), dotCol, dot)
}
```
（`PrimaryPercent` 是 `tui` 上的辅助：返回 Primary.PercentUsed 或 -1。）

- [ ] **Step 3: `account_details.go`** — 渲染选中账号**全部维度**，每维度一行进度条 + 已用/上限/剩余/重置，末尾"拉取 12:03 · api-balanced"。进度条用 `strings.Repeat("█", n)` + `Repeat("░", 20-n)` 实现，按 StatusColor 着色。

- [ ] **Step 4: `tui.go`** — `buildLayout` 严格复刻 lazytmux（header(2) / content[FlexColumn left(3)=FlexRow{search(3),list} | right(2)=FlexRow{details}] / statusbar(1)）。`Render(usages)` 缓存并刷新 list/details。`r`→刷新选中、`R`→刷新全部（通过回调调 service，Task 9/12 接线）。

- [ ] **Step 5: 测试 `formatAccountLine_test.go`** — 给定 VendorUsage，断言输出含 label、vendor、百分比、状态点色彩标签；给定 Primary=nil 断言含 "N/A" 和 "○"。

- [ ] **Step 6: 验证通过** — `go test -race ./internal/adapters/ui/`

- [ ] **Step 7: 手动冒烟** — 在 `cmd/main.go` 临时装配 mock provider + mock 数据，`make run` 看到布局正确、两种刷新键可响应。

- [ ] **Step 8: Commit** — `git commit -m "feat(ui): tui shell with 3:2 layout, list/details, dual refresh footer"`

---

## Task 9: 聚合 service

**Files:**
- Create: `internal/core/services/aggregator.go` · Test: `aggregator_test.go`
**Interfaces:** Consumes `*providers.Registry`、`domain.Account`；Produces `FetchAll(ctx, []Account) []VendorUsage`（并发、单点失败不连坐、每结果 SelectPrimary）。

- [ ] **Step 1: 写失败测试** — 3 个账号（2 成功 1 失败），断言返回 3 个 VendorUsage、失败那个 Err 非 nil 且不 panic、成功的有 Primary。
```go
func TestFetchAllIsolatesFailures(t *testing.T) {
	reg := providers.NewRegistry(
		mock.New("glm", []domain.UsageDimension{{Name:"5h",PercentUsed:30}}, nil),
		mock.New("minimax", nil, errors.New("boom")),
	)
	accs := []domain.Account{{ID:"g",Vendor:"glm"},{ID:"m",Vendor:"minimax"}}
	got := services.NewAggregator(reg).FetchAll(context.Background(), accs)
	if len(got) != 2 { t.Fatalf("want 2, got %d", len(got)) }
	// minimax 那条 Err != nil，glm 那条 Primary != nil
}
```

- [ ] **Step 2: 验证失败** — FAIL（package not found）

- [ ] **Step 3: 实现 `aggregator.go`** — 暴露两个方法（Task 12 两种刷新各用其一）：
```go
func (a *Aggregator) FetchOne(ctx context.Context, acc domain.Account) domain.VendorUsage {
	return a.fetchOne(ctx, acc) // r 刷新选中时调用
}
func (a *Aggregator) FetchAll(ctx context.Context, accs []domain.Account) []domain.VendorUsage {
	out := make([]domain.VendorUsage, len(accs))
	var wg sync.WaitGroup
	for i, acc := range accs {
		wg.Add(1)
		go func(i int, acc domain.Account) { defer wg.Done(); out[i] = a.fetchOne(ctx, acc) }(i, acc)
	}
	wg.Wait()
	return out // R 刷新全部时调用
}
// fetchOne：查 registry；无 adapter → Err=ErrUnknownVendor；成功 → SelectPrimary()；FetchedAt=time.Now()
// 单点失败不连坐：错误只进对应 out[i].Err，不 panic、不阻断其他账号。
```

- [ ] **Step 4: 验证通过** — PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(services): concurrent aggregator isolating per-account failures"`

---
---

# P1 — 真实 adapter（GLM + MiniMax）

## Task 10: GLM adapter

**Files:**
- Create: `internal/adapters/providers/glm/glm.go` · Test: `glm_test.go`
**Interfaces:** Consumes `domain.Account`（用 `TokenEnv` 取 key、`BaseURL` 覆盖默认）；Produces `glm.New()`，`Vendor()=="glm"`。

**真实契约**（来自 cc-switch #1588）：
- `GET https://open.bigmodel.cn/api/monitor/usage/quota/limit`（默认；`BaseURL` 可覆盖）
- Header: `Authorization: <API_TOKEN>`（**直接传 key，不加 Bearer**）+ `Content-Type: application/json`
- 响应：
```json
{"code":200,"data":{"level":"pro","limits":[
  {"type":"TIME_LIMIT","percentage":7,"usage":1000,"currentValue":72,"remaining":928,"nextResetTime":"2026-04-01T00:00:00Z"},
  {"type":"TOKENS_LIMIT","percentage":44,"nextResetTime":"..."},
  {"type":"TOKENS_LIMIT","percentage":53,"nextResetTime":"..."}
]}}
```
- 映射：`TOKENS_LIMIT` 按 `nextResetTime` 升序 → 第 1 个="5小时额度"、第 2 个="每周额度"；`TIME_LIMIT`="MCP每月"（Used=currentValue, Limit=usage, Remaining=remaining, Unit="次"）。`percentage` 即 PercentUsed。`level` 存入 Label 后缀（可选）。

- [ ] **Step 1: 写失败测试** — 用 `httptest.Server` 返回上面的固定 JSON，断言解析出 3 个维度、Primary 是 percentage 最大(53)那档、ResetsAt 正确、Authorization 头是裸 key。
```go
func TestFetchGLM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "KEY123" { t.Errorf("auth = %q", r.Header.Get("Authorization")) }
		fmt.Fprint(w, `{"code":200,"data":{"level":"pro","limits":[
			{"type":"TOKENS_LIMIT","percentage":44,"nextResetTime":"2026-04-01T00:00:00Z"},
			{"type":"TOKENS_LIMIT","percentage":53,"nextResetTime":"2026-04-08T00:00:00Z"},
			{"type":"TIME_LIMIT","percentage":7,"usage":1000,"currentValue":72,"remaining":928,"nextResetTime":"2026-05-01T00:00:00Z"}]}}`)
	}))
	t.Setenv("GLM_API_KEY", "KEY123")
	acc := domain.Account{ID:"g",Vendor:"glm",Label:"智谱",TokenEnv:"GLM_API_KEY",BaseURL:srv.URL}
	u, err := glm.New().FetchUsage(context.Background(), acc)
	if err != nil { t.Fatal(err) }
	if len(u.Dimensions) != 3 { t.Fatalf("dims=%d", len(u.Dimensions)) }
	if u.Primary.PercentUsed != 53 { t.Fatalf("primary=%v", u.Primary) }
}
```

- [ ] **Step 2: 验证失败** — FAIL

- [ ] **Step 3: 实现 `glm.go`**
```go
package glm

type Provider struct{ hc *http.Client }
func New() *Provider { return &Provider{hc: &http.Client{Timeout: 10 * time.Second}} }
func (p *Provider) Vendor() string { return "glm" }

type apiResp struct {
	Code int    `json:"code"`
	Data struct {
		Level  string `json:"level"`
		Limits []struct {
			Type          string `json:"type"`
			Percentage    int    `json:"percentage"`
			Usage         int64  `json:"usage"`
			CurrentValue  int64  `json:"currentValue"`
			Remaining     int64  `json:"remaining"`
			NextResetTime string `json:"nextResetTime"`
		} `json:"limits"`
	} `json:"data"`
}

func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.VendorUsage, error) {
	key := os.Getenv(acc.TokenEnv)
	base := acc.BaseURL; if base == "" { base = "https://open.bigmodel.cn" }
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/monitor/usage/quota/limit", nil)
	req.Header.Set("Authorization", key) // 裸 key，无 Bearer
	var r apiResp
	if _, err := p.do(req, &r); err != nil {
		return domain.VendorUsage{AccountID: acc.ID, Vendor: "glm", Label: acc.Label, Err: err}, err
	}
	// 分类 limits：tokens（按 nextResetTime 升序）、time
	// tokens[0]→"5小时额度", tokens[1]→"每周额度"; time→"MCP每月"(Used=currentValue,Limit=usage,Remaining,Unit="次")
	u := domain.VendorUsage{AccountID: acc.ID, Vendor: "glm", Label: acc.Label, FetchedAt: time.Now()}
	// ...遍历 r.Data.Limits 填 u.Dimensions，每个 Percentage→PercentUsed，parse NextResetTime→ResetsAt...
	u.SelectPrimary()
	return u, nil
}
```

- [ ] **Step 4: 验证通过** — `go test -race ./internal/adapters/providers/glm/` → PASS

- [ ] **Step 5: Commit** — `git commit -m "feat(glm): coding plan usage adapter (multi-dimension)"`

---

## Task 11: MiniMax adapter

**Files:**
- Create: `internal/adapters/providers/minimax/minimax.go` · Test: `minimax_test.go`

**真实契约**（来自 openclaw / MiniMax FAQ）：
- `GET https://api.minimaxi.com/v1/token_plan/remains`（`.io` 国际；`BaseURL` 可覆盖）
- Header: `Authorization: Bearer <token_plan_key>`
- 响应：`usage_percent`/`usagePercent` = **剩余**比例（需反转：used=100-remaining%）；`model_remains[]` 含 `start_time`/`end_time` 时间窗口。

- [ ] **Step 1: 写失败测试** — httptest 返回 `{"usagePercent":12,"model_remains":[{"start_time":"...","end_time":"..."}]}`，断言 PercentUsed=88（100-12）、ResetsAt 取自 end_time、Header 是 `Bearer KEY`。
- [ ] **Step 2: 验证失败** — FAIL
- [ ] **Step 3: 实现 `minimax.go`** — 结构同 GLM：定义 `apiResp`、`New()`、`Vendor()=="minimax"`、单维度 `{"Token Plan", PercentUsed:100-usagePercent, ResetsAt:end_time, Unit:"%", Source:"api-balanced"}`。`Bearer` 前缀。
- [ ] **Step 4: 验证通过** — PASS
- [ ] **Step 5: Commit** — `git commit -m "feat(minimax): token plan usage adapter"`

---

## Task 12: 接线（main 装配 + 两种刷新）

**Files:**
- Modify: `cmd/main.go`
**Interfaces:** Consumes 全部前置任务产物。

- [ ] **Step 1: 改 `cmd/main.go`** 装配：
```go
log, _ := logger.New("FLEETBOARD")
home, _ := os.UserHomeDir()
cfgPath := filepath.Join(home, ".fleetboard", "config.yaml")
store := yaml.NewStore(cfgPath)
reg := providers.NewRegistry(glm.New(), minimax.New())
agg := services.NewAggregator(reg)
t := ui.NewTUI(log, store, agg, version, gitCommit) // TUI 持有刷新回调
root := &cobra.Command{Use:"fleetboard", RunE: func(*cobra.Command,[]string) error { return t.Run() }}
```

- [ ] **Step 2: 在 `tui.go` 实现两种刷新** — `r` 调 `agg.FetchOne(ctx, selected)` 更新单条；`R` 调 `agg.FetchAll(ctx, allAccounts)` 全量；刷新后 `Render`。后台 goroutine 按 `Refresh.Interval`（默认 5m）定时 FetchAll。

- [ ] **Step 3: 集成冒烟** — 写 `~/.fleetboard/config.yaml` 填一个真实 GLM 或 MiniMax 账号（token 走 env），`make run`，确认拉到真实百分比、进度条、重置倒计时、详情多维度。

- [ ] **Step 4: 全量测试** — `make test`（-race -cover）全绿。

- [ ] **Step 5: Commit** — `git commit -m "feat: wire adapters, registry, dual refresh (r/R)"`

---

## Self-Review（写计划后自检）

1. **Spec 覆盖**：
   - 定位/范围（纯服务端聚合）→ Task 4/9/10/11 ✅
   - 六边形架构 → File Structure + Task 3/4 ✅
   - 数据模型（多维度）→ Task 3 ✅
   - adapter 接口 + GLM/MiniMax → Task 4/10/11 ✅
   - 配置（token_env/原子写/备份/0600）→ Task 5 ✅
   - UI 3:2 布局 / 列表项(名+彩tag+%+点) / 详情全维度 / footer 两刷新无时间 → Task 8 ✅
   - 并发+单点不连坐 → Task 9 ✅
   - 安全（脱敏/不明文）→ Global Constraints + Task 5 ✅
   - 测试（httptest golden）→ Task 10/11 ✅
   - **未覆盖（留 P2/P3 后续计划）**：Kimi adapter、cache.json+TTL、指数退避重试、Homebrew/goreleaser 发布、README。已在计划标题标注 P0+P1 范围。
2. **占位符扫描**：GLM/MiniMax 给了真实端点+响应+解析骨架；UI footer/list/details 给了核心代码；无 "TBD/TODO/适当处理"。个别移植步骤指向 lazytmux 具体文件（外部参照，非计划内占位）。
3. **类型一致性**：`VendorUsage`/`UsageDimension`/`SelectPrimary`/`PrimaryPercent`/`FetchAll`/`FetchOne`/`NewRegistry`/`VendorTag`/`StatusColor` 在各任务间命名一致。`FetchOne` 在 Task 12 首次出现——需在 Task 9 aggregator 同时定义 `FetchOne`（补：Task 9 Step 3 应含 `FetchOne(ctx, acc) VendorUsage`，与 `FetchAll` 共享单账号拉取内部函数）。
