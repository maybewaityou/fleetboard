# sub2api 详情扩展与三项 UI 优化设计规格

> 状态：草案 v1 · 日期：2026-07-28 · 作者：MeePwn

## 1. 概述

本规格覆盖三项独立但同批交付的改进：

1. **列表金额保留 2 位小数** —— 列表行的余额由 1 位小数改为 2 位。
2. **删除确认对话框改造** —— 用账号**名称**替换现在的**ID**，并参照 `lazytmux` 的删除（Kill）确认按钮复刻其位置、颜色、事件与快捷键。
3. **sub2api 详情页字段大幅扩展** —— 当前 sub2api 详情只显示单一余额，且响应解析基于错误的旧契约。本项重写 sub2api 适配器对接 `/v1/usage` 真实契约，按账号模式动态展示配额/余额、速率窗口、用量统计、套餐订阅、状态与过期等全部字段。

需求③是主体（涉及 domain 模型扩展与适配器重写），①②为附带的小幅 UI 调整。

## 2. 背景与动机

### 2.1 列表金额精度
`formatMoneyShort`（`account_list.go:228`）现为 `%.1f`，例如 `$42.5`。详情页 `formatMoney` 已是 `%.2f`（`$42.50`），列表与详情精度不一致。统一为 2 位小数，与详情对齐，也更符合"金额"直觉。

### 2.2 删除确认的辨识问题
当前 `confirmDelete`（`handlers.go:103-125`）文案为 `"Delete account " + id + "?"`，显示的是 12 字符 hex ID（如 `4f3a9c2b1e8d`），用户根本无法辨认要删的是哪个账号。按钮为纯文本 `"Delete"/"Cancel"`，无颜色、无字母/ESC 快捷键，与同族工具 `lazytmux` 的删除确认体验割裂。

### 2.3 sub2api 契约严重过时（核心问题）
现有 `sub2api.go:59-62` 假设 `/v1/usage` 返回 `{balance float, used float}`。对 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) `main` 分支源码（`backend/internal/handler/gateway_handler.go` 的 `Usage` handler）核实后发现：

- **`used` 字段在真实响应中根本不存在**，消耗实际位于 `usage.total.cost`。现有 `apiResp.Used` 解码后被丢弃，本就拿不到值。
- **只有"钱包余额"模式才有顶层 `balance`**。真实响应是**双模式**（`quota_limited` / `unrestricted`），且 `unrestricted` 又分**订阅**与**钱包**两子模式。配额型与订阅型账号**没有 `balance` 字段**，fleetboard 当前会把余额显示为 `0` —— **显示错误**。
- 真实响应信息极为丰富：5h/1d/7d 速率窗口（带重置时间）、今日/累计 token 与 cost、RPM/TPM、API Key 状态与过期、套餐与订阅日/周/月限额等，fleetboard **全部未利用**。

### 2.4 鉴权确认
`GET {BaseURL}/v1/usage`，请求头 `Authorization: Bearer <sk-api-key>`。**只需 API Key，不需要 JWT**（JWT 仅 `/api/v1/usage/*` 内部管理接口与日志接口需要）。fleetboard 现有鉴权方式正确，无需改动。可选 query：`days`(1-90)、`start_date`/`end_date`/`timezone`。

## 3. 目标与非目标

### 目标
- 列表金额统一 2 位小数，与详情一致。
- 删除确认显示账号名称 + provider 标识，按钮/快捷键/事件 1:1 复刻 lazytmux。
- sub2api 适配器对接 `/v1/usage` 真实双模式契约，余额/配额对三种账号模式都正确。
- sub2api 详情页按账号实际模式动态展示：配额/余额、5h/1d/7d 速率窗口、今日/累计用量统计、套餐与订阅周期、API Key 状态与过期。缺失字段自动隐藏。
- 复用 fleetboard 现有的多窗口维度、进度条染色、最近窗口优先机制，sub2api 配额型账号获得与 GLM 同构的多窗口体验。

### 非目标
- 不对接 `daily_usage`（按天曲线）与 `model_stats`（按模型统计）—— 数据量大，留待后续做历史趋势时一并。
- 不改动其他 provider（glm/minimax/kimi/deepseek/newapi）的现有行为。domain 新增字段对它们零值无害。
- 不做 sub2api 的 JWT 类内部管理接口（日志/计费明细等）。
- 不改 sub2api 鉴权方式与配置项（`token_env` + `base_url`）。

## 4. 需求①：列表金额 2 位小数

### 4.1 改动
`internal/adapters/ui/account_list.go` 的 `formatMoneyShort`（`:228-240`）：

