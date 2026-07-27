# 工程质量加固设计规格

> 状态：已确认 · 日期：2026-07-28 · 作者：MeePwn
> 范围：① 新增 golangci-lint v2 配置；② 新建 PR/push CI workflow；③ 修复现有 13 个 lint 问题；④ 把 `cmd/main.go` 的 `usageCache` 抽到 `internal/app/` 并补单测。
> 参考：兄弟项目 `lazytmux`（同栈，但其 `.golangci.yml` 为 v1 schema，v2 不兼容——见 §1）；`optimizations-design.md` §10「不加 CI lint（可选后续）」本次落地。

## 1. 背景与目标

fleetboard 当前只有 `release.yml`（tag 触发 goreleaser），**没有 PR/push 门禁**，也**没有项目级 lint 配置**——`make lint` 跑的是 golangci-lint 默认配置。`cmd/main.go` 把装配（wiring）、状态管理（`usageCache`）、账户查找 helper 混在一个文件里，导致 cmd 包测试覆盖 **0%**。

本期做三件事，让「本地能过 = CI 能过」并为后续大改打底：

1. 引入项目级 golangci-lint **v2** 配置（克制的 linter 集，非 `enable-all`）。
2. 新增 `ci.yml`：push to master + PR 触发 `make quality` + lint + `make test`。
3. 修复配置启用后暴露的 13 个真实 lint 问题（含 1 个真 bug）。
4. 把 `usageCache` 及 helper 抽到 `internal/app/`，补表驱动 + 并发单测。

### 关键发现（驱动设计）

- **lazytmux 的 `.golangci.yml` 是 v1 schema，golangci-lint v2 直接拒绝加载**（`unsupported version of the configuration`）。v2 把 `linters-settings` 拆成了 `linters.settings` + `formatters.settings`，且 **gofumpt 是 formatter 必须放 `formatters` 段**。所以不能照搬，要按 v2 重写。
- 用目标 linter 集实测，fleetboard 现有代码暴露 **13 个问题**（详见 §3），其中 `cmd/main.go:88` 的 `exitAfterDefer` 是真 bug。
- fleetboard 所有 `.go` 文件版权头齐全（Apache-2.0），格式一致。

### 非目标

- 不加 `goheader` 版权头 linter（文件头已齐全；可作后续防御性加入）。
- 不接 Codecov（覆盖率先本地 `-cover` 看，不引入外部服务）。
- 不提升 `internal/adapters/ui` 的 57.9% 覆盖（tview UI 测试成本高，留后续）。
- 不改 `makefile`（`quality`/`lint`/`test` 三个 target 已够，CI 直接调用）。
- 不加独立的 `fmt-check` target（格式检查由 golangci-lint 的 gofumpt formatter 兜底）。

## 2. 决策摘要（已与用户确认）

| 议题 | 决策 |
|------|------|
| 加固范围 | CI 门禁 + cmd `usageCache` 补测（含抽出） |
| CI 检查内容 | 调 `make quality`（vet）+ golangci-lint + `make test`（-race -cover） |
| CI 触发 | push to master + pull_request |
| `usageCache` 去处 | 新建 `internal/app/`（Cache + FindAccount/RemoveAccounts），cmd 只留装配 |
| 13 个 lint 问题 | **全修** |

## 3. `.golangci.yml`（v2 格式）

新增 `.golangci.yml`（项目根）。**以下配置已在本机 golangci-lint v2.12.2 实测加载通过**：

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
      # 对齐 lazytmux：测试文件豁免 errcheck/gocyclo（fmt.Fprint(w,...) 不检查返回值等）
      - path: _test\.go
        linters: [errcheck, gocyclo]
      # defer x.Close() 的返回值不检查（HTTP body / 文件句柄，工程上普遍接受）
      - linters: [errcheck]
        text: "Close"

formatters:
  default: none
  enable:
    - gofumpt
