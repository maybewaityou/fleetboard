# 工程质量加固 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 引入 golangci-lint v2 配置 + PR/push CI 门禁，修掉现有 13 个 lint 问题（含 1 个真 bug），并把 cmd 里的 usageCache 抽到可测的 internal/app 包。

**Architecture:** 纯增量 + 局部重构。新增 `internal/app/` 承载运行时状态（Cache + 账户 helper），cmd 瘦身为装配；新增 `.golangci.yml`（v2）与 `ci.yml`；lint 修复分 `--fix` 自动 + 手动两类。不改动业务逻辑、UI 行为、provider 契约。

**Tech Stack:** Go 1.24.6、golangci-lint v2.12.2、GitHub Actions（actions/setup-go@v5、golangci-lint-action@v6）、tview/tcell、cobra。

## Global Constraints

- Go 版本 `1.24.6`（`go.mod` 已固定，CI 用 `1.24`）。
- golangci-lint 配置必须是 **v2 schema**：`version: "2"`，`gofumpt` 走 `formatters` 段（非 `linters.enable`）。
- 语义化提交：`type(scope): 描述`（`feat`/`fix`/`refactor`/`test`/`ci`/`chore`/`docs`）。
- 测试一律 `go test -race`（`make test` = `go test -race -cover ./...`）。
- Termux/Android 运行时兼容不受影响：不动 `tz.Init`、路径、时区逻辑。
- 每个 task 结束 `go build ./...` + 相关 `go test` 绿，再提交。

## File Structure

**新增**：
- `internal/app/cache.go` — `Cache`（并发安全用量快照）+ `FindAccount`/`RemoveAccounts`，仅依赖 `domain`。
- `internal/app/cache_test.go` — 表驱动 + `-race` 并发测试。
- `.golangci.yml` — v2 lint 配置（6 linter + gofumpt formatter）。
- `.github/workflows/ci.yml` — PR/push 门禁。

**改动**：
- `cmd/main.go` — 删 `usageCache`/`findAccount`/`removeAccount`，接 `app` 包；`main` 重构为 `realMain`（修 `exitAfterDefer`）。
- `internal/adapters/ui/account_details.go` — QF1012 ×3。
- `internal/adapters/ui/account_list.go` — QF1008 ×3。
- `internal/adapters/ui/tui.go` — ifElseChain ×1。
- `internal/adapters/providers/{deepseek,glm,kimi}/*.go` — httpNoBody ×3。
- `internal/core/domain/account_test.go` — QF1001。
- `internal/core/services/aggregator_test.go` — QF1002。

---

### Task 1: 抽出 `internal/app/` Cache（TDD）

**Files:**
- Create: `internal/app/cache.go`
- Test: `internal/app/cache_test.go`

**Interfaces:**
- Consumes: `github.com/maybewaityou/fleetboard/internal/core/domain`（`ProviderUsage`、`Account`）。
- Produces: `app.NewCache() *Cache`；`(*Cache).ReplaceAll([]domain.ProviderUsage)`、`.Snapshot() []domain.ProviderUsage`、`.UpdateOne(domain.ProviderUsage)`、`.SetPinned(string, bool)`；`app.FindAccount([]domain.Account, string) (domain.Account, bool)`；`app.RemoveAccounts([]domain.Account, string) []domain.Account`。Task 2 依赖这些签名。

- [ ] **Step 1: 写失败测试 `internal/app/cache_test.go`**