| 分支 | 现状 | 改后 |
|------|------|------|
| 负值 + ≥1000 | `"-" + sym + fmt.Sprintf("%.1fk", -balance/1000)` | `%.2fk` |
| 负值 + <1000 | `"-" + sym + fmt.Sprintf("%.1f", -balance)` | `%.2f` |
| 正值 + ≥1000 | `fmt.Sprintf("%s%.1fk", sym, balance/1000)` | `%.2fk` |
| 正值 + <1000 | `fmt.Sprintf("%s%.1f", sym, balance)` | `%.2f` |

仅把 `%.1f`/`%.1fk` 改为 `%.2f`/`%.2fk`，缩写阈值（≥1000）与负号前置逻辑不变。

### 4.2 列宽
`padDisplay(pctStr, 7)`（`:221`）保持 7。改后最长显示：`$999.99` = 7（顶满）、`$100.00` = 7、`$1.23k` = 6、`-¥99.99` = 6，均 ≤ 7，列宽足够，无需调整。

### 4.3 验证
更新 `account_list_test.go` 中 `formatMoneyShort` 的金样本断言（`1.5 → $1.50`、`1234.5 → $1.23k`、`-50 → -¥50.00` 等）。

## 5. 需求②：删除确认改造

### 5.1 显示内容
通过 `t.accountList.GetSelected()`（`tui.go:98` 持有 `accountList`，`:360/:395` 已有用法）取 `domain.ProviderUsage`，得到 `Label` 与 `Provider`。文案：

```
Delete account 「<Label>」<provider chip>?

This action cannot be undone.
```

provider chip 复用列表渲染样式（`formatAccountLine:219` 的 `[black:<bg>] <provider> [-:-:-]`，颜色取自 `ProviderTag`），保持与列表视觉一致。`id` 不再出现在用户可见处（仍作为 `onDeleteAccount(id)` 的参数透传）。

### 5.2 按钮（1:1 复刻 lazytmux `showKillConfirmModal`）
- **顺序**：`["Cancel", "Delete"]`（Cancel 在前）。tview Modal 默认聚焦第一个按钮 → **Cancel 是安全默认**，误按回车只触发取消。
- **颜色标签**（嵌入按钮文字，复用 `const.go` 调色板，与 lazytmux 逐字一致）：
  - Cancel：`"[" + colorAccent + "]C[-]ancel"` → `[#7aa2f7]C[-]ancel`
  - Delete：`"[" + colorRed + "]D[-]elete"` → `[#f7768e]D[-]elete`（破坏性操作 = 红）

### 5.3 事件（双路径调度，同 lazytmux）
- `SetDoneFunc(func(buttonIndex int, _ string))`：`buttonIndex == 1` → 删除；`0` 或 `-1`（ESC 透传）→ 取消。
- `SetInputCapture`：`d`/`D` → 删除（确认键 = 触发键，lazytmux 惯例）；`c`/`C` → 取消；`KeyESC` → 取消。
- 抽本地闭包 `doDelete(id string)` 供两条路径共用，避免逻辑重复。删除成功后 `t.closeModal()` + `t.applyDataset(usages)`（沿用现有同步刷新模式，注释说明 modal 回调在主循环执行不能 QueueUpdateDraw）。

### 5.4 改动定位
仅 `internal/adapters/ui/handlers.go` 的 `confirmDelete`（`:103-125`）。快捷键 `d`（`tui.go:497`）、帮助面板声明（`keybindings.go:34`）、状态栏提示（`status_bar.go:58`）均不变。

## 6. 需求③：sub2api 详情全字段（主体）

### 6.1 sub2api `/v1/usage` 真实契约（已核实）

响应顶层 `mode` 决定分支：

| 模式 | 触发 | 余额/配额字段 |
|------|------|---------------|
| `quota_limited` | API Key 配了总额度或速率限制 | `quota{limit,used,remaining,unit:"USD"}`、顶层 `remaining`/`unit`、`rate_limits[]` |
| `unrestricted`-订阅 | 无 key 级限制、所在组为订阅型 | `planName`、`subscription{daily/weekly/monthly_usage_usd, *_limit_usd, weekly_window_start, expires_at}`、`remaining` |
| `unrestricted`-钱包 | 无 key 级限制、钱包余额计费 | `planName:"钱包余额"`、顶层 `balance`、`remaining` |

**三模式共有**：`isValid`、`status`、`usage{today:{requests,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,total_tokens,cost,actual_cost}, total:{…}, average_duration_ms, rpm, tpm}`、`daily_usage`、`model_stats`。

