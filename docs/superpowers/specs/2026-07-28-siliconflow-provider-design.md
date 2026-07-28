# SiliconFlow（硅基流动）provider 接入 设计规格

> 状态：已确认 · 日期：2026-07-28 · 作者：MeePwn
> 范围：新增 `siliconflow` provider，对接硅基流动 `/v1/user/info` 账户信息接口，把可用余额作为主余额展示，并在详情页补充「充值 / 总额」两项余额信息行。
> 参考：`deepseek.go`（同为单凭证余额型 adapter，本特性直接以此为模板）、`2026-07-28-deepseek-balance-breakdown-design.md`（余额细分字段归属与「次要信息失败不拖垮主余额」的错误处理哲学）、`2026-07-28-sub2api-details-and-ui-tweaks-design.md`（字符串金额字段 `ParseFloat` 容错的既有先例）。

## 0. 决策摘要（已与用户确认）

| 议题 | 决策 |
|------|------|
| provider 标识 | `siliconflow`（全小写，与官方一致） |
| 类型 | 纯**余额型**（按量扣费，无 5h/周/月配额窗口）；走 `Currency != ""` 余额渲染分支 |
| 接口端点 | `GET {base}/v1/user/info`，默认 base `https://api.siliconflow.cn`，`base_url` 可选覆盖 |
| 鉴权 | `Authorization: Bearer <API_KEY>` 单凭证，token 从 `token_env` 读（与 deepseek/kimi 同构） |
| 主余额来源 | `data.balance`（当前可用余额，string → `ParseFloat`） |
| 细分字段归属 | **方案 A**：扩展 `UsageDimension` 加 `ChargeBalance` + `TotalBalance` 两字段（非新增维度、非新 struct） |
| 细分语义 | **不做相加约定**（区别于 Granted/ToppedUp 的 `Granted+ToppedUp==Balance`）。label 用 API 原文「Charged」「Total」，不强行翻译成「剩余充值」——官方示例数据对「累计 vs 剩余」有歧义 |
| Status 映射 | `data.status == "normal"` → `"active"`；其余取值 → **保留原值字符串**（让用户看到 frozen/banned 等真实状态）。复用 `ProviderUsage.Status` 字段（DeepSeek 已用），不复用 `APIKeyStatus` |
| 货币 | 固定 `CNY`（国内站为主；国际站用户可 `base_url` 覆盖 `.com` 域名，货币仍按 CNY 显示） |
| 响应信封 | `{code, message, status, data:{...}}`，`code == 20000` 为成功（区别于 HTTP 状态码） |
| 不在范围 | 用量配额窗口（SiliconFlow 无此概念）、消耗明细/速率（接口未提供）、列表 mini 细分、多币种 |

执行顺序：domain 扩展 → siliconflow adapter → 装配（main.go + account_form.go + README）→ UI 渲染分支 → 测试（先红后绿）。每步 `go build ./...` + `make test` 绿。

## 1. 背景与动机

fleetboard 现支持 6 家 provider（glm / minimax / kimi / deepseek / sub2api / newapi），其中国内直连余额型已有 Kimi、DeepSeek。硅基流动（SiliconFlow）是国内主流的 AI 模型聚合平台，在 AI Coding 圈使用广泛，按量扣费、账户余额是用户最关心的指标，但当前无法在 fleetboard 一屏查看。

架构为新增 provider 优化到了极致（`UsageProvider` 是唯一数据端口，新厂商 = 一个 adapter），接入成本低。SiliconFlow 的 `/v1/user/info` 是单凭证、单端点、纯余额型，与 DeepSeek 高度同构，是理想的下一家。

**与既有余额细分的差异**：DeepSeek/Kimi 的细分（granted/topped_up、voucher/cash）是「剩余拆分」，文档明确保证「相加 == 当前余额」。SiliconFlow 的 `chargeBalance` / `totalBalance` 没有这种保证，且官方示例数据（balance=0.88 / chargeBalance=88.00 / totalBalance=88.88）对「累计 vs 剩余」存在歧义。因此本特性**不复用** Granted/ToppedUp 字段（避免破坏相加约定），而是新增独立字段、用 API 原文做 label，保证语义诚实。

## 2. 接口契约（已实测确认，docs.siliconflow.com）

### 2.1 请求

```
GET https://api.siliconflow.cn/v1/user/info
Authorization: Bearer <API_KEY>
```

- 默认 base：`https://api.siliconflow.cn`（国内站）；国际站 `api.siliconflow.com`，可用账号 `base_url` 覆盖。
- 单凭证 Bearer 鉴权，token 经 `token_env` 指定的环境变量读取（与 deepseek 一致）。

### 2.2 响应

```json
{
  "code": 20000,
  "message": "OK",
  "status": true,
  "data": {
    "id": "userid",
    "balance": "0.88",
    "status": "normal",
    "chargeBalance": "88.00",
    "totalBalance": "88.88"
  }
}
```

