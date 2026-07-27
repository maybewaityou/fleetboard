# new-api 原生余额与消耗接入 设计规格

> 状态：已确认 · 日期：2026-07-28 · 作者：MeePwn
> 范围：将 new-api provider 的取数通道从「伪装的 OpenAI billing 端点」切换到「new-api 原生管理层」，获取**真实**账户余额与近 7/30 天消耗摘要。
> 参考：`2026-07-27-relay-platforms-and-color-thresholds-design.md`（本期**反转**其 §1/§6 关于 new-api 接口选择的决策）、`deepseek` provider 模板。

## 0. 决策摘要（已与用户确认）

| 议题 | 决策 |
|------|------|
| 接入范围 | A 账户真实余额（`/api/user/self`）+ B 时间维度消耗（`/api/log/self/stat` 标量摘要）；**不**含消费明细、令牌维度 |
| 取数通道 | **废弃** `/v1/dashboard/billing/*`（假数据），改走 `/api/*` 原生层（单通道，不保留 billing 回退） |
| 凭证配置 | `access_token` 走环境变量（新增 `AccessTokenEnv`），`user_id` 写 yaml（新增 `UserID`） |
| 鉴权 | `Authorization: Bearer <access_token>` + `New-Api-User: <user_id>` 双 header |
| B 展示形态 | 详情页新增 `Usage (recent)` 键值对区块（7d/30d 消耗 + 实时 rpm/tpm），不做 sparkline |
| 货币换算 | `美元 = quota / quota_per_unit`，`quota_per_unit` 动态取自 `/api/status`，失败回退默认 500000 |
| 架构 | 方案 A：扩展现有 `newapi` 适配器，不新增 provider |
| 破坏性 | new-api 账号配置从 `token_env` 改为 `access_token_env`+`user_id`；bump **v0.2.0** |

执行顺序：domain 扩展 → 适配器重写（含 4 端点 + 换算）→ UI 区块 → 表单字段 → 测试（先红后绿）→ README/迁移说明。每步 `go build ./...` + `make test` 绿。

## 1. 背景与动机：反转 2026-07-27 的决策

2026-07-27 spec（§1 第 29 行、§6 第 174 行）**刻意选择**了 `/v1/dashboard/billing/*` 而非 `/api/user/self`，理由是「billing 仅需 sk-key 单凭证，对监控用户最友好；`/api/user/self` 需 `New-Api-User` 双凭证，迫使 Account 加字段」。

**该决策的前提是错的。** 2026-07-28 对真实实例 `https://kuaipao.pro`（user_id=16002）实测证实：billing 端点返回的是**伪装数据**，不可用于监控：

| 字段 | billing 端点（伪装） | 原生 `/api/user/self`（真实） |
|------|---------------------|------------------------------|
| 总额/上限 | `system_hard_limit_usd = 100000000`（1 亿占位，表"不限额"） | — |
| 已用 | `total_usage = 5601.46`（语义不明） | `used_quota = 69281250` → **138.56 USD** |
| 剩余 | ≈ 1 亿（无意义） | `quota = 121992688` → **243.99 USD** |

**根因**：new-api / one-api 这类中转站用自有 quota 体系计费，billing 端点仅为骗过「检查余额是否充足」的 OpenAI 兼容客户端而伪装成 1 亿上限。真实账目只存在于原生管理层 `/api/*`，而该层需要 access token（非 sk-key）+ user_id 双凭证。

**结论**：宁可让 Account 加两个字段，也要切到原生层——否则 new-api 的余额监控结构性失真。这是对旧决策的有据反转，不是旧设计的错误（当时未实测无法知晓）。

## 2. 接口调研结论（实测，kuaipao.pro / user_id=16002）

所有 `/api/*` 端点鉴权统一为 `Authorization: Bearer <access_token>` + `New-Api-User: <user_id>`。

