# Aggregator per-account 兜底超时 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 `services.Aggregator` 加 per-account 拉取兜底超时（默认 15s，可经 `config.yaml` 的 `refresh.timeout` 配置），与各 adapter 的 10s HTTP 超时形成双层防御，防止单个 provider 卡死拖垮整体刷新。

**Architecture:** 沿用六边形。`Aggregator` 加 `timeout` 字段 + `WithTimeout` builder（不改 `NewAggregator` 单参数签名，现有 8 处测试零改动）；`fetchOne` 给真实 `FetchUsage` 包 `context.WithTimeout`，unknown-provider 路径不包。`domain.RefreshConfig` 加 `Timeout string`，`main.go` 解析后经 `WithTimeout` 注入。超时复用现有失败隔离（`DeadlineExceeded` → `u.Err` → UI 标红），无新 UI/domain 字段。

**Tech Stack:** Go 1.24.6 · 标准库 `context`+`time`+`strconv`（无新依赖）· `gopkg.in/yaml.v3`（仅 Task 1 测试）· 表驱动 + 现有 mock/stub 测试 · `gofumpt`/`go vet`/`golangci-lint`。

## Global Constraints

- Go 1.24.6（toolchain 1.26.5 兼容）；零新增第三方依赖。
- per-account ctx 超时；`DefaultFetchTimeout = 15 * time.Second`（**导出**，供 main fallback）。
- `WithTimeout` builder 注入；**不改 `NewAggregator(lookup)` 签名**（保持现有 8 处调用零改动）。
- **时序契约**：aggregator 超时（默认 15s）必须 > adapter 的 `http.Client.Timeout`（10s）——外层宽松于内层，否则内层 HTTP 超时失效、错误信息退化。
- unknown-provider 路径**不包超时**（无网络调用，直接返回）。
- `a.timeout == 0` 表示不限超时（不包 WithTimeout，保留外部 ctx 原样）。
- 超时复用现有失败隔离：无新 UI 逻辑、无新 domain 字段（除 config 的 Timeout 字符串字段）。
- `RefreshConfig.Timeout` 为 string（"15s"），`time.ParseDuration` 解析；空/非法→默认 15s（静默回退，非致命）。
- 每任务 TDD（先红后绿），结尾 `go build ./...` + `go test ./...` 绿后 commit；conventional commits，结尾 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 不在范围：失败重试、自动定时刷新（Interval/OnStart 仍不消费）、adapter httpTimeout 上移。

---

## File Structure

| 文件 | 动作 | 职责 |
|------|------|------|
| `internal/core/domain/config.go` | Modify | `RefreshConfig` 加 `Timeout string` 字段 |
| `internal/core/domain/domain_test.go` | Modify | `TestRefreshConfigTimeoutYAML`（yaml 解析 + 零值） |
| `internal/core/services/aggregator.go` | Modify | `DefaultFetchTimeout` 常量 + `timeout` 字段 + `WithTimeout` + `fetchOne` 包超时 |
| `internal/core/services/aggregator_test.go` | Modify | `slowProvider` stub + 4 个超时测试 |
| `cmd/main.go` | Modify | 解析 `refresh.timeout` + `agg.WithTimeout(...)`（+ `time` import 若缺） |
| `README.md` / `README.zh-CN.md` | Modify | `refresh.timeout` 配置说明 |

---

## Task 1: domain — `RefreshConfig` 加 `Timeout` 字段

**Files:**
- Modify: `internal/core/domain/config.go`（`RefreshConfig` struct）
- Test: `internal/core/domain/domain_test.go`（`package domain`，import 需加 `gopkg.in/yaml.v3`）

**Interfaces:**
- Produces: `domain.RefreshConfig.Timeout string`（yaml `timeout`）—— Task 3 main 消费

- [ ] **Step 1: 写失败测试**

在 `domain_test.go` 末尾追加。同时把文件顶部 import 从 `import "testing"` 改为：