- 根级 `code` (int)：业务状态码，`20000` 为成功（区别于 HTTP 状态码，需显式校验）。
- `data.balance` (string)：当前可用余额（主展示）。
- `data.chargeBalance` (string)：充值余额。
- `data.totalBalance` (string)：总余额。
- `data.status` (string)：账号状态，已知取值 `"normal"`。
- 三个金额字段均为 **string**，需 `strconv.ParseFloat`，无单位换算。
- `data.name` / `data.image` / `data.email`：官方公告 2025-06-11 起不再返回，**不要依赖**。

### 2.3 字段映射

| 统一字段 | SiliconFlow 来源 | 说明 |
|----------|------------------|------|
| `Balance`（主） | `data.balance` | 当前可用余额 |
| `ChargeBalance`（新增） | `data.chargeBalance` | 充值余额（API 原值，不做语义推断） |
| `TotalBalance`（新增） | `data.totalBalance` | 总余额（API 原值） |
| `Currency` | 固定 `"CNY"` | 隐含人民币，接口无货币字段 |
| `Status` | `data.status` | `normal`→`active`，其余保留原值 |

## 3. 数据模型扩展（domain）

### 3.1 `provider_usage.go` — `UsageDimension` 加两个余额信息字段

```go
type UsageDimension struct {
    // ...现有字段不变（Name/Used/Limit/PercentUsed/Remaining/ResetsAt/Unit/Source/
    //    Balance/Currency/Granted/ToppedUp/MoneyLimit/MoneyUsed/Order）...

    // SiliconFlow 余额信息（adapter 填充，UI 读取）。零值=无，UI 不渲染。
    // 与 Granted/ToppedUp（剩余拆分，相加=Balance）语义不同：这里是 API 原值，
    // 不做相加约定——官方未保证 chargeBalance/totalBalance 与 balance 的恒等关系。
    // 仅 siliconflow provider 填充；配额型与其他余额型 provider 零值=无。
    ChargeBalance float64
    TotalBalance  float64
}
```

零值无害：配额型（glm/minimax）与既有余额型（kimi/deepseek/sub2api/newapi）不填这两个字段，UI 的 `!= 0` 跳过逻辑自动忽略，完全向后兼容。

## 4. adapter 设计（`internal/adapters/providers/siliconflow/siliconflow.go`）

以 `deepseek.go` 为模板，逐字对齐其结构与注释风格。

### 4.1 常量与类型

```go
const (
    defaultBaseURL = "https://api.siliconflow.cn"
    usagePath      = "/v1/user/info"
    httpTimeout    = 10 * time.Second

    sourceTag     = "api-balanced"
    nameAvailable = "Available balance"
)

// 响应信封。金额字段为 string。
type apiResp struct {
    Code    int      `json:"code"`
    Message string   `json:"message"`
    Status  bool     `json:"status"`
    Data    userInfo `json:"data"`
}

type userInfo struct {
    ID            string `json:"id"`
    Balance       string `json:"balance"`
    Status        string `json:"status"`
    ChargeBalance string `json:"chargeBalance"`
    TotalBalance  string `json:"totalBalance"`
}
```

`Provider` struct 持有 `*http.Client`（超时 10s），`New()` 构造，`Provider()` 返回 `"siliconflow"`。`var _ ports.UsageProvider = (*Provider)(nil)` 编译期保证接口实现。

### 4.2 `FetchUsage` 流程

1. 填充基础字段（AccountID/Provider/Label/FetchedAt）；`base := acc.BaseURL`，空则 `defaultBaseURL`；记录 `u.BaseURL`、`u.Endpoint = usagePath`。
2. `key := os.Getenv(acc.TokenEnv)`；`GET base+usagePath` + `Authorization: Bearer <key>` + `Content-Type: application/json`。
3. HTTP 非 2xx → 填 `u.Err` 返回。
4. `json.Decode` 失败 → 填 `u.Err` 返回。
5. **信封校验**：`r.Code != 20000` → `u.Err = fmt.Errorf("siliconflow: code %d: %s", r.Code, r.Message)` 返回。
6. **Status 映射**（先于金额解析，确保错误路径也携带状态）：
   ```go
   if r.Data.Status == "normal" {
       u.Status = "active"
   } else if r.Data.Status != "" {
       u.Status = r.Data.Status // 保留原值（frozen/banned 等）
   }
   ```
7. **主余额严格解析**：`balance, err := strconv.ParseFloat(r.Data.Balance, 64)`；失败 → `u.Err` 返回。
8. **细分容错解析**：`charge, _ := strconv.ParseFloat(r.Data.ChargeBalance, 64)`、`total, _ := strconv.ParseFloat(r.Data.TotalBalance, 64)`——用 `_` 忽略 err。主余额已成功，细分缺失（=0）不致命，UI 自动跳过零值行（沿用 deepseek/newapi「次要信息失败不拖垮核心余额」哲学）。
9. 填维度：
   ```go
   u.Dimensions = []domain.UsageDimension{{
       Name:          nameAvailable,
       Balance:       balance,
       Currency:      "CNY",
       PercentUsed:   -1,
       Source:        sourceTag,
       ChargeBalance: charge,
       TotalBalance:  total,
   }}
   u.Primary = &u.Dimensions[0]
   ```