| 端点 | 用途 | 关键响应字段 | 实测值 |
|------|------|-------------|--------|
| `GET /api/user/self` | 账户余额（核心） | `data.quota`、`data.used_quota`、`data.request_count` | quota=121992688, used_quota=69281250 |
| `GET /api/status` | 换算因子 | `data.quota_per_unit`、`data.usd_exchange_rate` | quota_per_unit=500000 |
| `GET /api/log/self/stat?start_timestamp=<ts>&end_timestamp=<ts>` | 区间消耗 + 实时速率 | `data.quota`、`data.rpm`、`data.tpm` | 近30天 quota=69281250, rpm/tpm=0 |

换算：`USD = quota / quota_per_unit`（500000）。验证：121992688/500000 = **243.99**，69281250/500000 = **138.56**。

字段单位说明：
- `quota` / `used_quota` / stat 的 `quota` 均为 new-api 内部单位，统一除以 `quota_per_unit` 得美元。
- `rpm`/`tpm` 为实时速率（每分钟请求数/token 数），无流量时为 0；监控场景作辅助信息展示。

> 待实现时校准项：`/api/status` 的 `price=0.4` 语义（疑似充值定价）不参与余额换算，本期忽略；若发现 `quota_per_unit` 在该实例非 500000，以 `/api/status` 实际返回为准（已动态获取，无需硬编码）。

## 3. 数据模型扩展（domain）

### 3.1 `account.go` — 两个新字段

```go
type Account struct {
    // ...现有字段不变...
    TokenEnv string `yaml:"token_env"` // 仍供其他 provider 使用；newapi 不再读它

    // 新增：new-api 原生层凭证。当前仅 newapi provider 使用（omitempty，其他 provider 无感）。
    AccessTokenEnv string `yaml:"access_token_env,omitempty"` // 存 access_token 的环境变量名
    UserID         string `yaml:"user_id,omitempty"`          // new-api 用户 ID，作 New-Api-User header
}
```

`UserID` 用 string 而非 int：header 值本就是字符串，且避免 yaml 数字解析/溢出问题。

### 3.2 `provider_usage.go` — 消耗摘要

```go
// RecentUsage 是近窗口消耗摘要（余额型 provider 的补充信息）。
// nil 表示该 provider 无此数据（UI 不渲染 Recent 区块）；零值结构体表示"拉到了但全是 0"。
type RecentUsage struct {
    Window7d  float64 // 近7天消耗（美元）
    Window30d float64 // 近30天消耗（美元）
    RPM       int     // 实时每分钟请求数
    TPM       int     // 实时每分钟 token 数
    Currency  string  // "USD"
}

type ProviderUsage struct {
    // ...现有字段不变...
    Recent *RecentUsage // 新增，由 adapter 填充，UI 读取
}
```

指针类型是有意为之：区分「无数据」（nil，UI 跳过）与「数据全零」（`Recent{}`，UI 仍渲染区块）。

## 4. 适配器重写（`internal/adapters/providers/newapi/newapi.go`）

### 4.1 端点与常量

```go
const (
    userSelfPath = "/api/user/self"
    statusPath   = "/api/status"
    logStatPath  = "/api/log/self/stat"

    defaultQPU  = 500000             // quota_per_unit 回退默认
    window7d    = 7 * 24 * time.Hour
    window30d   = 30 * 24 * time.Hour
    httpTimeout = 10 * time.Second

    nameAvailable = "Available balance"
    sourceTag     = "newapi"
)
```

**删除**：`subscriptionPath`、`usagePath` 及 `subscriptionResp`/`usageResp` 类型。

### 4.2 鉴权：`getJSON` 支持 `New-Api-User`

现有 `getJSON(ctx, url, bearer, out)` 仅设 `Authorization`。改为可附带额外 header：

```go
func (p *Provider) getJSON(ctx context.Context, url, bearer, newUser string, out any) error
```

内部设 `Authorization: Bearer <bearer>` + `New-Api-User: <newUser>`。`Content-Type: application/json` 保留。

### 4.3 `FetchUsage` 数据流