```go
package app

import (
	"sync"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func newUsage(id string, pct int, pinned bool) domain.ProviderUsage {
	return domain.ProviderUsage{
		AccountID: id,
		Provider:  "glm",
		Label:     id,
		Pinned:    pinned,
		Dimensions: []domain.UsageDimension{{
			Name:        "5h",
			PercentUsed: float64(pct),
		}},
	}
}

func TestCache_ReplaceAllAndSnapshot(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false), newUsage("b", 20, false)})
	got := c.Snapshot()
	if len(got) != 2 || got[0].AccountID != "a" || got[1].AccountID != "b" {
		t.Fatalf("Snapshot = %+v, want [a,b]", got)
	}
}

func TestCache_SnapshotIndependence(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false)})
	got := c.Snapshot()
	got[0].AccountID = "mutated"
	got = append(got, newUsage("x", 99, false))
	again := c.Snapshot()
	if again[0].AccountID != "a" || len(again) != 1 {
		t.Fatalf("Snapshot not independent: %+v", again)
	}
}

func TestCache_UpdateOne(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false), newUsage("b", 20, false)})
	c.UpdateOne(newUsage("a", 90, false)) // 替换已存在
	if got := c.Snapshot(); got[0].Dimensions[0].PercentUsed != 90 || len(got) != 2 {
		t.Fatalf("UpdateOne replace = %+v", got)
	}
	c.UpdateOne(newUsage("c", 30, false)) // 追加新条目
	if got := c.Snapshot(); len(got) != 3 || got[2].AccountID != "c" {
		t.Fatalf("UpdateOne append = %+v", got)
	}
}

func TestCache_SetPinned(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false)})
	c.SetPinned("a", true)
	if got := c.Snapshot(); !got[0].Pinned {
		t.Fatalf("SetPinned hit: want Pinned=true, got %+v", got[0])
	}
	c.SetPinned("missing", true) // 未命中 no-op
	if got := c.Snapshot(); len(got) != 1 {
		t.Fatalf("SetPinned miss mutated cache: %+v", got)
	}
}

func TestFindAccount(t *testing.T) {
	accs := []domain.Account{{ID: "a", Provider: "glm"}, {ID: "b", Provider: "kimi"}}
	if acc, ok := FindAccount(accs, "b"); !ok || acc.ID != "b" {
		t.Fatalf("FindAccount hit = %+v ok=%v", acc, ok)
	}
	if _, ok := FindAccount(accs, "missing"); ok {
		t.Fatal("FindAccount miss: want ok=false")
	}
}

func TestRemoveAccounts(t *testing.T) {
	accs := []domain.Account{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if got := RemoveAccounts(accs, "b"); len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("RemoveAccounts hit = %+v", got)
	}
	if got := RemoveAccounts(accs, "missing"); len(got) != 3 {
		t.Fatalf("RemoveAccounts miss = %+v", got)
	}
}

func TestCache_Concurrent(t *testing.T) {
	c := NewCache()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.ReplaceAll([]domain.ProviderUsage{newUsage("a", n, false)})
			_ = c.Snapshot()
			c.UpdateOne(newUsage("a", n, false))
			c.SetPinned("a", n%2 == 0)
		}(i)
	}
	wg.Wait()
	_ = c.Snapshot() // -race 下无竞争即通过
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/app/...`
Expected: FAIL / 编译失败（`NewCache` undefined，`cache.go` 还没建）。

- [ ] **Step 3: 写实现 `internal/app/cache.go`**

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

// Package app 存放 fleetboard 的运行时状态，与 cmd 装配分离，便于独立测试。
package app

import (
	"sync"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// Cache 是最近一次拉取的 per-account 用量快照，并发安全。
// r/R 回调与 CRUD 回调都写穿它，TUI 从 Snapshot 取只读副本渲染。
type Cache struct {
	mu      sync.Mutex
	current []domain.ProviderUsage
}

// NewCache 构造一个空 Cache。
func NewCache() *Cache { return &Cache{} }

// ReplaceAll 替换整个数据集（R / boot / CRUD 后用）。
func (c *Cache) ReplaceAll(usages []domain.ProviderUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = usages
}

// Snapshot 返回当前数据集的浅拷贝；调用方独占返回切片，
// 后台 tick 不会改动 TUI 正在绘制的快照。
func (c *Cache) Snapshot() []domain.ProviderUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]domain.ProviderUsage, len(c.current))
	copy(out, c.current)
	return out
}

// UpdateOne 按 AccountID 替换单条（r 刷新选中用）；不存在则防御性追加。
func (c *Cache) UpdateOne(u domain.ProviderUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.current {
		if c.current[i].AccountID == u.AccountID {
			c.current[i] = u
			return
		}
	}
	c.current = append(c.current, u)
}

// SetPinned 翻转 id 的 Pinned，不重新拉取（仅元数据变更）。
func (c *Cache) SetPinned(id string, pinned bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.current {
		if c.current[i].AccountID == id {
			c.current[i].Pinned = pinned
			return
		}
	}
}

// FindAccount 按 id 在 accs 中查账号配置（FetchOne 需要 provider/token_env/base_url）。
func FindAccount(accs []domain.Account, id string) (domain.Account, bool) {
	for _, a := range accs {
		if a.ID == id {
			return a, true
		}
	}
	return domain.Account{}, false
}

