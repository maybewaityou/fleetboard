# DeepSeek / Kimi 余额细分与账号状态接入 设计规格

> 状态：已确认 · 日期：2026-07-28 · 作者：MeePwn
> 范围：把 DeepSeek 与 Kimi 两个余额型 provider 响应里**已被丢弃**的字段用起来——余额细分（赠送 / 充值）与 DeepSeek 账号可用状态（is_available）。
> 参考：`deepseek.go` / `kimi.go`（struct 字段早已声明、仅未读取）、`2026-07-28-newapi-native-balance-design.md`（「次要信息失败不影响主余额」的既有错误处理哲学）。

## 0. 决策摘要（已与用户确认）

| 议题 | 决策 |
|------|------|
| 接入范围 | ① 余额细分 Granted/ToppedUp（DeepSeek + Kimi）；② 账号状态 Status（仅 DeepSeek） |
| 细分字段归属 | 方案 A：扩展 `UsageDimension` 加 `Granted`/`ToppedUp` 两个字段（非新增维度） |
| Kimi 字段映射 | `voucher_balance`→`Granted`（赠送券），`cash_balance`→`ToppedUp`（现金）——与 DeepSeek granted/topped_up 同构 |
| is_available 落点 | **新增** `ProviderUsage.Status` 字段；**不复用** `APIKeyStatus`（后者已被 sub2api 用作 key 的 active/expired，混用语义错乱） |
| Status 取值 | `is_available=true`→`"active"`，`false`→`"insufficient"`（英文，与 `APIKeyStatus` 取值风格一致） |
| UI 落点 | 详情页：余额维度分支加 Granted/Topped up 两行（非零显示）+ Basic Info 加 Status 行；**列表 mini 视图不改** |
| 细分解析容错 | granted/topped_up 解析失败**不致命**：跳过该细分（留零值），不影响主余额 total/available（沿用 newapi「次要信息失败不拖垮核心余额」哲学） |
| 不在范围 | newapi（单一 quota、无细分）、Recent 消耗摘要（DeepSeek/Kimi 无数据源）、列表 mini 细分、多币种 balance_infos[] |

执行顺序：domain 扩展 → deepseek adapter 接线 → kimi adapter 接线 → UI（Status 行 + 余额细分行）→ 测试（先红后绿）。每步 `go build ./...` + `make test` 绿。

## 1. 背景与动机：被丢弃的真实数据

DeepSeek 的 `/user/balance` 响应共 5 个字段（`is_available` + `balance_infos[]` 的 currency/total_balance/granted_balance/topped_up_balance）。当前 adapter（`deepseek.go`）只读取 `total_balance`→`Balance` 与 `currency`→`Currency`，**三个字段被直接丢弃**：

| 丢弃字段 | 含义 | 价值 |
|----------|------|------|
| `is_available` | 余额是否足以调用 API（欠费/可用） | 账号健康指示——欠费但仍有余额时仅看余额无法察觉 |
| `granted_balance` | 赠送余额（未过期） | 知道余额里多少是平台赠送、即将过期 |
| `topped_up_balance` | 充值余额 | 知道余额里多少是自费充值 |

恒等式 `total_balance = granted_balance + topped_up_balance` 已写在 `deepseek.go:23` 注释里，但三个细分值从未展示。

Kimi 完全同构：`available_balance = voucher_balance`（赠送券）+ `cash_balance`（现金），当前同样只展示 available，丢弃 voucher/cash。`kimi.go` 的 `apiResp.Data` struct 早已声明 `VoucherBalance`/`CashBalance`，仅 `FetchUsage` 未读取。

**结论**：这是一次「字段早已就位、只需接线」的低风险扩展，让两个余额型 provider 的详情页都能展示余额构成与（DeepSeek）账号状态。

## 2. 接口契约回顾

### 2.1 DeepSeek `/user/balance`（已实测确认，api-docs.deepseek.com）

```json
{
  "is_available": true,
  "balance_infos": [{
    "currency": "CNY",
    "total_balance": "110.00",
    "granted_balance": "10.00",
    "topped_up_balance": "100.00"
  }]
}
```

- `is_available` (bool)：余额是否充足，根级。
- 三个金额字段均为 **string**，需 `strconv.ParseFloat`，单位即元/美元，无换算。
- DeepSeek 平台**仅此一个**账户信息端点，无消耗明细/实时速率接口（故 Recent 区块对 DeepSeek 永不填充）。

