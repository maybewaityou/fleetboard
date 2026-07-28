# Aggregator per-account 兜底超时 设计规格

> 状态：已确认 · 日期：2026-07-28 · 作者：MeePwn
> 范围：给 `services.Aggregator` 加 per-account 拉取兜底超时（默认 15s，可经 `config.yaml` 的 `refresh.timeout` 配置），防止单个 provider 卡死拖垮整体刷新；与各 adapter 既有的 HTTP 超时形成双层防御。
> 参考：`internal/core/services/aggregator.go`（现有失败隔离 + 并发模型）、各 adapter 的 `httpTimeout = 10s`（内层 HTTP 超时）、`cmd/main.go:128` 的 `WithCancel` ctx（当前无刷新超时）。

## 0. 决策摘要（已与用户确认）

| 议题 | 决策 |
|------|------|
| 超时形态 | **可配置兜底超时**：aggregator 加 per-account ctx 超时，默认 15s，`config.yaml` `refresh.timeout` 可调 |
| 注入方式 | **`WithTimeout` builder**（保持 `NewAggregator(lookup)` 单参数不变，现有 8 处测试零改动） |
| 默认时长 | **15s**（> adapter 的 10s HTTP 超时，作外层兜底；满足时序契约） |
| 双层防御 | 保留各 adapter 的 `http.Client.Timeout=10s`（内层精细），aggregator 15s 为外层兜底；**外层必须宽松于内层** |
| 配置字段 | `domain.RefreshConfig` 加 `Timeout string`（`"15s"`），空/非法→默认 15s |
| 超时后行为 | **复用现有失败隔离**：`context.DeadlineExceeded` → adapter 返回 err → `u.Err` → UI 标红，不连坐其他账号 |
| 超时范围 | per-account（每个账号独立 `context.WithTimeout`），FetchAll 各 goroutine 独立超时；unknown-provider 路径不包超时（无网络调用） |
| 导出 | `services.DefaultFetchTimeout`（供 main 装配 fallback 引用） |
| 不在范围 | 失败重试（退避/次数/错误分类）、自动定时刷新（`Interval`/`OnStart` 仍不消费）、adapter httpTimeout 上移统一 |

执行顺序：domain/config 加 `Timeout` 字段 → aggregator 加 `timeout` 字段 + `DefaultFetchTimeout` + `WithTimeout` + `fetchOne` 包超时 → main 装配解析配置 → 测试。每步 `go build ./...` + `go test ./...` 绿。

## 1. 背景与动机：缺第三道防线

`FetchAll`（`aggregator.go:60`）并发拉取多账号用量，用 `sync.WaitGroup` 等所有 goroutine 返回。当前有两道防线：

1. **失败隔离（已实现）**：单账号 err 只进 `out[i].Err`，不 panic、不阻断其他账号。
2. **各 adapter 的 `http.Client.Timeout=10s`（已实现）**：单个 HTTP 请求最多 10s。

但缺第三道：**aggregator 层没有 per-account 超时**。`main.go:128` 的 ctx 是 `WithCancel`（仅退出时 cancel），无刷新超时。`FetchAll` 的 `wg.Wait()` 会等所有 goroutine——若某 adapter 忘了设 HTTP 超时（或未来引入不带超时的 adapter、或有非 HTTP 阻塞），`FetchAll` 会无限等待，整个刷新卡死，用户按 R 无响应。

本特性补这道兜底：aggregator 给每次 `fetchOne` 包 `context.WithTimeout`，给出「单账号最多 N 秒」的硬保证，与 adapter 的 HTTP 超时形成 Go 标准双层防御（内层精细、外层兜底）。

**这是防御性增强，不是修 bug**——当前所有 adapter 都自觉设了 10s HTTP 超时，所以"卡死"尚未发生；本特性把"依赖每个 adapter 自觉"升级为"aggregator 层硬保证"。

## 2. 时序契约（双层防御的关键）

当前 ctx 链：`main.WithCancel` → `adapter.http.Client.Timeout(10s)`。

本特性后：`main.WithCancel` → `aggregator.WithTimeout(15s)` → `adapter.http.Client.Timeout(10s)`。

**关键时序契约**：aggregator 超时（15s）**必须 >** adapter HTTP 超时（10s）。

