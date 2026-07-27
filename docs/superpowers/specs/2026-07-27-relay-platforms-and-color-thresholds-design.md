# 中转平台接入 + 颜色阈值可配置 + 列表高亮撑满 设计规格

> 状态：已确认 · 日期：2026-07-27 · 作者：MeePwn
> 范围：3 项优化（① account 列表选中高亮撑满整行 padding→margin；② 进度条/状态点颜色阈值可配置；③ 新增 sub2api / new-api 两个中转平台余额显示）。
> 参考：现有 `deepseek` provider 模板、`optimizations-design.md`（上一批，不重叠）。

## 0. 决策摘要（已与用户确认）

| 议题 | 决策 |
|------|------|
| 需求 1 高亮撑满 | 去掉 `AccountList` 左右内部 padding，让选中高亮顶满边框；视觉呼吸由 Flex 3:2 比例自然提供，不引入额外 margin 容器 |
| 需求 2 配置载体 | 代码内置默认阈值 + `config.yaml` 的 `ui.colors` 段可选覆盖（nil 回退默认） |
| 需求 2 YAML 风格 | `thresholds`（边界数组）+ `colors`（颜色数组，比 thresholds 多 1 个兜底）；配额升序、余额降序 |
| 需求 3 new-api 接口 | `GET /v1/dashboard/billing/subscription`（总额）+ `/v1/dashboard/billing/usage`（已用），OpenAI 兼容、单凭证 sk-key、用户级余额 |
| 需求 3 sub2api 接口 | `GET /v1/usage`，Bearer sk-key，返回余额（USD，可为负） |
| 需求 3 Account 模型 | **不改**——两平台都用 `BaseURL`+`TokenEnv` 单凭证，契合现有字段 |

执行顺序：需求 1（布局，机械改动 + golden）→ 需求 2（颜色收口 + 配置，影响列表/详情两处）→ 需求 3（两个 provider + 装配 + 测试）。每步 `go build ./...` + `make test` 绿。

## 1. 接口调研结论

两平台均为 OpenAI 兼容 AI 网关、自部署（BaseURL 必填）、单凭证查询：

| 平台 | 接口 | 鉴权 | 余额换算 | 类型 |
|------|------|------|---------|------|
| new-api | `GET {BaseURL}/v1/dashboard/billing/subscription` + `/v1/dashboard/billing/usage` | `Authorization: Bearer <sk-key>` | `system_hard_limit_usd − total_usage` | 余额型 USD |
| sub2api | `GET {BaseURL}/v1/usage` | `Authorization: Bearer <sk-key>` | 响应直接含余额（USD，可为负） | 余额型 USD |

- **为何不用 `/api/user/self`**：new-api 该接口需额外 `New-Api-User: <用户ID>` header（两份凭证），会迫使 `Account` 加字段；OpenAI 兼容的 billing 接口仅需 sk-key，对监控用户最友好。
- **sub2api 余额可为负**（官方 Issue #2011：订阅套餐 + 余额并存时能扣到负），颜色阈值须容忍负值。
- **字段实现风险**：`billing/usage` 的 `total_usage` 在 OpenAI 原版单位是美分（/100），new-api/one-api 兼容版通常已是美元；sub2api `/v1/usage` 精确字段社区文档未公开。两者均需在实现/测试时用真实响应校准（spec §5 已列为验证项）。

## 2. 需求 1 — 选中高亮撑满整行（padding→margin）

**根因**：`internal/adapters/ui/account_list.go::build()` 中 `SetBorderPadding(0,0,1,1)` 让 List 内容左右各缩进 1 格；`SetHighlightFullLine(true)` 填充的是内容区（padding 之内），故选中蓝色高亮两端各空 1 格、未顶到边框。

**改法**：

1. `account_list.go`：`SetBorderPadding(0, 0, 1, 1)` → `SetBorderPadding(0, 0, 0, 0)`，高亮顶满左右边框。
2. 行首视觉缩进由 `formatAccountLine` 的 `pin` 占位（已有 `  `/📌，显示宽 2）提供，内容不贴左边框；行尾 `    Last Refreshed` 已自带 4 空格，不贴右边框。
3. 不引入额外 margin 容器：tview 无原生 margin，嵌套 Box 徒增复杂度；List 与 details 的呼吸感由 `buildLayout` 的 Flex 3:2 列比已自然提供（YAGNI）。
4. 同步更新 `account_list_test.go` 的 golden 输出（每行行首少 1 空格）。
5. **真机验证**（用户在 Termux/OPPO Pad 跑）：确认选中行蓝色高亮确实顶满左右边框；若 tview 版本下 `SetBorderPadding(0,0,0,0)` 仍不足以撑满，备选 `SetBackgroundColor` 配合 `SetHighlightFullLine` 调试。

