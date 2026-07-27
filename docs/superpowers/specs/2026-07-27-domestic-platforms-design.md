# 国内其他平台接入设计规格

> 状态：草案 v1 · 日期：2026-07-27 · 作者：MeePwn
> 关联：`docs/superpowers/specs/2026-07-27-fleetboard-design.md`（主设计，§4 P3 阶段「国内其他厂商 adapter」）

## 1. 背景与目标

fleetboard 已对接 GLM、MiniMax 两家**配额型**厂商（接口返回已用/上限/百分比/重置时间）。本期接入**国内其他 AI 平台**，扩展监控覆盖面。

接入新平台前，对候选 4 家（Kimi、DeepSeek、阿里百炼、火山方舟）做了真实接口契约调研，结论是它们的可行性差异巨大，分三个梯队（见 §2）。**本期范围聚焦第一梯队：Kimi + DeepSeek**；火山方舟（需签名 SDK）、阿里百炼（接口不存在且 ToS 禁止）留待后续单独迭代。

本期同时引入一个**架构概念：「余额型」vendor** —— Kimi/DeepSeek 的接口只返回账户余额（剩余），没有已用/上限/百分比/重置时间。这与现有「配额型」模型不同，需要在数据模型与 UI 层做最小适配，使余额型账户在列表与详情页都能正确、诚实地展示。

### 目标
- 新增 Kimi、DeepSeek 两个 `UsageProvider` adapter，各自一个 GET + Bearer 鉴权
- 在 `UsageDimension` 增加「余额」表达，让余额型与配额型共用同一展示管线
- 列表/详情页对余额型有专门渲染（余额数字 + 货币 + 健康色点），不硬凑百分比
- 完整 httptest golden test 覆盖两家响应解析

### 非目标（本期不做）
- 火山方舟（AK/SK + SignerV4 签名、双密钥、多接口组合）—— 复杂度高，后续单独迭代
- 阿里百炼（Coding/Token Plan 用量无公开 API，且官方 ToS 明确禁止脚本化调用，违规可能封号）—— 不接入
- 余额型账户的「已用百分比」推导（如本地基线追踪）—— YAGNI，余额型只展示绝对余额
- 历史趋势、余额变动图表

## 2. 候选平台调研结论（范围决策依据）

| 平台 | 公开用量 API | 鉴权 | 复杂度 | 数据语义 | 结论 |
|------|------------|------|--------|---------|------|
| **Kimi / Moonshot** | ✅ `GET api.moonshot.cn/v1/users/me/balance` | `Bearer token` | 简单（≈minimax） | 余额（剩余），无百分比 | 🟢 本期接入 |
| **DeepSeek** | ✅ `GET api.deepseek.com/user/balance` | `Bearer token` | 简单 | 余额（剩余），无百分比 | 🟢 本期接入 |
| 火山方舟 / 豆包 | ✅ 管控面 OpenAPI | AK/SK + SignerV4 签名 | 复杂 | 已用 token + 套餐额度 | 🟡 后续迭代 |
| 阿里百炼 / 通义 | ❌ Coding/Token Plan 用量无公开 API | — | 不可行 | — | 🔴 不接入（ToS 禁止） |

调研要点：
- Kimi 的 **Coding Plan 订阅用量**（5h 滚动 / 周额度）**无公开 HTTP API**，只能在网页/CLI `/usage` 查看；本期能拿到的是 **API 余额**（`/v1/users/me/balance`）。
- DeepSeek 是纯预充值余额制，**无 OpenAI 风格 usage 接口、无重置周期**，官方只暴露 `/user/balance`。
- 两家都是单 Bearer token、一个 GET，与现有 minimax 同级，落地成本低。

## 3. 数据模型扩展

在 `internal/core/domain/vendor_usage.go` 的 `UsageDimension` 增加两个字段。**配额型零值，完全不受影响**：

```go
type UsageDimension struct {
    // ...现有字段（Name/Used/Limit/PercentUsed/Remaining/ResetsAt/Unit/Source）不变...
    Balance  float64 // 余额型 vendor 的当前余额（元/美元）；配额型零值
    Currency string  // 余额型货币："CNY"/"USD"；配额型空 ← 用 Currency!="" 判断余额型
}
```

**设计约定**：
- **余额型判断依据是 `Currency != ""`**，而非 `Balance > 0`。余额可能为 0（耗尽）或负（Kimi `cash_balance` 欠费时可为负），但仍是余额型。
- 余额型 adapter **不调用 `SelectPrimary()`**（后者取「PercentUsed 最大那档」，余额型无此语义）。改为显式 `u.Primary = &u.Dimensions[0]`，指向「可用余额」维度。
- 余额型维度 `PercentUsed = -1`（N/A），`Limit = 0`，`ResetsAt` 零值。