### 2.2 Kimi `/v1/users/me/balance`

```json
{ "code": 0, "data": { "available_balance": 110.0, "voucher_balance": 10.0, "cash_balance": 100.0 }, "status": true }
```

- 金额字段为 **float64**，直接可用，无需解析。
- Kimi 无 is_available 等价字段（`status` 是请求状态非账号状态），故 `Status` 仅 DeepSeek 填充。

### 2.3 同构映射

| 统一字段 | DeepSeek 来源 | Kimi 来源 |
|----------|---------------|-----------|
| `Balance`（已用） | total_balance | available_balance |
| `Granted`（新增） | granted_balance | voucher_balance |
| `ToppedUp`（新增） | topped_up_balance | cash_balance |

## 3. 数据模型扩展（domain）

### 3.1 `provider_usage.go` — `UsageDimension` 加两个余额细分字段

```go
type UsageDimension struct {
    // ...现有字段不变（Name/Used/Limit/PercentUsed/Remaining/ResetsAt/Unit/Source/Balance/Currency/MoneyLimit/MoneyUsed/Order）...

    // 余额细分（余额型 provider 可选）：Granted=赠送/赠券部分，ToppedUp=充值/现金部分。
    // DeepSeek 填 granted_balance/topped_up_balance；Kimi 填 voucher_balance/cash_balance。
    // 配额型与其他余额型 provider 零值=无，UI 不渲染。语义约定 Granted+ToppedUp==Balance。
    Granted  float64
    ToppedUp float64
}
```

命名权衡：用 `Granted`/`ToppedUp` 而非 `GrantedBalance`/`ToppedUpBalance`——与现有 `Balance`/`MoneyLimit`/`MoneyUsed` 的简洁风格一致，且它们本就是 Balance 的子项，「Balance」前缀冗余。

### 3.2 `provider_usage.go` — `ProviderUsage` 加账号状态字段

```go
type ProviderUsage struct {
    // ...现有字段不变...

    // 账号可用状态（adapter 填充，UI 读取）。DeepSeek 由 is_available 映射：
    // true→"active"，false→"insufficient"。其他 provider 零值=无，UI 不渲染。
    // 与 APIKeyStatus（sub2api 的 key active/expired）语义不同，故独立成字段。
    Status string
}
```

## 4. 适配器改动（struct 不动，仅补读取）

### 4.1 `deepseek.go`

`balanceInfo` struct（第 70-75 行）已声明 `GrantedBalance`/`ToppedUpBalance`，`apiResp` 已声明 `IsAvailable`——**struct 零改动**，仅在 `FetchUsage` 补接线：

1. **Status（先填，确保错误路径也携带）**：解码成功后立即 `u.Status = "active"` / `"insufficient"`（按 `r.IsAvailable`），早于 total_balance 解析——这样即便后续金额解析失败报错，账号状态仍能反映给 UI。
2. **细分（容错解析）**：对 `info.GrantedBalance`、`info.ToppedUpBalance` 各做 `strconv.ParseFloat`；**解析失败不报错**，该细分留零值（主余额 total 不受影响）。

```go
// Status 先于金额解析填充（错误路径也携带账号状态）。
if r.IsAvailable {
    u.Status = "active"
} else {
    u.Status = "insufficient"
}

total, err := strconv.ParseFloat(info.TotalBalance, 64)
if err != nil { /* 现有致命错误分支不变 */ }

// 细分容错解析：ParseFloat 失败时返回 0 值，用 _ 忽略 err 即可——
// 主余额 total 已成功，细分缺失（=0）不致命，UI 自动跳过零值行。
granted, _ := strconv.ParseFloat(info.GrantedBalance, 64)
topped, _ := strconv.ParseFloat(info.ToppedUpBalance, 64)

u.Dimensions = []domain.UsageDimension{{
    Name:        nameAvailable,
    Balance:     total,
    Currency:    info.Currency,
    PercentUsed: -1,
    Source:      sourceTag,
    Granted:     granted,
    ToppedUp:    topped,
}}
```

### 4.2 `kimi.go`

`apiResp.Data` struct 已声明 `VoucherBalance`/`CashBalance`（float64，直接可用）。在构建 Dimensions 时填充：