```

**设计要点**：
- **克制 linter 集**：不 `enable-all`（lazytmux 的 v1 用 enable-all，在 v2 会爆大量噪音）。只启用 6 个高价值 linter。
- **gofumpt 走 formatter 段**：v2 里 gofumpt 是 formatter，放 `formatters.enable`。`golangci-lint run`（不带 `--fix`）会检查格式差异并报错，CI 用它兜底格式门禁（makefile 的 `fmt` target 用 `-w` 会写文件，不适合 CI 检查模式）。
- **exclusions**：test 文件豁免 errcheck/gocyclo；`Close` 类 errcheck 豁免——这两条覆盖了现有所有 errcheck 噪音，零改代码。

## 4. 现有 13 个 lint 问题修复

启用 §3 配置后实测暴露 13 个问题，全修：

| linter | 规则 | 数量 | 位置 | 修法 |
|--------|------|------|------|------|
| gocritic | `exitAfterDefer` | 1 | `cmd/main.go:88` | **真 bug**，见 §4.1 |
| staticcheck | QF1012 | 3 | `account_details.go:160,164,177` | `b.WriteString(fmt.Sprintf(...))` → `fmt.Fprintf(b, ...)` |
| staticcheck | QF1008 | 3 | `account_list.go:48,61,67` | 移除冗余嵌入字段选择器（`x.List.Foo` → `x.Foo`） |
| gocritic | `httpNoBody` | 3 | `deepseek.go:95` `glm.go:120` `kimi.go:107` | `nil` request body → `http.NoBody` |
| gocritic | `ifElseChain` | 1 | `tui.go:362` | if-else 链改写为 `switch` |
| staticcheck | QF1001 | 1 | `account_test.go:26` | 应用德摩根定律 |
| staticcheck | QF1002 | 1 | `aggregator_test.go:219` | if-else 链改 `switch acc.Provider` |

**修复策略（分两步）**：
1. 先跑 `golangci-lint run --fix ./...`——staticcheck 的 `QF*` 系列（QF1012/QF1008/QF1001/QF1002，共 8 个）大多可自动修复。跑完 `git diff` 核对自动改动无误。
2. 手动修 gocritic 的 5 个（`--fix` 不覆盖）：`httpNoBody`×3、`ifElseChain`×1、`exitAfterDefer`×1（§4.1）。

### 4.1 `exitAfterDefer`（真 bug）修复

现状 `cmd/main.go::main()`：开头 `defer func(){ _ = sugar.Sync() }()`，但错误路径（第 88 行 `root.Execute()` 失败）调 `os.Exit(1)`——**`os.Exit` 不执行 defer，出错时 zap 缓冲日志丢失**。在 `os.Exit` 前补 `_ = sugar.Sync()` 既修不了 bug 也消不了 gocritic 警告。

**正确修法**：把退出逻辑抽成返回 exit code 的函数，`main` 仅 `os.Exit(...)`，让 `defer sync` 在 `return` 路径正常执行（Go 处理「defer + 退出」的标准模式）：

```go
func main() {
    os.Exit(realMain())
}

// realMain 承载原 main 的初始化与退出码。defer sugar.Sync() 在 return 路径正常执行。
func realMain() int {
    sugar, err := logger.New("FLEETBOARD")
    if err != nil {
        fmt.Fprintf(os.Stderr, "fleetboard: init logger: %v\n", err)
        return 1
    }
    defer func() { _ = sugar.Sync() }()

    root := &cobra.Command{ /* 不变 */ }
    root.SilenceUsage = true
    if err := root.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        return 1
    }
    return 0
}
```

现有 `run(sugar *zap.SugaredLogger) error`（装配逻辑，被 cobra `RunE` 调用）**保持不变**，仅 `main` 重构为 `realMain`。

## 5. CI workflow（`.github/workflows/ci.yml`）

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

**设计要点**：
- **触发**：push to master + 任何 PR。保持与 release.yml（tag 触发）正交。
- **golangci-lint 用官方 action**：自动安装、固定版本（`v2.12.2`，与本地一致）、读 `.golangci.yml`、PR 内联评论。不手写安装步骤。
- **vet 走 `make quality`**（`go vet ./...`）；**test 走 `make test`**（`go test -race -cover ./...`）——与本地 target 对齐。
- `make quality` 里的 `fmt`（`gofumpt -w`）在 CI 因未装 gofumpt 退化为 `go fmt`，但格式门禁已由上一步 golangci-lint 的 gofumpt formatter 兜底，不漏。

## 6. `usageCache` 抽到 `internal/app/`

### 6.1 新建 `internal/app/cache.go`

把 `cmd/main.go` 的 `usageCache`（`replaceAll`/`snapshot`/`updateOne`/`setPinned`）与 helper（`findAccount`/`removeAccount`）抽出，导出，仅依赖 `domain` 包：

```go
// Package app 存放 fleetboard 的运行时状态（与装配分离），便于独立测试。
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