// RemoveAccounts 返回不含 id 的新切片（不改原切片），供删除账号使用。
func RemoveAccounts(accs []domain.Account, id string) []domain.Account {
	out := make([]domain.Account, 0, len(accs))
	for _, a := range accs {
		if a.ID != id {
			out = append(out, a)
		}
	}
	return out
}
```

- [ ] **Step 4: 跑测试确认通过（含 -race）**

Run: `go test -race ./internal/app/...`
Expected: PASS（7 个测试全过，含并发用例无 race）。

- [ ] **Step 5: 全量构建确认无副作用**

Run: `go build ./...`
Expected: 成功（app 包独立，cmd 尚未引用，不影响现有编译）。

- [ ] **Step 6: 提交**

```bash
git add internal/app/cache.go internal/app/cache_test.go
git commit -m "refactor(app): extract usageCache into testable internal/app package"
```

---

### Task 2: `cmd/main.go` 接线 `app` 包 + `realMain` 重构

**Files:**
- Modify: `cmd/main.go`

**Interfaces:**
- Consumes: Task 1 的 `app.NewCache`、`app.FindAccount`、`app.RemoveAccounts`、`(*Cache).ReplaceAll/Snapshot/UpdateOne/SetPinned`。
- Produces: 不再有 `usageCache`/`findAccount`/`removeAccount` 包内符号；`exitAfterDefer` 消除。

- [ ] **Step 1: import 增补 `app` 包**

在 `cmd/main.go` 的 import 块中按字母序插入（`adapters/ui` 与 `core/domain` 之间；gofumpt 会校验顺序）：

```go
	"github.com/maybewaityou/fleetboard/internal/adapters/ui"
	"github.com/maybewaityou/fleetboard/internal/app"
	"github.com/maybewaityou/fleetboard/internal/core/domain"
```

> 包路径为 `internal/app`（Task 1 创建于 `internal/app/cache.go`，`package app`）。

- [ ] **Step 2: 替换 cache 构造与所有方法调用**

将 `cache := &usageCache{}` 改为：

```go
	cache := app.NewCache()