`quota_limited` 额外：`rate_limits[]` 每项 `{window:"5h"|"1d"|"7d", limit, used, remaining, window_start, reset_at}`，以及 `expires_at`/`days_until_expiry`。

### 6.2 方案选择：扩展 domain（方案 A）

| 方案 | 做法 | 结论 |
|------|------|------|
| **A（采用）** | 针对性扩展 `domain` 三处结构，适配器按 `mode` 分支解析，详情页按字段非零动态渲染 | 类型安全；复用现有渲染；新字段对其他 provider 零值无害 |
| B | `ProviderUsage` 加 `Raw map[string]any`，详情页 `if provider=="sub2api"` 特判 | 破坏类型安全，详情页塞 provider 分支，弃 |
| C | 抽象通用 `Subscription`/`Quota` 结构供所有 provider | 过度设计，超范围，弃 |

**核心洞察**：sub2api 的 `rate_limits`(5h/1d/7d) 与订阅的日/周/月限额，本质都是"**带重置时间的金额配额窗口**"，与 GLM 的 5h token 窗口语义同构。把它们统一映射成 `UsageDimension`，即可零改动复用 fleetboard 的多窗口排序、进度条染色、最近窗口优先机制。

### 6.3 domain 扩展（`internal/core/domain/provider_usage.go`）

全部为**新增字段**，零值对其他 provider 无害：

**`UsageDimension`** —— 增加金额配额字段（区别于 token 语义的 int64 `Limit`/`Used`）：
```go
// 金额型配额窗口（USD）：sub2api 的 rate_limits 与订阅日/周/月限额。
// 非零时 renderDimension 走金额配额分支；token 型 provider 不填，零值跳过。
MoneyLimit float64
MoneyUsed  float64
// 金额剩余复用既有 Balance 字段（Balance = MoneyLimit - MoneyUsed）。
```

**`RecentUsage`** —— 增加今日/累计统计：
```go
TodayCost      float64
TotalCost      float64
TodayTokens    int64
TotalTokens    int64
TodayRequests  int64
TotalRequests  int64
AvgDurationMs  int64
// 既有 Window7d/Window30d/RPM/TPM/Currency 保留。
```

**`ProviderUsage`** —— 增加 API Key 状态与过期：
```go
APIKeyStatus   string     // sub2api status（"active"/"quota_exhausted"/...）
ExpiresAt      *time.Time // API Key / 订阅过期时间
DaysUntilExpiry int       // 剩余天数
// 既有 PlanLevel 复用承载 planName。
```

### 6.4 sub2api 适配器重写（`internal/adapters/providers/sub2api/sub2api.go`）

**鉴权与请求不变**（Bearer + `/v1/usage`）。`apiResp` 替换为完整结构：
```go
type apiResp struct {
    Mode            string             `json:"mode"`              // quota_limited | unrestricted
    IsValid         bool               `json:"isValid"`
    Status          string             `json:"status"`
    PlanName        string             `json:"planName"`
    Remaining       float64            `json:"remaining"`
    Unit            string             `json:"unit"`
    Balance         float64            `json:"balance"`           // 钱包模式
    Quota           *quotaResp         `json:"quota"`             // 配额模式
    RateLimits      []rateLimitResp    `json:"rate_limits"`
    Subscription    *subscriptionResp  `json:"subscription"`      // 订阅模式
    Usage           *usageResp         `json:"usage"`
    ExpiresAt       *time.Time         `json:"expires_at"`
    DaysUntilExpiry *int               `json:"days_until_expiry"`
}
// 各子结构（quotaResp{Limit,Used,Remaining,Unit}、rateLimitResp{Window,Limit,Used,Remaining,WindowStart,ResetAt}、
// subscriptionResp{Daily/Weekly/Monthly UsageUSD/LimitUSD, WeeklyWindowStart, ExpiresAt}、
// usageResp{Today/Total: {Requests,InputTokens,OutputTokens,CacheCreationTokens,CacheReadTokens,TotalTokens,Cost,ActualCost}, AverageDurationMs, Rpm, Tpm}）
```

**`FetchUsage` 按 `mode` 分支构造 `ProviderUsage`**：

1. **Primary 余额维度**（`Dimensions[0]`，列表显示用的"归一剩余"）：
   - 钱包：`Balance = r.Balance`
   - 配额：`Balance = r.Quota.Remaining`
   - 订阅：`Balance = r.Remaining`（已由服务端按日/周/月最小剩余算好）
   - 统一 `Currency = "USD"`、`Name = nameAvailable`、`PercentUsed = -1`、`Source = "sub2api"`。
   - **Primary 始终是纯余额维度**（列表只需一个剩余数字）；任何带 limit/used 的配额明细都在下面的金额配额维度展开。`Primary = &Dimensions[0]`。