### 6.2 `cmd/main.go` 改动

- 删除 `usageCache` 类型及其 4 个方法、`findAccount`、`removeAccount` 的定义。
- `cache := &usageCache{}` → `cache := app.NewCache()`。
- 闭包内调用改为导出 API：`cache.ReplaceAll(...)`、`cache.Snapshot()`、`cache.UpdateOne(...)`、`cache.SetPinned(...)`、`app.FindAccount(...)`、`app.RemoveAccounts(...)`。
- 加 `import "github.com/maybewaityou/fleetboard/internal/app"`。
- cmd 只剩装配 + `run()` 闭包编排 + §4.1 的 `realMain` 重构。

### 6.3 新建 `internal/app/cache_test.go`

表驱动 + 并发，覆盖每个导出符号的契约：

- **ReplaceAll + Snapshot 往返**：放入 N 条 → Snapshot 返回相同 N 条（顺序、字段）。
- **Snapshot 独立性**（关键契约）：拿到返回切片后改其元素 / append，不影响下次 Snapshot（证明是 copy）。
- **UpdateOne 替换**：已存在 AccountID → 该条被替换，其余不动。
- **UpdateOne 追加**：不存在的 AccountID → 末尾追加（防御性）。
- **SetPinned 命中**：存在的 id → Pinned 翻转。
- **SetPinned 未命中**：不存在的 id → no-op，数据集不变。
- **FindAccount**：命中返回 (acc, true)；未命中返回 (零值, false)。
- **RemoveAccounts**：命中返回不含 id 的切片（长度 -1）；未命中返回等长切片（原顺序）。
- **并发安全**（`-race`）：启动多个 goroutine 并发 `ReplaceAll` / `Snapshot` / `UpdateOne` / `SetPinned`，断言无 race、不死锁。

## 7. 触点清单

**新增**：
1. `.golangci.yml`（§3）
2. `.github/workflows/ci.yml`（§5）
3. `internal/app/cache.go`（§6.1）
4. `internal/app/cache_test.go`（§6.3）

**改动**：
5. `cmd/main.go` — 删 cache/helper 定义 + 用 `app` 包（§6.2）+ `realMain` 重构修 `exitAfterDefer`（§4.1）
6. `internal/adapters/ui/account_details.go` — QF1012 ×3
7. `internal/adapters/ui/account_list.go` — QF1008 ×3
8. `internal/adapters/ui/tui.go` — ifElseChain ×1
9. `internal/adapters/providers/deepseek/deepseek.go`、`glm/glm.go`、`kimi/kimi.go` — httpNoBody ×3
10. `internal/core/domain/account_test.go` — QF1001
11. `internal/core/services/aggregator_test.go` — QF1002

## 8. 验证

- `golangci-lint run ./...` 全绿（13 → 0）。
- `make quality` 通过（vet）。
- `make test`（`-race -cover`）全绿；`internal/app` 覆盖率高（cmd 原 0% 的状态逻辑转移到此）。
- `git diff` 核对 `golangci-lint --fix` 的自动改动仅限预期范围。
- ci.yml 首次由 PR 触发跑通（实现完成后推送验证）。

## 9. 非目标（重申）

- goheader、Codecov、ui 覆盖率提升、makefile 改动、独立 fmt-check target —— 均不在本期。