- 若外层 ≤ 内层：外层先触发，内层 HTTP 超时形同虚设，且错误信息退化为笼统的 `context.DeadlineExceeded`（而非 adapter 自己更精确的 `"HTTP 500"`/`"request error"`）。
- 默认值 15s 满足此契约（15 > 10）。
- 用户通过 `refresh.timeout` 配置时，若设为 ≤10s 会破坏契约——spec 不强制阻止（用户自主），但 README 会注明建议 ≥12s。

## 3. 数据模型扩展（domain/config）

### `RefreshConfig` 加 `Timeout` 字段

```go
// config.go
type RefreshConfig struct {
    OnStart  bool   `yaml:"on_start"`
    Interval string `yaml:"interval"` // "5m"（仍不消费，属另一条 feature 线）
    Timeout  string `yaml:"timeout"`  // "15s"；空/非法→默认 15s（aggregator per-account 兜底超时）
}
```

字符串形式（yaml 友好，与 `Interval` 一致），`main.go` 用 `time.ParseDuration` 解析。零值（未配置）= 用默认 15s，向后兼容（现有 config.yaml 无此字段时行为不变，只是多了兜底超时——这恰恰是要加的保护）。

## 4. Aggregator 设计（services）

### 4.1 加 timeout 字段 + 默认值 + `WithTimeout` builder

```go
// DefaultFetchTimeout 是 per-account 拉取的默认兜底超时（main 装配 fallback 用）。
// 必须 > 各 adapter 的 http.Client.Timeout（10s），见时序契约。
const DefaultFetchTimeout = 15 * time.Second

type Aggregator struct {
    lookup  ports.ProviderLookup
    timeout time.Duration // per-account 兜底超时；0=不限
}

// NewAggregator 构造聚合器，默认 per-account 超时 DefaultFetchTimeout。
// 保持单参数：现有调用方（8 处测试 + main）零改动。
func NewAggregator(lookup ports.ProviderLookup) *Aggregator {
    return &Aggregator{lookup: lookup, timeout: DefaultFetchTimeout}
}

// WithTimeout 设置 per-account 拉取兜底超时（builder，链式）。
// main 从 config.refresh.timeout 解析后调用；0 表示不限超时（仅测试或特殊场景）。
// 应在 FetchAll/FetchOne 调用前设置（构造期一次），之后并发只读 a.timeout。
func (a *Aggregator) WithTimeout(d time.Duration) *Aggregator {
    a.timeout = d
    return a
}
```

`NewAggregator` 保持单参数 → 现有 8 处 `NewAggregator(reg)` 调用零改动，自动获得默认 15s 兜底超时（这对它们是无害的增强：测试用 mock，瞬时返回，15s 永不触发）。

### 4.2 `fetchOne` 包 per-account 超时

```go
func (a *Aggregator) fetchOne(ctx context.Context, acc domain.Account) domain.ProviderUsage {
    p, ok := a.lookup.Get(acc.Provider)
    if !ok {
        // unknown provider 路径完全不变（不涉及网络，无需超时）
        return domain.ProviderUsage{ /* ErrUnknownProvider，同现状 */ }
    }

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

要点：
- **unknown-provider 路径不包超时**（无网络调用，直接返回）。
- `a.timeout == 0` 时不包 WithTimeout（保留外部 ctx 原样），用于"不限超时"场景。
- `defer cancel()` 确保每次 fetchOne 的 ctx 资源释放。

### 4.3 并发安全

`a.timeout` 是构造后只读字段：`WithTimeout` 在 main 装配期调用一次，之后 `FetchAll`/`FetchOne` 并发读 `a.timeout`，无写。`fetchOne` 内 `fetchCtx`/`cancel` 是局部变量，goroutine 间独立。`-race` 安全。

## 5. 装配（cmd/main.go）

```go
agg := services.NewAggregator(reg)