2. **金额配额维度**（`Dimensions[1:]`，复用 `UsageDimension`，仅当有 limit/used 才追加）：
   - 配额模式：先追加总额度维度 `Dimension{Name: "Total quota", MoneyLimit: r.Quota.Limit, MoneyUsed: r.Quota.Used, Balance: r.Quota.Remaining, PercentUsed: used/limit*100, Currency: "USD"}`；再追加每个 `rate_limits` 项 `Dimension{Name: window+" window"（"5h window"）, MoneyLimit, MoneyUsed, Balance: remaining, PercentUsed: used/limit*100, ResetsAt: reset_at, Currency: "USD", Source: "sub2api"}`。
   - 订阅模式：日/周/月限额（`*_limit_usd > 0` 才加）→ `Dimension{Name: "Daily/Weekly/Monthly limit", MoneyLimit: *_limit_usd, MoneyUsed: *_usage_usd, Balance: limit-usage, PercentUsed, Currency: "USD"}`。
   - 钱包模式：无额外配额维度（仅 Primary 余额）。
   - 列表 `displayDimension` 会自动把带 `ResetsAt` 的速率窗口顶到首位（复用 GLM 最近窗口优先），配额型账号列表即显示最紧迫窗口剩余 + 进度条染色。

3. **用量统计**（`Recent`）：`r.Usage` 非空时填 `RecentUsage{TodayCost: today.Cost, TotalCost: total.Cost, TodayTokens: today.TotalTokens, TotalTokens: total.TotalTokens, TodayRequests: today.Requests, TotalRequests: total.Requests, RPM: r.Usage.Rpm, TPM: r.Usage.Tpm, AvgDurationMs: r.Usage.AverageDurationMs, Currency: "USD"}`。

4. **Basic Info / 状态**：`PlanLevel = r.PlanName`、`APIKeyStatus = r.Status`、`ExpiresAt = r.ExpiresAt`、`DaysUntilExpiry = r.DaysUntilExpiry`（解引用）。

删除旧 `apiResp.Used` 假设字段。

### 6.5 详情页渲染（`internal/adapters/ui/account_details.go`）

**`renderDimension` 增加金额配额分支**（在现有余额分支之后、token 配额分支之前判断）：
- 条件：`d.MoneyLimit > 0`
- 渲染：`<Name>` 加粗 + 一行 `$<MoneyUsed> / $<MoneyLimit> (<PercentUsed>%)` + 进度条（复用 `renderBar(d.PercentUsed, N)` + `StatusColor` 染色）+ 若 `ResetsAt` 非零显示重置倒计时（复用现有配额型的 reset 渲染）。
- 金额用 `formatMoney`（2 位小数）。

**`renderRecent` 扩展**（在现有 7d/30d/RPM/TPM 之后追加，非零才显）：
- 今日/累计：`Today: $<TodayCost> · <TodayTokens> tok · <TodayRequests> req` / `Total: $<TotalCost> · <TotalTokens> tok`
- 平均耗时：`Avg <AvgDurationMs>ms`（若 >0）

**Basic Info 区块追加**（`account_details.go:73-86`，非零/非空才显）：
- `API Key` 行：`APIKeyStatus`（用状态色：active 绿 / quota_exhausted·expired 黄·红，可简单映射或复用现有色）。
- `Expires` 行：`ExpiresAt` 本地时间 + `(<DaysUntilExpiry>d left)`。

**Plan 行**（`:81`）现状 `firstNonEmpty(PlanLevel, Model, "—")` 无需改，sub2api 填了 `PlanLevel = planName` 后自动显示套餐名。

### 6.6 字段映射总表

| sub2api 响应 | domain 落点 | 详情页位置 |
|---|---|---|
| `balance` / `quota.remaining` / `remaining` | `Primary.Balance` | 余额维度行 |
| `quota{limit,used}` | `Dimensions[]`（"Total quota" 金额配额维度） | 配额维度区块 |
| `rate_limits[]`(5h/1d/7d) | `Dimensions[]` | 配额维度区块 |
| 订阅日/周/月 `*_limit_usd`/`*_usage_usd` | `Dimensions[]` | 配额维度区块 |
| `usage.today/total.cost` | `Recent.TodayCost/TotalCost` | Usage (recent) 区块 |
| `usage.today/total.total_tokens` | `Recent.Today/TotalTokens` | Usage (recent) 区块 |
| `usage.today/total.requests` | `Recent.Today/TotalRequests` | Usage (recent) 区块 |
| `usage.rpm/tpm` | `Recent.RPM/TPM` | Usage (recent) 区块 |
| `usage.average_duration_ms` | `Recent.AvgDurationMs` | Usage (recent) 区块 |
| `planName` | `PlanLevel` | Basic Info · Plan 行 |
| `status` | `APIKeyStatus` | Basic Info · API Key 行 |
| `expires_at` / `days_until_expiry` | `ExpiresAt` / `DaysUntilExpiry` | Basic Info · Expires 行 |