> 注意：`AccountDetails` 与 `SearchBar` 同样用了 `SetBorderPadding(0,0,1,1)`，但用户仅要求 account 列表条目；本次不改动它们，保持改动聚焦（如后续需全局一致可再开任务）。

## 3. 需求 2 — 颜色阈值可配置（代码默认 + config 覆盖）

### 3.1 收口：新增 `BalanceColor`，与 `StatusColor` 对称

当前余额型点色逻辑散在 `account_list.go::formatAccountLine`（硬编码 `Balance>0→绿 / ≤0→红`）。先抽到 `theme.go`：

```go
// BalanceColor 按余额数值选色（余额越低越危险，降序阈值）。
func BalanceColor(balance float64, currency string) string
```

`formatAccountLine` 的余额分支改为调用 `BalanceColor(d.Balance, d.Currency)`，消除散落逻辑。`renderBar` 对余额型本就不画条（`PercentUsed=-1`→灰条），不受影响。

### 3.2 配置结构（`domain.UIConfig` 扩展）

```go
// config.go
type UIConfig struct {
    Theme  string       `yaml:"theme"`     // tokyo-night
    Colors ColorsConfig `yaml:"colors"`    // 新增，零值→代码默认
}

type ColorsConfig struct {
    Quota   ThresholdColors `yaml:"quota"`   // 配额型（百分比，升序）
    Balance ThresholdColors `yaml:"balance"` // 余额型（数值，降序）
}

// ThresholdColors：thresholds 为边界数组，colors 比 thresholds 多 1 个（末尾兜底）。
// 配额型 thresholds 升序、余额型降序；方向由调用方决定。
type ThresholdColors struct {
    Thresholds []float64 `yaml:"thresholds"`
    Colors     []string  `yaml:"colors"`     // 预设名(green/yellow/red/...)或 #RRGGBB
}
```

### 3.3 YAML 形态（用户可见）

```yaml
ui:
  colors:
    quota:                         # 配额型：用量百分比升序分档
      thresholds: [70, 90]         #   <70 绿 / 70-90 黄 / >=90 红（即当前默认）
      colors:     [green, yellow, red]
    balance:                       # 余额型：余额数值降序分档（支持负值）
      thresholds: [10, 1]          #   >=10 绿 / >=1 黄 / <1 红（默认）
      colors:     [green, yellow, red]
```

### 3.4 解析与校验（`theme.go` 或新建 `color_config.go`）

- `defaultQuota = {Thresholds:[70,90], Colors:[green,yellow,red]}`、`defaultBalance = {Thresholds:[10,1], Colors:[green,yellow,red]}`。
- 解析函数 `resolveColors(cfg ColorsConfig) (quota, balance ThresholdColors)`：
  - 任一档 `len(Colors) != len(Thresholds)+1` 或 `len(Thresholds)==0` → 该档回退默认。
  - 颜色项：预设名（`green/yellow/red/gray/purple/cyan/blue/accent/primary/secondary`）映射到 `const.go` 调色板；否则按 `#RRGGBB` 原样；非法值 → 回退该档默认。
- 选色函数（方向感知）：
  - `pickByQuota(tc, pct)`：升序——找第一个 `thresholds[i] > pct`，返回 `colors[i]`；都未超过返回 `colors[len-1]`。
  - `pickByBalance(tc, balance)`：降序——找第一个 `thresholds[i] <= balance`，返回 `colors[i]`；都低于返回 `colors[len-1]`。
  - `pct<0`（N/A）固定灰，与现在一致。

### 3.5 注入方式（最小侵入，沿用全局风格）

`StatusColor`/`BalanceColor` 现为包级函数，被 `account_list.go`、`account_details.go` 多处调用。采用与 `initializeTheme` 设置全局 `tview.Styles` 一致的全局态：

- 包级变量 `var activeColors = resolvedDefaults`。
- `TUI.Run()`/`buildComponents()` 后调用 `applyColorScheme(cfg.UI.Colors)` 设置之（main 把 `cfg.UI` 透传给 `ui.Config`）。
- `StatusColor`/`BalanceColor` 读 `activeColors` 选色；测试用 `applyColorScheme(ColorsConfig{})` 重置为默认，保证可复现。
- `cmd/main.go`：`ui.Config` 增 `UIConfig domain.UIConfig` 字段，`NewTUI` 透传。