// 解析 per-account 兜底超时；空/非法→默认 DefaultFetchTimeout（15s）。
fetchTimeout := services.DefaultFetchTimeout
if cfg.Refresh.Timeout != "" {
    if d, err := time.ParseDuration(cfg.Refresh.Timeout); err == nil && d > 0 {
        fetchTimeout = d
    }
    // 非法值（如 "abc"）静默回退默认，不阻断启动——刷新超时不是致命配置。
}
agg.WithTimeout(fetchTimeout)
```

`NewAggregator(reg).WithTimeout(...)` 链式也可，但分两行更易读。

**导出决策**：`services.DefaultFetchTimeout`（而非 `defaultFetchTimeout`）导出，供 main 装配 fallback 引用——避免 main 内联 `15 * time.Second` 魔法数与 services 常量漂移。

## 6. 错误处理（完全复用失败隔离）

超时路径：
1. `context.WithTimeout` 到期 → fetchCtx.Done
2. adapter 的 `p.FetchUsage` 内：`http.Client.Do` 返回 `context.DeadlineExceeded`（或 adapter 主动检查 ctx）→ 返回 `err`
3. aggregator：`err != nil && u.Err == nil` → `u.Err = err`
4. `u.Err` 包装 `context.DeadlineExceeded`（`errors.Is(u.Err, context.DeadlineExceeded)` 可判）
5. UI 据现有逻辑对失败账号标红，**其他账号不受影响**

**无新 UI 逻辑、无新 domain 字段**（ProviderUsage.Err 已存在）。超时只是 err 的一种，走既有失败隔离管道。

## 7. 测试策略

### 现有测试（零改动）
`aggregator_test.go` 的 8 个测试用 `NewAggregator(reg)`（默认 15s）；mock 瞬时返回，15s 永不触发，行为完全不变。

### 新增测试（`aggregator_test.go`）

测试内定义 `slowProvider` stub（不污染 `mock.go`）：

```go
type slowProvider struct {
    name string
}
func (s *slowProvider) Provider() string { return s.name }
func (s *slowProvider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
    <-ctx.Done() // 阻塞到 ctx 超时/cancel
    return domain.ProviderUsage{AccountID: acc.ID, Provider: s.name, Label: acc.Label, FetchedAt: time.Now()}, ctx.Err()
}
```

- **TestFetchOneTimeout**：`NewAggregator(reg with slowProvider).WithTimeout(50ms).FetchOne` → 在 ~50ms 内返回；`u.Err != nil`；`errors.Is(u.Err, context.DeadlineExceeded)`。用 `time.Since(start)` 断言耗时在合理区间（如 40ms~250ms，避免 flaky）。
- **TestFetchOneNoTimeout**：`WithTimeout(0)` → slowProvider 收到的 ctx 无 deadline（`ctx.Deadline()` 返回 `ok=false`），不被截断。用 `select` + 短 `time.After` 验证 FetchUsage 未立即返回，再 cancel 外部 ctx 让其退出（避免测试自身卡住）。
- **TestFetchAllTimeoutDoesNotBlockOthers**：`FetchAll([slow, fast], WithTimeout(50ms))` → fast 账号立即返回无 err，slow 账号 ~50ms 返回带 `DeadlineExceeded` err；两者都完成（慢账号超时不阻塞快账号）。
- **TestWithTimeoutBuilder**：`NewAggregator(reg).WithTimeout(7*time.Second)` → `a.timeout == 7s`（白盒断言字段）；未调用 WithTimeout 时 `a.timeout == DefaultFetchTimeout`。

### 测试纪律
- 超时测试用小阈值（50ms）+ 容忍区间，避免 CI 慢机器 flaky。
- `TestFetchOneNoTimeout` 必须用外部 cancel 主动退出，**不能让测试依赖 slowProvider 自然返回**（它会阻塞到 ctx cancel）。

## 8. 文档（README）

`refresh.timeout` 在 README 的配置说明里加一行（如有 refresh 配置段；否则在配置示例注释提及）：
```yaml
refresh:
  timeout: 15s  # 单账号拉取兜底超时；建议 ≥12s（≥ adapter 的 HTTP 超时）；空=默认 15s
```

## 9. 不在范围（YAGNI）

- **失败重试**（退避策略、重试次数、可重试错误分类）——独立特性，需单独设计（瞬时错误识别、指数退避等），本特性只做超时。
- **自动定时刷新**（`Interval`/`OnStart` 仍不消费）——另一条 feature 线（"自动刷新"），与本特性正交。
- **adapter 的 httpTimeout 上移到 aggregator 统一**——改动 6 个 adapter、风险大、收益低（双层防御已足够，adapter 各自的 HTTP 超时是好的实践）。
- **配置非法值的报错/警告**——静默回退默认（刷新超时非致命配置），保持启动健壮。