```go
import (
	"testing"

	"gopkg.in/yaml.v3"
)
```

追加测试：

```go
// TestRefreshConfigTimeoutYAML 验证 RefreshConfig.Timeout 的 yaml 解析与零值默认。
func TestRefreshConfigTimeoutYAML(t *testing.T) {
	var cfg struct {
		Refresh RefreshConfig `yaml:"refresh"`
	}
	if err := yaml.Unmarshal([]byte("refresh:\n  timeout: 15s\n"), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Refresh.Timeout != "15s" {
		t.Errorf("Refresh.Timeout = %q, want 15s", cfg.Refresh.Timeout)
	}
	// 零值：未配置时为空字符串（main 据此回退默认 15s）。
	var zero RefreshConfig
	if zero.Timeout != "" {
		t.Errorf("zero-value Timeout should be empty, got %q", zero.Timeout)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/core/domain/ -run TestRefreshConfigTimeoutYAML -v`
Expected: FAIL（`cfg.Refresh.Timeout` 字段不存在 / 编译错 `unknown field`）

- [ ] **Step 3: 加字段**

在 `config.go` 的 `RefreshConfig` struct 加 `Timeout` 字段（`Interval` 之后）：

```go
// RefreshConfig 控制定时刷新行为。
type RefreshConfig struct {
	OnStart  bool   `yaml:"on_start"`
	Interval string `yaml:"interval"` // "5m"（仍不消费，属另一条 feature 线）
	Timeout  string `yaml:"timeout"`  // "15s"；空/非法→默认 15s（aggregator per-account 兜底超时）
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/core/domain/ -run TestRefreshConfigTimeoutYAML -v`
Expected: PASS

- [ ] **Step 5: 全量构建 + 测试 + gofmt**

Run: `gofmt -w internal/core/domain/config.go internal/core/domain/domain_test.go && go build ./... && go test ./...`
Expected: 全绿

- [ ] **Step 6: Commit**

```bash
git add internal/core/domain/config.go internal/core/domain/domain_test.go
git commit -m "feat(domain): add RefreshConfig.Timeout field

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: aggregator — 兜底超时核心（字段 + builder + fetchOne + 测试）

**Files:**
- Modify: `internal/core/services/aggregator.go`
- Test: `internal/core/services/aggregator_test.go`（`package services`）

**Interfaces:**
- Consumes: `domain.Account`、`ports.UsageProvider`、`ports.ProviderLookup`
- Produces: `services.DefaultFetchTimeout`（导出常量，Task 3 main 引用）、`(*Aggregator).WithTimeout(time.Duration) *Aggregator`（builder）

- [ ] **Step 1: 写失败测试（builder + 超时）**

在 `aggregator_test.go` 末尾追加。`slowProvider` 是测试专用 stub（不污染 `mock.go`），实现 `ports.UsageProvider`，阻塞到 ctx.Done：

```go
// slowProvider 是超时测试专用 stub：FetchUsage 阻塞到 ctx 被取消/超时，
// 然后返回 ctx.Err。不放入 mock.go（mock 不模拟阻塞）。
type slowProvider struct {
	name string
}

func (s *slowProvider) Provider() string { return s.name }

func (s *slowProvider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	<-ctx.Done()
	return domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  s.name,
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}, ctx.Err()
}

// TestWithTimeoutBuilder 验证 WithTimeout 设置字段；未调用时为 DefaultFetchTimeout。
func TestWithTimeoutBuilder(t *testing.T) {
	reg := providers.NewRegistry(mock.New("glm", nil, nil))
	def := NewAggregator(reg)
	if def.timeout != DefaultFetchTimeout {
		t.Fatalf("default timeout = %v, want %v", def.timeout, DefaultFetchTimeout)
	}
	custom := NewAggregator(reg).WithTimeout(7 * time.Second)
	if custom.timeout != 7*time.Second {
		t.Fatalf("WithTimeout(7s) = %v, want 7s", custom.timeout)
	}
	zero := NewAggregator(reg).WithTimeout(0)
	if zero.timeout != 0 {
		t.Fatalf("WithTimeout(0) = %v, want 0 (unlimited)", zero.timeout)
	}
}