```go
u.Dimensions = []domain.UsageDimension{{
    Name:        nameAvailable,
    Balance:     r.Data.AvailableBalance,
    Currency:    currencyFor(base),
    PercentUsed: -1,
    Source:      sourceTag,
    Granted:     r.Data.VoucherBalance,   // 赠送券 → Granted
    ToppedUp:    r.Data.CashBalance,      // 现金 → ToppedUp
}}
```

Kimi 无 is_available，`Status` 不填（零值，UI 不渲染 Status 行）。

## 5. UI 改动（`internal/adapters/ui/account_details.go`）

### 5.1 Basic Info 加 Status 行

接在 `APIKeyStatus` 渲染之后（第 83-85 行邻近），同 `basicInfoLine` 模式：

```go
if u.APIKeyStatus != "" {
    b.WriteString(basicInfoLine("API Key", u.APIKeyStatus))
}
if u.Status != "" {
    b.WriteString(basicInfoLine("Status", u.Status))
}
```

### 5.2 `renderDimension` 余额分支加细分行

当前余额分支（第 210-215 行）只输出一行 `Balance:`。改为非零追加 Granted/Topped up：

```go
if dim.Currency != "" {
    fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
        colorSecondary, "Balance:", colorPrimary, formatMoney(dim.Balance, dim.Currency))
    if dim.Granted != 0 {
        fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
            colorSecondary, "Granted:", colorPrimary, formatMoney(dim.Granted, dim.Currency))
    }
    if dim.ToppedUp != 0 {
        fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
            colorSecondary, "Topped up:", colorPrimary, formatMoney(dim.ToppedUp, dim.Currency))
    }
    b.WriteString("\n")
    return b.String()
}
```

渲染效果（DeepSeek，granted=10/topped=100）：

```
  Available balance
      Balance:     ¥110.00
      Granted:     ¥10.00
      Topped up:   ¥100.00
```

零值细分（如某 provider 仅返回 total、无细分）则只显示 Balance 行，与现状一致——**向后兼容**。

### 5.3 Basic Info 区块效果（DeepSeek，is_available=true）

```
  Basic Info
    Plan:        —
    Provider:    [ deepseek ]
    Status:      active              ← 新增
    BaseURL:     https://api.deepseek.com
    Endpoint:    /user/balance
    Refreshed:   2026-07-28 14:30
    Pinned:      false
```

列表 mini 视图**不改**——保持紧凑，细分与状态仅详情页呈现。

## 6. 测试（httptest + golden 模式）

### 6.1 `deepseek_test.go`

- **golden 用例**（`TestFetchUsageGolden`）：补断言 `Granted=10.0`、`ToppedUp=100.0`、`Status="active"`（goldenPayload 已含 granted=10/topped=100/is_available=true）。
- **新增** `TestFetchUsageUnavailable`：`is_available:false` 的响应，验 `Status="insufficient"` 且余额照常返回。
- **新增** `TestFetchUsageBadGrantedBalance`：`granted_balance:"oops"` 非法，验主余额 total 仍成功、`Granted=0`（容错不致命）。

### 6.2 `kimi_test.go`

- golden 用例补断言 `Granted=voucher 值`、`ToppedUp=cash 值`、`Status==""`（Kimi 不填状态）。

### 6.3 `account_details` UI 测试

- 余额维度含非零 Granted/ToppedUp → 输出含 "Granted:" / "Topped up:" 行。
- 余额维度全零细分 → 仅 "Balance:" 行（向后兼容）。
- `ProviderUsage.Status` 非空 → Basic Info 含 "Status:" 行；空 → 无该行。

## 7. 不在范围与取舍

- **newapi**：单一 `quota`、无赠送/充值拆分，且靠 Recent 区块展示消耗——不适用细分，`Granted`/`ToppedUp` 留零值，UI 无感。
- **Recent 消耗摘要**（RPM/TPM/7d/30d/今日累计）：DeepSeek/Kimi 官方无此类接口，`Recent` 对二者永不填充——勿被模型现成字段误导。
- **列表 mini 视图细分**：保持紧凑，不加。
- **多币种 `balance_infos[]`**：DeepSeek 通常单币种，仍取 `[0]`，多档渲染留待后续。
- **Status 取色/图标**：本期仅文本 "active"/"insufficient"，不为 insufficient 单独染色（如需，后续按 StatusColor 扩展）。