## 4. Adapter 设计

两家 adapter 结构对齐现有 glm/minimax：`Provider{hc *http.Client}` + `New()` + `Vendor()` + `FetchUsage()` + 编译期断言 `var _ ports.UsageProvider = (*Provider)(nil)`。文件头注释写明真实接口契约与易错点（项目惯例）。

### 4.1 Kimi（`internal/adapters/providers/kimi/kimi.go`）

**接口契约**（官方文档 `platform.kimi.com/docs/api/balance`）：
- `GET {BaseURL}/v1/users/me/balance`
- 默认 BaseURL = `https://api.moonshot.cn`（国内，CNY）；国际版可覆盖为 `https://api.moonshot.ai`（USD）；`acc.BaseURL` 可覆盖。
- 鉴权头：`Authorization: Bearer <MOONSHOT_API_KEY>` —— **必须带 "Bearer " 前缀**。

**响应结构**（成功态）：
```json
{
  "code": 0,
  "data": {
    "available_balance": 49.58894,
    "voucher_balance": 46.58893,
    "cash_balance": 3.00001
  },
  "scode": "0x0",
  "status": true
}
```

**易错点**：
1. **成功判 `code == 0`**（业务码），不能只看 HTTP 200。
2. **错误响应结构不同**：错误态是 OpenAI 风格 `{error:{message, type, code}}`，与成功态的 `{code, data, status}` 完全不同 —— 解析需分支处理。
3. **货币随 base URL**：`.cn` → CNY（¥），`.ai` → USD（$）。adapter 据 base URL 推断 currency。

**维度映射**：
- 主维度「可用余额」：`Balance = data.available_balance`，`Currency` 按 base URL 推断，`PercentUsed = -1`。设为 `Primary`。
- 子维度（可选，详情页展开）：「现金余额」`Balance = cash_balance`、「代金券余额」`Balance = voucher_balance`，同 Currency。

### 4.2 DeepSeek（`internal/adapters/providers/deepseek/deepseek.go`）

**接口契约**（官方文档 `api-docs.deepseek.com/api/get-user-balance`）：
- `GET {BaseURL}/user/balance`
- 默认 BaseURL = `https://api.deepseek.com`；`acc.BaseURL` 可覆盖。
- 鉴权头：`Authorization: Bearer <API_KEY>`。

**响应结构**：
```json
{
  "is_available": true,
  "balance_infos": [
    {
      "currency": "CNY",
      "total_balance": "110.00",
      "granted_balance": "10.00",
      "topped_up_balance": "100.00"
    }
  ]
}
```

**易错点**：
1. **所有金额字段是 string**（如 `"110.00"`），不是 number —— 必须 `strconv.ParseFloat`。**不需要 /100 换算**（单位本就是元/美元，非 cents）。
2. **`balance_infos` 是数组**：单账户一般一条，按 `currency`；adapter 取首项（或按需过滤），从中读 `currency`、`total_balance`。
3. **无「已用」字段**：三个金额都是「剩余」语义。`total_balance = granted + topped_up`。
4. **无重置周期**：余额扣减制，`ResetsAt` 零值。

**维度映射**：
- 主维度「可用余额」：`Balance = parse(total_balance)`，`Currency = balance_infos[0].currency`，`PercentUsed = -1`。设为 `Primary`。
- 子维度（可选）：「赠金余额」`granted_balance`、「充值余额」`topped_up_balance`。
- `is_available` 可作为健康状态辅助（但 dot 颜色主要看 `Balance > 0`）。

## 5. 余额型 UI 适配（2 处改动）

现有 UI 对余额型的「不改则坏」：
- 列表：`Primary` 若为余额型且 `PercentUsed=-1`，会显示 `"-1%"`（错）；若 `Primary=nil` 显示灰色 `"N/A ○"`（像故障，其实账户健康）。
- 详情：`renderDimension` 因 `Limit=0`（第 172 行）会**跳过余额数字**，只画灰条 N/A —— 余额型最该显示的余额反而看不到。

### 5.1 列表行（`account_list.go` → `formatAccountLine`）

余额型（`Primary != nil && Primary.Currency != ""`）分支：
- `pctStr` → 余额短格式：`formatMoneyShort(Balance, Currency)`，如 `¥49.6` / `$3.0`（1 位小数，>1000 用 `¥1.2k`）
- `dot = "●"`
- `dotCol`：`Balance > 0` → 绿（`colorGreen`）；`Balance <= 0` → 红（`colorRed`，欠费/耗尽）
- `miniBar`：复用 `renderBar(-1, 4)` → 全灰空心条（与配额型 N/A 一致，语义=无进度，且保持列对齐）