// TestFetchOneTimeout 验证 per-account 超时截断慢 provider 并回填 DeadlineExceeded。
func TestFetchOneTimeout(t *testing.T) {
	reg := providers.NewRegistry(&slowProvider{name: "slow"})
	agg := NewAggregator(reg).WithTimeout(50 * time.Millisecond)
	acc := domain.Account{ID: "s1", Provider: "slow", Label: "slow"}

	start := time.Now()
	u := agg.FetchOne(context.Background(), acc)
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, want ~50ms (tolerance 40ms~500ms)", elapsed)
	}
	if u.Err == nil {
		t.Fatal("expected timeout err, got nil")
	}
	if !errors.Is(u.Err, context.DeadlineExceeded) {
		t.Fatalf("u.Err = %v, want wraps context.DeadlineExceeded", u.Err)
	}
	// 账号元信息仍回填（UI 标红但展示账号）。
	if u.AccountID != "s1" || u.Provider != "slow" {
		t.Errorf("meta not backfilled: %+v", u)
	}
}

// TestFetchOneNoTimeout 验证 WithTimeout(0) 不限超时：slowProvider 收到的 ctx 无 deadline。
// 用外部 cancel 主动退出，避免测试自身卡住。
func TestFetchOneNoTimeout(t *testing.T) {
	reg := providers.NewRegistry(&slowProvider{name: "slow"})
	agg := NewAggregator(reg).WithTimeout(0)
	acc := domain.Account{ID: "s2", Provider: "slow", Label: "slow"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan domain.ProviderUsage, 1)
	go func() { done <- agg.FetchOne(ctx, acc) }()

	// 给一点时间确认 FetchUsage 确实进入阻塞（未被立即超时截断）。
	select {
	case u := <-done:
		t.Fatalf("FetchOne returned early without external cancel: %+v", u)
	case <-time.After(30 * time.Millisecond):
		// 预期：还在阻塞 → 说明无 deadline 截断。
	}

	cancel() // 主动退出
	select {
	case u := <-done:
		if u.Err == nil {
			t.Error("expected err after cancel, got nil")
		}
	case <-time.After(time.Second):
		t.Fatal("FetchOne did not return within 1s after cancel")
	}
}