效果：列表状态点、详情进度条、详情余额行三处颜色同源，改一处配置全局生效。

### 3.6 受影响测试

- `theme_test.go`：`StatusColor` 默认行为不变（回归）；新增 `BalanceColor`、`resolveColors`（合法/非法/缺省）、`pickByQuota/pickByBalance`（含负值边界）用例。
- `account_list_test.go` golden：余额型点色若默认阈值与旧"正/负"二档一致则不变；确认默认 `[10,1]` 下 `Balance>0` 仍多数为绿（除非 <1）。

## 4. 需求 3 — 新增 sub2api / new-api provider

各仿 `internal/adapters/providers/deepseek/deepseek.go` 模板。

### 4.1 sub2api（`internal/adapters/providers/sub2api/sub2api.go`）

- `Provider()` → `"sub2api"`。
- `FetchUsage`：`GET {base}/v1/usage`，`Authorization: Bearer <TokenEnv>`。
- 响应解析（字段待真实响应校准，先按社区惯例 `balance`/`used` 假设，容忍嵌套 `data.*`）：
  ```go
  type apiResp struct {
      Balance float64 `json:"balance"`   // USD，可为负
      Used    float64 `json:"used"`
  }
  ```
- 填 `Dimensions=[{Name:"Available balance", Balance, Currency:"USD", PercentUsed:-1, Source:"sub2api"}]`，`Primary` 指向之。
- `BaseURL` 为空时 **报错**（自部署，无默认）：`Err = "sub2api: base_url is required"`。
- `theme.go::providerColor` 加 `"sub2api": {"#8B5CF6", "#FFFFFF"}`（紫，与 glm 区分）。

### 4.2 new-api（`internal/adapters/providers/newapi/newapi.go`）

- `Provider()` → `"newapi"`（包名 `newapi`，slug `newapi`；避免连字符）。
- 两次请求：
  1. `GET {base}/v1/dashboard/billing/subscription` → `system_hard_limit_usd`（总额，回退 `hard_limit_usd`）。
  2. `GET {base}/v1/dashboard/billing/usage` → `total_usage`（已用）。
- 余额 = 总额 − 已用；`Dimensions=[{Name:"Available balance", Balance, Currency:"USD", PercentUsed:-1, Source:"newapi"}]`。
- `total_usage` 单位：new-api/one-api 兼容版通常已是美元（区别于 OpenAI 原版的美分 cents）；**实现时用真实响应确认**，不在运行时猜测换算（spec §5 验证项）。
- `BaseURL` 为空报错（同上）。
- `theme.go::providerColor` 加 `"newapi": {"#10B981", "#FFFFFF"}`（翠绿）。

### 4.3 装配（两处）

- `cmd/main.go`：`providers.NewRegistry(glm.New(), minimax.New(), kimi.New(), deepseek.New(), sub2api.New(), newapi.New())`。
- `account_form.go`：`providerOptions = []string{"glm","minimax","kimi","deepseek","sub2api","newapi"}`。

### 4.4 测试（TDD，httptest mock）

- `sub2api/sub2api_test.go`：mock `/v1/usage` 返回余额（含负值场景），断言 `Balance`/`Currency`/`PercentUsed=-1`；`base_url` 缺失分支。
- `newapi/newapi_test.go`：mock 两个 billing 端点，断言余额=差；测总额/已用字段回退（`system_hard_limit_usd` 缺失→`hard_limit_usd`）；`base_url` 缺失分支。

## 5. 验证

- 每步 `go build ./...` 通过；终态 `make test`（`go test -race -cover ./...`）全绿，`make quality`（gofumpt + go vet）通过。
- 需求 1：`account_list_test.go` golden 更新；真机确认高亮撑满。
- 需求 2：`theme_test.go` 新增解析/选色用例（含负余额、非法颜色、缺省回退）；默认配置下行为与现状一致（回归保护）。
- 需求 3：两个 provider `*_test.go`；**字段校准**——若有真实 new-api/sub2api 实例，用 `curl -H "Authorization: Bearer <sk>" <url>` 比对响应字段，修正 JSON tag。
- README：在 supported providers 列表补 sub2api / new-api（如有该节）。

## 6. 非目标

- 不改 `Account` 模型（两平台单凭证够用）。
- 不动 `AccountDetails`/`SearchBar` 的 padding（仅 account 列表）。
- 不支持 new-api 的 `/api/user/self`（两份凭证）路径。
- 不做颜色配置的 UI 内编辑（仅 config.yaml）。
- 不为 sub2api 的订阅套餐限额（daily/weekly/monthly_limit_usd）单独建模——本期只取余额维度。