出错路径仍返回带账号字段（AccountID/Provider/Label/FetchedAt/BaseURL/Endpoint/Err）的 `ProviderUsage`，与 deepseek 一致，UI 据此在列表显示错误态。

## 5. 装配（3 处机械改动）

1. **`cmd/main.go`**：import `siliconflow` 包；`NewRegistry(...)` 调用追加 `siliconflow.New()`（当前在 `main.go:124`）。
2. **`internal/adapters/ui/account_form.go:37`**：`providerOptions` 切片追加 `"siliconflow"`（与 cmd/main 注册的 adapter 一一对应）。
3. **`README.md` / `README.zh-CN.md`**：providers 表格各加一行——`siliconflow` | Balance | 可用余额（CNY）+ 充值/总额细分 | optional。

配置示例（写入 README 配置段）：
```yaml
accounts:
  - id: siliconflow-main
    provider: siliconflow
    label: SiliconFlow main
    token_env: SILICONFLOW_API_KEY
```

## 6. UI 渲染

### 6.1 列表（无需新代码）
余额型，走既有 `formatMoneyShort(dim.Balance, "CNY")` 与状态点逻辑。

### 6.2 详情页（`account_details.go` 余额分支，约 `:213`）
在 `Balance:` 行之后、`Granted`/`ToppedUp` 判断同级，追加：
```go
if dim.ChargeBalance != 0 {
    fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
        colorSecondary, "Charged:", colorPrimary, formatMoney(dim.ChargeBalance, dim.Currency))
}
if dim.TotalBalance != 0 {
    fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
        colorSecondary, "Total:", colorPrimary, formatMoney(dim.TotalBalance, dim.Currency))
}
```
渲染效果：
```
Available balance
    Balance:   ¥0.88
    Charged:   ¥88.00
    Total:     ¥88.88
```

### 6.3 Status 行
复用 DeepSeek 已有的 `ProviderUsage.Status` 渲染逻辑（Basic Info 区块的 Status 行）。`active` 正常着色；非 active 原值字符串按现有规则显示。

## 7. 错误处理哲学

完全沿用 deepseek 既定模式：
- **主余额失败 = 整体失败**：`balance` 解析失败、信封 `code != 20000`、HTTP 非 2xx、网络错误、decode 失败 → 返回 `Err`，UI 列表显示错误态。
- **次要信息失败不拖垮核心**：`chargeBalance`/`totalBalance` 解析失败 → 容错为 0，跳过该行，不影响主余额展示。
- **状态优先于金额**：`Status` 在金额解析之前填充，即使金额解析出错，账号状态仍随 `ProviderUsage` 返回。

## 8. 测试策略（httptest golden，仿 `deepseek_test.go`）

`internal/adapters/providers/siliconflow/siliconflow_test.go`，用 `httptest.NewServer` 返回固定 JSON：

- `TestFetchUsageSuccess`：完整成功响应 → 断言 `Balance==0.88`、`ChargeBalance==88.0`、`TotalBalance==88.88`、`Currency=="CNY"`、`Status=="active"`、`Primary` 指向、`PercentUsed==-1`。账号用 `BaseURL: srv.URL`（httptest 地址）覆盖默认 base，并断言 `u.BaseURL` 回填——base 覆盖逻辑随此路径覆盖，不另设独立测试（与 deepseek 一致）。
- `TestFetchUsageStatusNonNormal`：`data.status=="frozen"` → `Status=="frozen"`（保留原值）。
- `TestFetchUsageBadBalance`：`balance` 非数字 → 返回非 nil `Err`，`Dimensions` 为空。
- `TestFetchUsageChargeBalanceBad`：`chargeBalance` 非数字、`balance` 合法 → 无 `Err`，`ChargeBalance==0`（容错）。
- `TestFetchUsageBadEnvelope`：`code != 20000`（如 `code:40100`）→ 返回 `Err`，Err 信息含 code。
- `TestFetchUsageHTTPError`：返回 HTTP 500 → 返回 `Err`。
- `TestFetchUsageEmptyToken`：`token_env` 指向未设置的环境变量 → 请求仍发出（SiliconFlow 不会因缺 token 在客户端报错，服务端返回 401 → 走 HTTP 错误路径）。验证不 panic、返回 Err。
- `TestProvider`：`Provider()` 返回 `"siliconflow"`。

`FetchUsage` 的成功路径须 `go test -race` 通过（adapter 无共享状态，天然安全）。

## 9. 不在范围（YAGNI）

- 用量配额窗口：SiliconFlow 按量扣费，无 GLM 式配额维度，不填 `PercentUsed`/`ResetsAt`。
- 消耗明细 / 实时速率：`/v1/user/info` 不提供，`RecentUsage` 永不填充。
- 列表 mini 视图的细分展示：列表只显示主余额，细分仅详情页。
- 多币种：SiliconFlow 单一 CNY，无需 `balance_infos[]` 式多币种处理。
- 国际站 USD 适配：当前固定 CNY；若国际站用户反馈货币不符，后续再按 `base_url` 域名区分。