// TestFetchAllTimeoutDoesNotBlockOthers 验证慢账号超时不阻塞快账号。
func TestFetchAllTimeoutDoesNotBlockOthers(t *testing.T) {
	reg := providers.NewRegistry(
		&slowProvider{name: "slow"},
		mock.New("fast", []domain.UsageDimension{{Name: "5h", PercentUsed: 30}}, nil),
	)
	agg := NewAggregator(reg).WithTimeout(50 * time.Millisecond)
	accs := []domain.Account{
		{ID: "slow-1", Provider: "slow", Label: "Slow"},
		{ID: "fast-1", Provider: "fast", Label: "Fast"},
	}

	start := time.Now()
	got := agg.FetchAll(context.Background(), accs)
	elapsed := time.Since(start)

	// FetchAll 总耗时 ≈ 最慢账号（slow ~50ms），不是它们的和。
	if elapsed > 500*time.Millisecond {
		t.Fatalf("FetchAll elapsed = %v, want < 500ms (slow must not serialize behind... itself)", elapsed)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// slow 超时带 err；fast 立即返回无 err。
	if got[0].Err == nil || !errors.Is(got[0].Err, context.DeadlineExceeded) {
		t.Errorf("slow result err = %v, want DeadlineExceeded", got[0].Err)
	}
	if got[1].Err != nil {
		t.Errorf("fast result should be clean, got %v", got[1].Err)
	}
	if got[1].Primary == nil || got[1].Primary.Name != "5h" {
		t.Errorf("fast Primary missing: %+v", got[1].Primary)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/core/services/ -run 'TestWithTimeoutBuilder|TestFetchOneTimeout|TestFetchOneNoTimeout|TestFetchAllTimeoutDoesNotBlockOthers' -v`
Expected: FAIL（`DefaultFetchTimeout` / `a.timeout` / `WithTimeout` undefined；`slowProvider` 的 FetchUsage 不被截断）

- [ ] **Step 3: 实现 aggregator 改动**

3a. 在 `aggregator.go` 顶部常量区（`ErrUnknownProvider` 附近）加导出常量：

```go
// DefaultFetchTimeout 是 per-account 拉取的默认兜底超时（main 装配 fallback 用）。
// 必须 > 各 adapter 的 http.Client.Timeout（10s）——见 spec 时序契约：外层宽松于内层，
// 否则内层 HTTP 超时失效、错误信息退化为笼统的 context.DeadlineExceeded。
const DefaultFetchTimeout = 15 * time.Second
```

3b. `Aggregator` struct 加 `timeout` 字段：

```go
type Aggregator struct {
	lookup  ports.ProviderLookup
	timeout time.Duration // per-account 兜底超时；0=不限。构造期经 WithTimeout 设置，之后并发只读。
}
```

3c. `NewAggregator` 设默认超时（保持单参数）：

```go
func NewAggregator(lookup ports.ProviderLookup) *Aggregator {
	return &Aggregator{lookup: lookup, timeout: DefaultFetchTimeout}
}
```

3d. 加 `WithTimeout` builder 方法（放在 `NewAggregator` 之后）：

```go
// WithTimeout 设置 per-account 拉取兜底超时（builder，链式）。
// main 从 config.refresh.timeout 解析后调用；0 表示不限超时。
// 应在 FetchAll/FetchOne 调用前设置（构造期一次），之后并发只读 a.timeout。
func (a *Aggregator) WithTimeout(d time.Duration) *Aggregator {
	a.timeout = d
	return a
}
```

3e. 改 `fetchOne`，在真实 FetchUsage 前包 per-account 超时（unknown 路径不变）。把现有：

```go
func (a *Aggregator) fetchOne(ctx context.Context, acc domain.Account) domain.ProviderUsage {
	p, ok := a.lookup.Get(acc.Provider)
	if !ok {
		return domain.ProviderUsage{
			AccountID: acc.ID,
			Provider:  acc.Provider,
			Label:     acc.Label,
			FetchedAt: time.Now(),
			Pinned:    acc.Pinned,
			Err:       fmt.Errorf("%w: %q", ErrUnknownProvider, acc.Provider),
		}
	}

	u, err := p.FetchUsage(ctx, acc)
	if err != nil && u.Err == nil {
		u.Err = err
	}
	u.Pinned = acc.Pinned
	return u
}
```

改为（仅中间段包超时）：

```go
func (a *Aggregator) fetchOne(ctx context.Context, acc domain.Account) domain.ProviderUsage {
	p, ok := a.lookup.Get(acc.Provider)
	if !ok {
		return domain.ProviderUsage{
			AccountID: acc.ID,
			Provider:  acc.Provider,
			Label:     acc.Label,
			FetchedAt: time.Now(),
			Pinned:    acc.Pinned,
			Err:       fmt.Errorf("%w: %q", ErrUnknownProvider, acc.Provider),
		}
	}

	// per-account 兜底超时：给每次 FetchUsage 包独立 deadline，防止单 adapter
	// 卡死（忘了设 HTTP 超时、或非 HTTP 阻塞）拖垮整个 FetchAll。a.timeout==0 不限。
	// 时序契约：a.timeout 必须 > adapter 的 http.Client.Timeout，故默认 15s > 10s。
	fetchCtx := ctx
	if a.timeout > 0 {
		var cancel context.CancelFunc
		fetchCtx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}

	u, err := p.FetchUsage(fetchCtx, acc)
	if err != nil && u.Err == nil {
		u.Err = err
	}
	u.Pinned = acc.Pinned
	return u
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/core/services/ -run 'TestWithTimeoutBuilder|TestFetchOneTimeout|TestFetchOneNoTimeout|TestFetchAllTimeoutDoesNotBlockOthers' -v`
Expected: 4 个新测试 PASS

- [ ] **Step 5: 确认现有测试不破 + 全量 + race + gofmt**

Run: `gofmt -w internal/core/services/aggregator.go internal/core/services/aggregator_test.go && go test -race ./...`
Expected: 全绿（现有 8 个 aggregator 测试 + 4 个新测试 + 其余包）

- [ ] **Step 6: Commit**

```bash
git add internal/core/services/aggregator.go internal/core/services/aggregator_test.go
git commit -m "feat(aggregator): add per-account fetch timeout with WithTimeout builder

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: 装配 — main 解析 `refresh.timeout` 并注入

**Files:**
- Modify: `cmd/main.go`（import 块 + `NewAggregator` 调用处 `:126`）

**Interfaces:**
- Consumes: `services.NewAggregator`、`services.DefaultFetchTimeout`、`(*Aggregator).WithTimeout`、`domain.Config.Refresh.Timeout`（Task 1）
- Produces: 运行时 aggregator 带 per-account 超时

- [ ] **Step 1: 加 `time` import**

`cmd/main.go` 当前未 import `time`（已确认仅注释提及）。在 import 块标准库分组（字母序：`context` < `fmt` < `os` < `path/filepath` < `time`），即 `"path/filepath"` 之后加：

```go
	"time"
```

- [ ] **Step 2: 改装配——`NewAggregator` 后解析配置并注入超时**

把 `cmd/main.go:126` 附近的：

```go
	agg := services.NewAggregator(reg)
```

改为：

```go
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
```

- [ ] **Step 3: 构建 + 全量测试 + gofmt**

Run: `gofmt -w cmd/main.go && go build ./... && go test ./...`
Expected: 构建成功 + 全绿（main 无 test files，靠 build + 其余包绿验证）

- [ ] **Step 4: Commit**

```bash
git add cmd/main.go
git commit -m "feat: wire refresh.timeout into aggregator

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: README 文档 + 全量验证收尾

**Files:**
- Modify: `README.md` / `README.zh-CN.md`

**Interfaces:**
- Consumes: 全部前序任务

- [ ] **Step 1: 更新 `README.md`**

在配置示例段（`refresh:` 或 `~/.fleetboard/config.yaml` 示例附近）加 `refresh.timeout` 说明。若现有示例无 `refresh:` 块，在配置 yaml 示例末尾加注释段：

```yaml
refresh:
  timeout: 15s  # per-account fetch timeout; recommend ≥12s (≥ adapter HTTP timeout); empty = default 15s
```

并在「How it works」或 Features 段补一句：单个 provider 拉取有 15s 兜底超时（可配），某家卡住不会拖垮整体刷新。

- [ ] **Step 2: 更新 `README.zh-CN.md`**

中文化对应改动：

```yaml
refresh:
  timeout: 15s  # 单账号拉取兜底超时；建议 ≥12s（≥ adapter 的 HTTP 超时）；空=默认 15s
```

并补一句中文说明。

- [ ] **Step 3: 格式化 + lint + 全量测试（含 race）**

Run: `make fmt && make lint && make test`
Expected: gofumpt 无变更、golangci-lint 0 问题、`go test -race -cover ./...` 全绿

> 若 `golangci-lint` 未安装导致 `make lint` 失败：改跑 `go vet ./...` 替代并说明。

- [ ] **Step 4: Commit**

```bash
git add README.md README.zh-CN.md
git commit -m "docs: document refresh.timeout in README

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

- [ ] **Step 5: 最终全量验证**

Run: `make quality && make test`
Expected: 全绿，`git status` 工作树 clean