## 7. 测试策略

### 7.1 单元测试
- **`formatMoneyShort`**（`account_list_test.go`）：覆盖 `<1000`/`≥1000`/负值/CNY·USD，断言 2 位小数与缩写。
- **sub2api 适配器**（`sub2api_test.go`）：三组金样本响应（quota_limited / unrestricted-subscription / unrestricted-wallet，字段值参考 sub2api 源码与 `gateway_handler_usage_test.go`），断言：
  - Primary 余额正确归一（钱包=balance、配额=quota.remaining、订阅=remaining）。
  - Dimensions 数量与字段（配额模式有 5h/1d/7d 窗口维度；订阅模式有日/周/月维度）。
  - Recent 的 today/total/rpm/tpm 正确填充。
  - PlanLevel/APIKeyStatus/ExpiresAt/DaysUntilExpiry 正确。
  - 余额维度 `Currency == "USD"`、`PercentUsed == -1`。
- **`renderDimension` 金额分支**（`account_details_test.go`）：`MoneyLimit>0` 时输出含 `$used / $limit (xx%)` 与进度条。
- **`renderRecent` 扩展**：today/total/avg 非零时出现，零值时不出。

### 7.2 手动验证（TUI 交互）
- 删除确认：显示名称+provider chip；`d` 删除、`c`/`ESC` 取消；Cancel 默认聚焦；颜色正确。
- sub2api 三模式账号各接入一个，确认详情页按模式动态显示、缺失字段隐藏、余额不再错误显示 0。
- 列表金额 2 位小数、列对齐无错位。

## 8. 实现顺序

| 步骤 | 内容 | 依赖 |
|------|------|------|
| 1 | 需求① `formatMoneyShort` 改 `%.2f` + 测试 | 无 |
| 2 | 需求② `confirmDelete` 重写 + 手动验证 | 无 |
| 3 | 需求③a domain 三处加字段 | 无 |
| 4 | 需求③b sub2api 适配器重写 + 三模式测试 | ③a |
| 5 | 需求③c 详情页 `renderDimension`/`renderRecent`/Basic Info 扩展 + 测试 | ③a、③b |

步骤 1、2 与 3 相互独立，可并行；4、5 依赖 3。

## 9. 兼容性与风险

- **domain 加字段**：纯新增，其他 provider 不填即为零值，渲染分支以 `>0`/`!=nil`/`!=""` 判断跳过，行为不变。
- **sub2api 行为变化（bugfix 性质）**：配额型与订阅型账号此前余额错显为 0，改后正确显示。属修复，但需在 changelog 注明 sub2api 显示行为变化。
- **`apiResp` 结构不向后兼容旧契约**：旧 `{balance,used}` 假设被完整结构替代。钱包模式仍兼容（顶层 `balance` 保留解码）。`used` 假设字段删除。
- **删除按钮顺序变化**：`[Delete, Cancel]` → `[Cancel, Delete]`，肌肉记忆改变，但更安全（Cancel 默认聚焦）。已在需求②说明。
- **sub2api 版本漂移**：本契约基于 sub2api `main` 分支核实。若用户实例为旧版且字段不同，适配器按零值容错（缺失字段不显示），不会崩溃。

## 10. 参考来源

- [Wei-Shaw/sub2api — `gateway_handler.go`（`/v1/usage` 的 `Usage`/`usageQuotaLimited`/`usageUnrestricted`/`buildUsageData`）](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/handler/gateway_handler.go)
- [Wei-Shaw/sub2api — `api_key_auth.go`（鉴权中间件，`/v1/usage` 跳过计费）](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/server/middleware/api_key_auth.go)
- [KonataAPI — sub2api 余额查询工具（确认 `/v1/usage` 接口与日志需 JWT）](https://github.com/xiaopenghuang/KonataAPI)
- fleetboard 同族先例：`docs/superpowers/specs/2026-07-28-newapi-native-balance-design.md`（newapi 原生余额对接）