```
FetchUsage(ctx, acc):
  1. 校验 acc.BaseURL / acc.AccessTokenEnv / acc.UserID 非空 → 否则 u.Err 报错返回
  2. accessToken := os.Getenv(acc.AccessTokenEnv)
  3. GET /api/user/self  → quota, used_quota        （核心，失败 → 整体报错）
  4. GET /api/status     → quotaPerUnit             （失败 → defaultQPU=500000）
  5. now := time.Now()
     GET /api/log/self/stat?start_timestamp=now-7d&end_timestamp=now  → q7, rpm, tpm
     GET /api/log/self/stat?start_timestamp=now-30d&end_timestamp=now → q30
     （stat 失败 → Recent=nil，余额仍返回；不报错）
  6. usd := func(q) float64 { return q / quotaPerUnit }
  7. Dimensions = [{Name:"Available balance", Balance: usd(quota), Currency:"USD",
                    PercentUsed:-1, Source:"newapi"}]
     Primary 指向 Dimensions[0]
  8. （仅当两次 stat 均成功）Recent = {Window7d: usd(q7), Window30d: usd(q30), RPM, TPM, Currency:"USD"}
     —— stat 任一失败则 Recent 保持 nil（见第 5 步），UI 跳过 Recent 区块；余额不受影响
  9. Basic Info：Endpoint = userSelfPath，BaseURL = acc.BaseURL
```

4 个端点顺序执行（均 <1s，总耗时可控）；如需优化可并发，但顺序更简单且错误归因清晰，本期不做并发（YAGNI）。

## 5. UI 改动（`internal/adapters/ui/account_details.go`）

### 5.1 新增 `renderRecent`

在 `Render` 的 Quota Dimensions 块**之后**追加（仅当 `u.Recent != nil`）：

```go
func renderRecent(r domain.RecentUsage) string {
    var b strings.Builder
    b.WriteString("\n[" + colorTitle + "::b]Usage (recent)[-]\n")
    b.WriteString(basicInfoLine("7-day",  formatMoney(r.Window7d,  r.Currency)))
    b.WriteString(basicInfoLine("30-day", formatMoney(r.Window30d, r.Currency)))
    b.WriteString(basicInfoLine("Live",   fmt.Sprintf("%d rpm / %d tpm", r.RPM, r.TPM)))
    return b.String()
}
```

复用现有 `basicInfoLine`（键值对 pad10 对齐）与 `formatMoney`（带货币符号），风格与 Basic Info 一致。渲染效果（带色）：

```
Usage (recent)
  7-day:        $51.20
  30-day:       $138.56
  Live:         0 rpm / 0 tpm
```

### 5.2 `Render` 接入

`d.Dimensions` 循环后插入：
```go
if u.Recent != nil {
    b.WriteString(renderRecent(*u.Recent))
}
```

其他 provider 的 `ProviderUsage.Recent` 为 nil，行为不变（回归安全）。

### 5.3 表单（`account_form.go`）

new-api 的编辑表单增加两个输入项：`access_token_env`（环境变量名）、`user_id`。沿用现有表单字段风格。`token_env` 字段对 newapi 可隐藏或保留（保留更简单，用户可不填）。

## 6. 错误处理（沿用"核心硬、辅助软"）

| 失败项 | 策略 | 理由 |
|--------|------|------|
| `BaseURL`/`AccessTokenEnv`/`UserID` 任一为空 | `u.Err` 报错 | 无法鉴权 |
| `/api/user/self` 非 2xx 或解码失败 | `u.Err` 报错 | 拿不到余额（核心） |
| `/api/status` 失败 | `quotaPerUnit = defaultQPU` | 换算因子可默认，不阻断 |
| `/api/log/self/stat` 失败 | `Recent = nil`，余额正常返回 | 消耗为次要信息 |
| `os.Getenv(AccessTokenEnv)` 为空 | `u.Err` 报错（"access token not set in <env>"） | 凭证缺失 |

错误信息前缀统一 `newapi:`，与现有风格一致；非 nil `u.Err` 不抑制已填字段（沿用详情页 ⚠ 透出约定）。