```
配额型:  K 智谱编码-主力     glm   ▓▒░░ 45% ●    5m ago
余额型:  K Kimi-主力         kimi  ░░░░ ¥49.6 ●   5m ago
         DeepSeek-备用    deepseek ░░░░ $3.0 ●    3m ago
```

### 5.2 详情维度（`account_details.go` → `renderDimension`）

余额型（`dim.Currency != ""`）分支：
- **不画进度条**（余额无进度语义），改为显示 `Balance:` 行：`formatMoney(Balance, Currency)`（2 位小数，如 `¥49.58`）
- 不显示 `Used/Limit/Remaining`（余额型无此概念）
- 不显示 `Resets:`（余额型无重置）
- 维度名照常显示（如「可用余额」）

新增 helper：
- `formatMoney(balance float64, currency string) string`：详情用，2 位小数。`CNY`→`¥%.2f`，`USD`→`$%.2f`，其他→`%.2f <code>`。
- `formatMoneyShort(...)`：列表用，1 位小数 + >1000 缩写。

## 6. 触点清单（共 8 处）

**新增**：
1. `internal/adapters/providers/kimi/kimi.go` + `kimi_test.go`
2. `internal/adapters/providers/deepseek/deepseek.go` + `deepseek_test.go`

**改动**：
3. `internal/core/domain/vendor_usage.go` — `UsageDimension` 加 `Balance` + `Currency`
4. `internal/adapters/ui/account_list.go` — `formatAccountLine` 余额型分支
5. `internal/adapters/ui/account_details.go` — `renderDimension` 余额型分支 + `formatMoney` helper
6. `internal/adapters/ui/account_form.go` — `vendorOptions` 加 `"kimi"`, `"deepseek"`
7. `internal/adapters/ui/theme.go` — `vendorColor` 加 `"deepseek": {"#2563EB","#FFFFFF"}`（kimi 已有 `#06B6D4`）；同步主 spec §9.2 配色清单
8. `cmd/main.go` — `NewRegistry(glm.New(), minimax.New(), kimi.New(), deepseek.New())`

## 7. 配色

`theme.go` `vendorColor` 新增：
- `"deepseek": {"#2563EB", "#FFFFFF"}`（深蓝，与 DeepSeek 鲸鱼 logo 品牌色基调一致，且与 kimi 青 `#06B6D4` 区分明显）

Kimi 已在 `vendorColor` 预登记（`#06B6D4` 青），无需改动。

> 注：`theme_test.go` 校验 `vendorColor` 每项匹配主 spec §9.2。新增 deepseek 需在主 spec §9.2 配色清单补一行，避免测试失败。

## 8. 测试策略

沿用主 spec §12（表驱动 + golden，httptest.Server mock）：
- **kimi_test.go**：mock 成功响应（`code=0` + 三余额）→ 断言维度映射、Currency 按 base URL 推断（`.cn`→CNY / `.ai`→USD）、Primary 指向可用余额；mock 错误响应（`{error:{message}}`）→ 断言 `Err` 非空且 VendorUsage 仍带账号字段。
- **deepseek_test.go**：mock string 金额响应 → 断言 ParseFloat 正确、无 /100 换算、Currency 取自 `balance_infos[0].currency`、Primary 指向可用余额。
- **UI 测试**：`formatAccountLine` / `renderDimension` 对余额型 fixture 的渲染输出断言（余额数字、绿/红点、灰条、不画进度条），沿用现有 `account_list_test.go` / `account_details_test.go` 风格。

## 9. 配置示例

新增两家后，`~/.fleetboard/config.yaml`：
```yaml
accounts:
  - id: kimi-main
    vendor: kimi
    label: Kimi-主力
    token_env: MOONSHOT_API_KEY
    # base_url: https://api.moonshot.ai   # 国际版可覆盖（USD）
  - id: deepseek-backup
    vendor: deepseek
    label: DeepSeek-备用
    token_env: DEEPSEEK_API_KEY
```

## 10. 开放问题（实现阶段确认）

1. **Kimi 子维度展开**：详情页是否展开「现金/代金券」子余额，还是只显示「可用余额」单维度？建议先单维度，子维度按需再加。
2. **DeepSeek 多币种**：`balance_infos` 理论可能多条（不同 currency）。本期取首项，多币种场景待实测后补。
3. **DeepSeek 品牌色 `#2563EB`**：实现时若官方有更准的品牌色可微调（与 `theme_test` 同步）。
4. **金额大额缩写阈值**：`formatMoneyShort` 的 `>1000 → 1.2k` 阈值是否合适，实现时可调。