```

机械替换 `run` 内闭包里的方法名（小写→导出）：

| 旧 | 新 |
|----|----|
| `cache.replaceAll(usages)` | `cache.ReplaceAll(usages)` |
| `cache.snapshot()` | `cache.Snapshot()` |
| `cache.updateOne(agg.FetchOne(ctx, acc))` | `cache.UpdateOne(agg.FetchOne(ctx, acc))` |
| `cache.setPinned(id, pinned)` | `cache.SetPinned(id, pinned)` |
| `findAccount(cfg.Accounts, accountID)` | `app.FindAccount(cfg.Accounts, accountID)` |
| `removeAccount(cfg.Accounts, id)` | `app.RemoveAccounts(cfg.Accounts, id)` |

> `cache.snapshot()` 出现在 4 处闭包返回语句；`findAccount` 出现在 `refreshSelected` 与 `onLoadAccount`。逐个替换。

- [ ] **Step 3: 删除旧定义**

删除 `cmd/main.go` 末尾的 `usageCache` 类型、`replaceAll`/`snapshot`/`updateOne`/`setPinned` 四个方法、`findAccount`、`removeAccount`（原文件第 231–311 行整段），以及结尾那段已注释的「background auto-refresh removed」注释（第 313–314 行，随 `usageCache` 一起清掉，不再相关）。

- [ ] **Step 4: `main` 重构为 `realMain`（修 `exitAfterDefer`）**

将整个 `func main() { ... }`（原第 61–90 行）替换为：

```go
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
```

> 保留 `tz.Init` 注释与逻辑不变；仅 `os.Exit(1)` → `return 1`，并新增 `func main() { os.Exit(realMain()) }`。`run(sugar)` 函数（装配）保持不变。

- [ ] **Step 5: 构建 + 全量测试**

Run: `go build ./... && go test -race ./...`
Expected: 成功；现有测试全绿（cache 逻辑转移到 app，行为不变）。

- [ ] **Step 6: 提交**

```bash
git add cmd/main.go
git commit -m "refactor(cmd): wire app.Cache, split realMain to fix exitAfterDefer"
```

---

### Task 3: `.golangci.yml` + 修复剩余 lint 问题

**Files:**
- Create: `.golangci.yml`
- Modify: `internal/adapters/ui/account_details.go`、`account_list.go`、`tui.go`、`providers/{deepseek,glm,kimi}/*.go`、`core/domain/account_test.go`、`core/services/aggregator_test.go`

**Interfaces:**
- Consumes: 无（配置 + 机械修复）。
- Produces: `make lint` / `golangci-lint run ./...` 零问题。

> 顺序前提：Task 2 已修 `exitAfterDefer`，故本任务启用配置后 lint 只会报其余 12 个。

- [ ] **Step 1: 写 `.golangci.yml`（v2，已实测加载通过）**

```yaml
version: "2"

run:
  timeout: 5m

linters:
  default: none
  enable:
    - govet
    - errcheck
    - staticcheck
    - ineffassign
    - unused
    - gocritic
  settings:
    gocritic:
      enabled-tags: [diagnostic, style]
  exclusions:
    paths:
      - "bin"
    rules:
      - path: _test\.go
        linters: [errcheck, gocyclo]
      - linters: [errcheck]
        text: "Close"

formatters:
  default: none
  enable:
    - gofumpt
```

- [ ] **Step 2: 用 `--fix` 自动修 staticcheck QF（8 个）**

Run: `golangci-lint run --fix ./...`
Expected: 自动修复 QF1012（`account_details.go` ×3：`WriteString(fmt.Sprintf(...))` → `fmt.Fprintf(&b, ...)`）、QF1008（`account_list.go` ×3：移除冗余 `.List` 选择器）、QF1001（`account_test.go`）、QF1002（`aggregator_test.go`）。

Run: `git diff --stat`
Expected: 仅上述文件被改。**人工核对 `git diff`** 确认自动改动语义正确（`fmt.Fprintf(&b, ...)` 第一参为 `*strings.Builder`，实现 `io.Writer`）。

- [ ] **Step 3: 手动修 gocritic `httpNoBody`（3 处）**

`internal/adapters/providers/glm/glm.go:120`、`deepseek/deepseek.go:95`、`kimi/kimi.go:107`，把 `http.NewRequestWithContext` 的 body 实参从 `nil` 改为 `http.NoBody`：

```go
// 旧
req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+usagePath, nil)
// 新
req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+usagePath, http.NoBody)
```

> 三处同一模式。`newapi`/`sub2api` 不在 lint 报告内（构造方式不同），不动。

- [ ] **Step 4: 手动修 gocritic `ifElseChain`（`tui.go` applyCacheToViews）**

将 `internal/adapters/ui/tui.go` 第 360–368 行的 if-else 链改为 `switch`：

```go
	// 旧
	if u, ok := t.accountList.GetSelected(); ok {
		t.details.Render(u)
	} else if len(visible) > 0 {
		t.details.Render(visible[0])
	} else if len(t.allCache) == 0 {
		t.details.RenderEmpty("no accounts configured")
	} else {
		t.details.RenderEmpty("no matching accounts")
	}

	// 新
	u, ok := t.accountList.GetSelected()
	switch {
	case ok:
		t.details.Render(u)
	case len(visible) > 0:
		t.details.Render(visible[0])
	case len(t.allCache) == 0:
		t.details.RenderEmpty("no accounts configured")
	default:
		t.details.RenderEmpty("no matching accounts")
	}
```

- [ ] **Step 5: 跑 lint 确认零问题**

Run: `golangci-lint run ./...`
Expected: EXIT 0，无输出（13 个全清，含 Task 1/2 新增改动的代码）。

- [ ] **Step 6: 全量测试 + quality**

Run: `make quality && make test`
Expected: 全绿（vet + fmt + race + cover）。

- [ ] **Step 7: 提交**

```bash
git add .golangci.yml internal/adapters/ui/account_details.go internal/adapters/ui/account_list.go internal/adapters/ui/tui.go internal/adapters/providers/glm/glm.go internal/adapters/providers/deepseek/deepseek.go internal/adapters/providers/kimi/kimi.go internal/core/domain/account_test.go internal/core/services/aggregator_test.go
git commit -m "ci: add golangci-lint v2 config and fix all 13 lint findings"
```

---

### Task 4: PR/push CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: Task 3 的 `.golangci.yml`、makefile 的 `quality`/`test` target。
- Produces: push master + PR 触发的门禁 job。

- [ ] **Step 1: 写 `.github/workflows/ci.yml`**

```yaml
name: ci

on:
  push:
    branches: [master]
  pull_request:

permissions:
  contents: read

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true

      - name: vet (make quality)
        run: make quality

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v2.12.2

      - name: test (race + cover)
        run: make test
```

- [ ] **Step 2: 校验 YAML 语法**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('yaml ok')"`
Expected: `yaml ok`（无语法错误）。

- [ ] **Step 3: 本地复跑 CI 等价命令，确认能过**

Run: `make quality && golangci-lint run ./... && make test`
Expected: 全绿（与 CI 三个 step 等价，本地能过 = CI 能过）。

- [ ] **Step 4: 提交**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add push/PR workflow (quality + golangci-lint + test)"
```

> 首次真实触发由推送 master 或开 PR 验证（CI 在 GitHub 侧跑）。

---

## Self-Review 记录

- **Spec 覆盖**：spec §3（.golangci.yml）→ Task 3 Step 1；§4（13 问题）→ Task 2（exitAfterDefer）+ Task 3（其余 12）；§5（ci.yml）→ Task 4；§6（internal/app）→ Task 1+2。全覆盖。
- **占位符扫描**：无 TBD/TODO；所有代码块完整；手动修均给了 old/new diff。
- **类型一致**：Task 1 定义的 `app.NewCache/ReplaceAll/Snapshot/UpdateOne/SetPinned/FindAccount/RemoveAccounts` 与 Task 2 引用、cache_test.go 调用一致。