## 7. 测试（TDD，httptest + t.Setenv）

### 7.1 `newapi/newapi_test.go`

**删除**（billing 专用）：`TestFetchUsageGolden`、`TestFetchUsage_LimitFallback`、`TestFetchUsage_UsageDegraded`、`TestFetchUsage_SubscriptionFails`。
**保留**：`TestProviderReturnsSlug`、`TestFetchUsage_BaseURLRequired`（扩展为也覆盖缺失 access_token_env / user_id）。

**新增**：

| 用例 | mock 行为 | 断言 |
|------|-----------|------|
| `TestFetchUsage_NativeGolden` | user/self(quota=121992688,used_quota=69281250) + status(quota_per_unit=500000) + 2× stat(q7,q30,rpm,tpm) | `Balance=243.99`、`Recent.Window7d/30d` 正确、`Currency="USD"`、请求带 `New-Api-User` header |
| `TestFetchUsage_QPUFallback` | status 返回 500 | 用 500000 换算，余额仍正确 |
| `TestFetchUsage_StatDegraded` | 两 stat 返回 404 | `Recent=nil`，余额正确，无 err |
| `TestFetchUsage_UserSelfFails` | user/self 返回 401 | 返回 err |
| `TestFetchUsage_MissingCreds` | AccessTokenEnv 或 UserID 为空 | 返回 err |

### 7.2 `ui/account_details_test.go`

新增 `TestRenderRecent`：构造带 `Recent` 的 `ProviderUsage`，断言输出含 `Usage (recent)`、`7-day`、`30-day`、`Live` 行且金额格式正确；构造 `Recent=nil` 断言不含该区块。

### 7.3 验证门槛

每步 `go build ./...`；终态 `make test`（`go test -race -cover ./...`）全绿、`make quality`（gofumpt + go vet）通过。字段校准：用 kuaipao.pro 真实响应比对（余额对得上后台 ≈244 USD 即正确）。

## 8. 破坏性变更与迁移

new-api 账号配置变更：

```yaml
# 旧（v0.1.x，billing 路径，已废弃）
- id: n1
  provider: newapi
  base_url: https://kuaipao.pro
  token_env: NEWAPI_KEY          # sk-key，不再用于 newapi

# 新（v0.2.0，原生路径）
- id: n1
  provider: newapi
  base_url: https://kuaipao.pro
  access_token_env: NEWAPI_AT    # 存 access_token 的环境变量
  user_id: "16002"               # new-api 用户 ID（后台个人设置生成 access token；user_id 见 Local Storage/Network 请求头）
```

- access token 生成路径：后台 → 个人设置 → 系统访问令牌。
- user_id 获取：F12 → Network → 任一 `/api/` 请求的 `New-Api-User` header，或 Local Storage 的 `user.id`。
- 未补字段的旧 newapi 账号刷新时报清晰错误（§6），不静默失败。
- bump **v0.2.0**（破坏性配置变更）。
- `README.md` / `README.zh-CN.md` 更新 new-api 配置章节与迁移说明。

## 9. 非目标

- 不接入消费明细（`/api/log/self?type=2` 按模型/key 流水）——本期只取标量摘要。
- 不接入令牌维度（`/api/token/` 每个 key 的余额）——该 fork 端点路径待确认，且多 key 用户场景未明确。
- 不做 sparkline/趋势图——B 展示用标量键值对。
- 不保留 billing 路径作回退——假数据无回退价值（YAGNI）。
- 不为 stat 做并发请求优化——顺序足够。
- 不改其他 provider（GLM/MiniMax/Kimi/DeepSeek/sub2api）。

## 10. 开放问题

- `account_form.go` 中 newapi 的 `token_env` 字段是否对用户隐藏？倾向**保留**（表单逻辑更简单，用户可不填），实现时确认。
- 若未来 sub2api 等其他中转站也暴露真实 quota，`RecentUsage` 结构可复用——本期不预先抽象。
