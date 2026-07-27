# 中转平台接入 + 颜色阈值可配置 + 列表高亮撑满 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 account 列表选中高亮撑满整行、进度条/状态点颜色阈值可通过 config.yaml 配置、并新增 sub2api 与 new-api 两个中转平台的余额显示。

**Architecture:** 六边形架构不变。需求 1 仅调 `AccountList` 的 border padding；需求 2 在 `domain.UIConfig` 增配色结构、在 `ui` 包用 `atomic.Pointer` 持有解析后的配色供 `StatusColor`/`BalanceColor` 读取；需求 3 各仿 `deepseek` 模板新增 provider 并在 `main.go`/`account_form.go` 装配。`Account` 模型不改（两平台单凭证）。

**Tech Stack:** Go 1.24+ · tview/tcell（Tokyo Night 调色板）· httptest（provider 测试）· cobra · module `github.com/maybewaityou/fleetboard`。

## Global Constraints

- 不修改 `domain.Account` 结构（sub2api/new-api 仅用 `BaseURL`+`TokenEnv`）。
- 颜色值支持预设名（`green/yellow/red/gray/purple/cyan/blue/accent/primary/secondary`，映射 `const.go` 调色板）或 `#RRGGBB`；非法→回退默认。
- 默认配色：quota `{thresholds:[70,90], colors:[green,yellow,red]}`；balance `{thresholds:[10,1], colors:[green,yellow,red]}`（余额可为负，降序阈值）。
- provider slug 与包名：`sub2api`、`newapi`（无连字符）。
- 每步 `go build ./...` 绿；每 Task 末 `go test ./...` 绿。语义提交（`feat`/`fix`/`refactor`/`style`/`chore`）。
- `AccountDetails`/`SearchBar` 的 padding 本期不动（仅 account 列表）。

---

## File Structure

- **Modify** `internal/core/domain/config.go` — 增 `ColorsConfig`/`ThresholdColors`，`UIConfig.Colors` 字段。
- **Create** `internal/adapters/ui/color_config.go` — 配色解析、`atomic.Pointer` 全局态、选色函数。
- **Create** `internal/adapters/ui/color_config_test.go` — 解析/选色测试。
- **Modify** `internal/adapters/ui/theme.go` — `StatusColor` 接入配色；新增 `BalanceColor`；`providerColor` 增两条。
- **Modify** `internal/adapters/ui/theme_test.go` — `TestProviderTag_KnownProviders` 增 sub2api/newapi。
- **Modify** `internal/adapters/ui/account_list.go` — padding 归零；余额分支调 `BalanceColor`。
- **Modify** `internal/adapters/ui/account_list_test.go` — 增 padding 契约测试。
- **Modify** `internal/adapters/ui/tui.go` — `Config` 增 `UIConfig` 字段；`Run` 调 `applyColorScheme`。
- **Modify** `cmd/main.go` — `NewRegistry` 注册两 provider；`ui.Config` 透传 `cfg.UI`。
- **Modify** `internal/adapters/ui/account_form.go` — `providerOptions` 增两条。
- **Create** `internal/adapters/providers/sub2api/sub2api.go` + `sub2api_test.go`。
- **Create** `internal/adapters/providers/newapi/newapi.go` + `newapi_test.go`。

---

### Task 1: 选中高亮撑满整行（padding→margin）

**Files:**
- Modify: `internal/adapters/ui/account_list.go`（`build()` 内 `SetBorderPadding`）
- Test: `internal/adapters/ui/account_list_test.go`

**Interfaces:**
- Produces: `AccountList` 的左右 border padding 为 0（`GetBorderPadding()` 返回 left=right=0），选中高亮顶满边框。

- [ ] **Step 1: 写失败测试**

追加到 `account_list_test.go`：

```go
// TestAccountList_NoHorizontalPadding 守护选中高亮撑满整行的契约：List 左右 border
// padding 必须为 0（padding 会把 SetHighlightFullLine 的高亮向内顶缩 1 格）。
// 视觉呼吸由 Flex 3:2 列比提供，不再用内部 padding 占位。
func TestAccountList_NoHorizontalPadding(t *testing.T) {
	al := NewAccountList()
	_, _, left, right := al.GetBorderPadding()
	if left != 0 || right != 0 {
		t.Errorf("horizontal border padding = (left=%d, right=%d), want (0,0) so the selection highlight fills the line", left, right)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/adapters/ui/ -run TestAccountList_NoHorizontalPadding -v`
Expected: FAIL，`left=1, right=1`。

- [ ] **Step 3: 改实现**

`internal/adapters/ui/account_list.go` 的 `build()` 中，把：

```go
		SetBorderPadding(0, 0, 1, 1) // 左右各 1 空格：条目与选中高亮不再紧贴边框
```

改为：

```go
		SetBorderPadding(0, 0, 0, 0) // 左右 padding 归零：选中高亮（SetHighlightFullLine）顶满边框；
		// 行首视觉缩进由 formatAccountLine 的 pin 占位（  /📌，显示宽 2）提供，内容不贴边。
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/adapters/ui/ -run TestAccountList_NoHorizontalPadding -v`
Expected: PASS。

- [ ] **Step 5: 全量回归**

Run: `go test ./...`
Expected: 全绿（`formatAccountLine` 的字符串输出不含 padding，既有 golden 不受影响）。

- [ ] **Step 6: 真机确认（人工，非自动化）**

在 Termux/OPPO Pad 运行 `fleetboard`，确认光标所在行蓝色高亮顶满列表左右边框。若特定 tview 版本下仍不撑满，备选：在 `build()` 的 `SetHighlightFullLine(true)` 之后追加 `al.List.SetBackgroundColor(tcell.GetColor(colorSelected))` 调试（仅作排查手段，默认不改）。

- [ ] **Step 7: 提交**

```bash
git add internal/adapters/ui/account_list.go internal/adapters/ui/account_list_test.go
git commit -m "fix(ui): fill account list selection highlight to the border (padding→0)"
```

---

### Task 2: 颜色配置基础设施（domain 结构 + 解析/选色 + theme 接入）

**Files:**
- Modify: `internal/core/domain/config.go`
- Create: `internal/adapters/ui/color_config.go`
- Create: `internal/adapters/ui/color_config_test.go`
- Modify: `internal/adapters/ui/theme.go`

**Interfaces:**
- Consumes: `domain.ColorsConfig`/`domain.ThresholdColors`（本 Task 定义）。
- Produces: `ui.applyColorScheme(domain.ColorsConfig)`、`ui.StatusColor(float64) string`（改为读配色）、`ui.BalanceColor(balance float64, currency string) string`（新增）。后续 Task 3 的 `formatAccountLine` 与 `tui.Run` 依赖这些签名。

- [ ] **Step 1: 扩展 domain 配置结构**

在 `internal/core/domain/config.go` 的 `UIConfig` 下方追加类型，并把 `Colors` 字段加进 `UIConfig`：

```go
// UIConfig 控制 TUI 表现层。
type UIConfig struct {
	Theme  string       `yaml:"theme"`  // tokyo-night
	Colors ColorsConfig `yaml:"colors"` // 颜色阈值；零值→代码默认
}

// ColorsConfig 持有配额型与余额型两套颜色阈值。
type ColorsConfig struct {
	Quota   ThresholdColors `yaml:"quota"`   // 配额型（百分比，升序阈值）
	Balance ThresholdColors `yaml:"balance"` // 余额型（数值，降序阈值；支持负值）
}

// ThresholdColors：thresholds 为边界数组，colors 比 thresholds 多 1 个（末尾兜底）。
// 配额型 thresholds 升序、余额型降序；方向由选色函数（pickByQuota/pickByBalance）决定。
type ThresholdColors struct {
	Thresholds []float64 `yaml:"thresholds"`
	Colors     []string  `yaml:"colors"` // 预设名或 #RRGGBB
}
```

- [ ] **Step 2: 写 color_config 测试（先于实现）**

新建 `internal/adapters/ui/color_config_test.go`：

```go
package ui

import (
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func restoreColors(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { applyColorScheme(domain.ColorsConfig{}) })
}

// TestResolveColor 预设名→palette hex；#RRGGBB 原样；非法→false。
func TestResolveColor(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"green", colorGreen, true},
		{"RED", colorRed, true}, // 大小写不敏感
		{"#10B981", "#10B981", true},
		{"nope", "", false},
		{"#xyz", "", false},
	}
	for _, tc := range cases {
		got, ok := resolveColor(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("resolveColor(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestResolveThresholds 校验：颜色数≠阈值+1 或空阈值 → 回退（ok=false）。
func TestResolveThresholds(t *testing.T) {
	if _, ok := resolveThresholds(domain.ThresholdColors{Thresholds: []float64{70}}); ok {
		t.Error("colors length mismatch should fail")
	}
	if _, ok := resolveThresholds(domain.ThresholdColors{Thresholds: []float64{70, 90}, Colors: []string{"green", "yellow", "red"}}); !ok {
		t.Error("valid quota should resolve")
	}
	// 非法颜色 → 回退
	if _, ok := resolveThresholds(domain.ThresholdColors{Thresholds: []float64{1}, Colors: []string{"green", "nope"}}); ok {
		t.Error("illegal color should fail")
	}
}

// TestResolveColors_FallbackDefault 零值配置 → 默认 [70,90]/[10,1]。
func TestResolveColors_FallbackDefault(t *testing.T) {
	s := resolveColors(domain.ColorsConfig{})
	if len(s.quota.Thresholds) != 2 || s.quota.Thresholds[0] != 70 || s.quota.Thresholds[1] != 90 {
		t.Errorf("default quota thresholds = %v, want [70 90]", s.quota.Thresholds)
	}
	if len(s.balance.Thresholds) != 2 || s.balance.Thresholds[0] != 10 || s.balance.Thresholds[1] != 1 {
		t.Errorf("default balance thresholds = %v, want [10 1]", s.balance.Thresholds)
	}
}

// TestPickByQuota 升序：<70 绿、[70,90) 黄、>=90 红。
func TestPickByQuota(t *testing.T) {
	tc := domain.ThresholdColors{Thresholds: []float64{70, 90}, Colors: []string{colorGreen, colorYellow, colorRed}}
	cases := []struct {
		pct  float64
		want string
	}{
		{0, colorGreen}, {69, colorGreen}, {70, colorYellow},
		{89, colorYellow}, {90, colorRed}, {120, colorRed},
	}
	for _, c := range cases {
		if got := pickByQuota(tc, c.pct); got != c.want {
			t.Errorf("pickByQuota(%v) = %q, want %q", c.pct, got, c.want)
		}
	}
}

// TestPickByBalance 降序（含负值）：>=10 绿、[1,10) 黄、<1 红。
func TestPickByBalance(t *testing.T) {
	tc := domain.ThresholdColors{Thresholds: []float64{10, 1}, Colors: []string{colorGreen, colorYellow, colorRed}}
	cases := []struct {
		bal  float64
		want string
	}{
		{100, colorGreen}, {10, colorGreen}, {9.99, colorYellow},
		{1, colorYellow}, {0.99, colorRed}, {0, colorRed}, {-5, colorRed},
	}
	for _, c := range cases {
		if got := pickByBalance(tc, c.bal); got != c.want {
			t.Errorf("pickByBalance(%v) = %q, want %q", c.bal, got, c.want)
		}
	}
}

// TestApplyColorScheme_OverridesDefault 自定义配置经 applyColorScheme 后 StatusColor/BalanceColor 生效。
func TestApplyColorScheme_OverridesDefault(t *testing.T) {
	restoreColors(t)
	applyColorScheme(domain.ColorsConfig{
		Quota:   domain.ThresholdColors{Thresholds: []float64{50}, Colors: []string{"green", "red"}},
		Balance: domain.ThresholdColors{Thresholds: []float64{100}, Colors: []string{"red", "green"}},
	})
	// quota: <50 green(50→red 边界，pickByQuota 50<50 否→兜底 red)
	if got := StatusColor(40); got != colorGreen {
		t.Errorf("StatusColor(40) = %q, want green", got)
	}
	if got := StatusColor(60); got != colorRed {
		t.Errorf("StatusColor(60) = %q, want red", got)
	}
	// balance: >=100 red(降序首档), <100 green
	if got := BalanceColor(150, "USD"); got != colorRed {
		t.Errorf("BalanceColor(150) = %q, want red", got)
	}
	if got := BalanceColor(50, "USD"); got != colorGreen {
		t.Errorf("BalanceColor(50) = %q, want green", got)
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./internal/adapters/ui/ -run 'TestResolveColor|TestResolveThresholds|TestResolveColors|TestPickByQuota|TestPickByBalance|TestApplyColorScheme' -v`
Expected: FAIL（函数未定义）。

- [ ] **Step 4: 实现 color_config.go**

新建 `internal/adapters/ui/color_config.go`：

```go
package ui

import (
	"strings"
	"sync/atomic"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// presetColors 把 YAML 里的预设色名映射到 const.go 的 Tokyo Night 调色板。
// 大小写不敏感；未列出的名字按非法处理（回退默认档）。
var presetColors = map[string]string{
	"green":    colorGreen,
	"yellow":   colorYellow,
	"red":      colorRed,
	"gray":     colorGray,
	"grey":     colorGray,
	"purple":   colorPurple,
	"cyan":     colorCyan,
	"blue":     colorAccent,
	"accent":   colorAccent,
	"primary":  colorPrimary,
	"secondary": colorSecondary,
}

// colorScheme 是解析后的活动配色（颜色项已展开为 #RRGGBB 或调色板常量），
// 供 StatusColor / BalanceColor 读取。thresholds 空时由调用方回退默认。
type colorScheme struct {
	quota, balance domain.ThresholdColors
}

func defaultQuota() domain.ThresholdColors {
	return domain.ThresholdColors{
		Thresholds: []float64{70, 90},
		Colors:     []string{colorGreen, colorYellow, colorRed},
	}
}

func defaultBalance() domain.ThresholdColors {
	return domain.ThresholdColors{
		Thresholds: []float64{10, 1},
		Colors:     []string{colorGreen, colorYellow, colorRed},
	}
}

func defaultScheme() *colorScheme {
	return &colorScheme{quota: defaultQuota(), balance: defaultBalance()}
}

// activeColors 是进程级活动配色。用 atomic.Pointer 保证并发读（t.Parallel 测试 +
// 主循环渲染）与启动期单次写之间无 data race。
var activeColors atomic.Pointer[colorScheme]

func init() {
	activeColors.Store(defaultScheme())
}

// applyColorScheme 解析并安装用户配色；任一档非法则该档回退默认。main 启动期调用一次。
func applyColorScheme(cfg domain.ColorsConfig) {
	activeColors.Store(resolveColors(cfg))
}

// resolveColors 把用户配置解析为 colorScheme；非法档回退默认。
func resolveColors(cfg domain.ColorsConfig) *colorScheme {
	s := defaultScheme()
	if q, ok := resolveThresholds(cfg.Quota); ok {
		s.quota = q
	}
	if b, ok := resolveThresholds(cfg.Balance); ok {
		s.balance = b
	}
	return s
}

// resolveThresholds 校验一档：thresholds 非空、colors 数 == thresholds+1、每色合法。
// 通过则返回颜色已解析的副本；否则 ok=false（调用方回退默认）。
func resolveThresholds(tc domain.ThresholdColors) (domain.ThresholdColors, bool) {
	if len(tc.Thresholds) == 0 || len(tc.Colors) != len(tc.Thresholds)+1 {
		return domain.ThresholdColors{}, false
	}
	cols := make([]string, len(tc.Colors))
	for i, c := range tc.Colors {
		resolved, ok := resolveColor(c)
		if !ok {
			return domain.ThresholdColors{}, false
		}
		cols[i] = resolved
	}
	return domain.ThresholdColors{Thresholds: tc.Thresholds, Colors: cols}, true
}

// resolveColor 解析单个颜色项：预设名（大小写不敏感）或 #RRGGBB。
func resolveColor(name string) (string, bool) {
	if c, ok := presetColors[strings.ToLower(name)]; ok {
		return c, true
	}
	if isHexColor(name) {
		return name, true
	}
	return "", false
}

// isHexColor 校验 #RRGGBB（7 字符，#开头，后 6 位十六进制）。
func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// pickByQuota 配额型选色（thresholds 升序）：首个 threshold > pct 命中即返回对应色，
// 都未超过返回末尾兜底色。调用方需先处理 pct<0（N/A 灰）。
func pickByQuota(tc domain.ThresholdColors, pct float64) string {
	for i, t := range tc.Thresholds {
		if pct < t {
			return tc.Colors[i]
		}
	}
	return tc.Colors[len(tc.Colors)-1]
}

// pickByBalance 余额型选色（thresholds 降序）：首个 threshold <= balance 命中即返回对应色，
// 都低于返回末尾兜底色（含负值场景）。
func pickByBalance(tc domain.ThresholdColors, balance float64) string {
	for i, t := range tc.Thresholds {
		if balance >= t {
			return tc.Colors[i]
		}
	}
	return tc.Colors[len(tc.Colors)-1]
}
```

- [ ] **Step 5: 改 theme.go 接入配色 + 新增 BalanceColor**

在 `internal/adapters/ui/theme.go` 中，把 `StatusColor` 函数体改为读配色（保留注释与签名）：

```go
// StatusColor 把用量百分比映射到状态色。读活动配色（默认 <70 绿 / [70,90] 黄 / >90 红，
// 可经 config.yaml ui.colors.quota 覆盖）。pct<0 固定灰（N/A 或拉取失败），先于阈值判断。
func StatusColor(percent float64) string {
	if percent < 0 {
		return colorGray
	}
	return pickByQuota(activeColors.Load().quota, percent)
}

// BalanceColor 把余额数值映射到状态色（余额越低越危险）。读活动配色（默认 >=10 绿 /
// >=1 黄 / <1 红，可经 config.yaml ui.colors.balance 覆盖；支持负余额）。
// currency 暂未参与选色，保留参数供未来按币别分档。
func BalanceColor(balance float64, currency string) string {
	_ = currency
	return pickByBalance(activeColors.Load().balance, balance)
}
```

（删除旧 `StatusColor` 内的硬编码 switch，由上替换。）

- [ ] **Step 6: 运行新测试通过**

Run: `go test ./internal/adapters/ui/ -run 'TestResolveColor|TestResolveThresholds|TestResolveColors|TestPickByQuota|TestPickByBalance|TestApplyColorScheme' -v`
Expected: PASS。

- [ ] **Step 7: 主题回归（含并行 TestStatusColor）**

Run: `go test ./internal/adapters/ui/ -run 'TestStatusColor|TestProviderTag' -race -v`
Expected: PASS，且 `-race` 无报警（验证 `atomic.Pointer` 并发安全）。

- [ ] **Step 8: 提交**

```bash
git add internal/core/domain/config.go internal/adapters/ui/color_config.go internal/adapters/ui/color_config_test.go internal/adapters/ui/theme.go
git commit -m "feat(ui): configurable status/balance color thresholds with code defaults"
```

---

### Task 3: 配色注入与列表余额分支接线

**Files:**
- Modify: `internal/adapters/ui/account_list.go`（`formatAccountLine` 余额分支）
- Modify: `internal/adapters/ui/tui.go`（`Config.UIConfig` + `Run` 调 `applyColorScheme`）
- Modify: `cmd/main.go`（`ui.Config` 透传 `cfg.UI`）

**Interfaces:**
- Consumes: `ui.applyColorScheme`、`ui.BalanceColor`（Task 2 产出）。
- Produces: `tui.Config.UIConfig domain.UIConfig`；`formatAccountLine` 余额点色经 `BalanceColor`。

- [ ] **Step 1: 写失败测试（自定义余额阈值经 formatAccountLine 生效）**

追加到 `account_list_test.go`：

```go
// TestFormatAccountLine_BalanceColorConfigurable 验证余额点色随 applyColorScheme 变化：
// 把余额阈值设为「>=100 红 / <100 绿」后，Balance=150 应染红点（默认下 150>=10 为绿）。
func TestFormatAccountLine_BalanceColorConfigurable(t *testing.T) {
	restoreColors(t) // color_config_test.go 提供：t.Cleanup 复位为默认配色
	applyColorScheme(domain.ColorsConfig{
		Balance: domain.ThresholdColors{Thresholds: []float64{100}, Colors: []string{"red", "green"}},
	})
	balDim := domain.UsageDimension{Name: "Available balance", Balance: 150, Currency: "USD", PercentUsed: -1}
	u := domain.ProviderUsage{AccountID: "x", Provider: "newapi", Label: "relay", Primary: &balDim, Dimensions: []domain.UsageDimension{balDim}}
	got := formatAccountLine(u)
	// 阈值 [100] 降序：150>=100 → colors[0]=red。默认 [10,1] 下 150 会是绿，故此断言证明配置生效。
	if !strings.Contains(got, "["+colorRed+"]●[-]") {
		t.Errorf("with balance>=100 threshold=red, 150 should render red dot: %q", got)
	}
}
```

> 注：`restoreColors(t)` 已在 `color_config_test.go`（Task 2）定义，同包可用。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/adapters/ui/ -run TestFormatAccountLine_BalanceColorConfigurable -v`
Expected: FAIL（当前余额分支硬编码 `Balance>0→绿`，150 会被染绿而非红）。

- [ ] **Step 3: 改 formatAccountLine 余额分支**

`internal/adapters/ui/account_list.go` 的 `formatAccountLine` 中，把余额分支：

```go
	if d != nil && d.Currency != "" {
		// 余额型：显示余额 + 绿/红点（按余额正负）
		pctStr = formatMoneyShort(d.Balance, d.Currency)
		dot = "●"
		if d.Balance > 0 {
			dotCol = colorGreen
		} else {
			dotCol = colorRed
		}
	}
```

改为：

```go
	if d != nil && d.Currency != "" {
		// 余额型：显示余额 + BalanceColor 染色（阈值可配，默认 >=10 绿 / >=1 黄 / <1 红）。
		pctStr = formatMoneyShort(d.Balance, d.Currency)
		dot = "●"
		dotCol = BalanceColor(d.Balance, d.Currency)
	}
```

- [ ] **Step 4: 运行该测试通过**

Run: `go test ./internal/adapters/ui/ -run TestFormatAccountLine_BalanceColorConfigurable -v`
Expected: PASS。

- [ ] **Step 5: 既有余额测试回归**

Run: `go test ./internal/adapters/ui/ -run 'TestFormatAccountLineBalance|TestFormatAccountLineBalanceDepleted' -v`
Expected: PASS（默认 [10,1]：49.58→绿、0→红，与旧断言一致）。

- [ ] **Step 6: tui.Config 透传 UIConfig + Run 调 applyColorScheme**

`internal/adapters/ui/tui.go` 的 `Config` 结构体增字段：

```go
type Config struct {
	Logger          *zap.SugaredLogger
	Version         string
	Commit          string
	UIConfig        domain.UIConfig // 颜色阈值等 UI 配置（来自 config.yaml ui: 段）
	InitialData     []domain.ProviderUsage
	// ... 其余字段不变
```

在 `Run()` 中，`t.app = initializeTheme()` 之后追加：

```go
	t.app = initializeTheme()
	applyColorScheme(t.uiConfig.Colors) // 安装用户配色（零值→默认）
```

并给 `TUI` 增字段 `uiConfig domain.UIConfig`，`NewTUI` 中赋值 `uiConfig: cfg.UIConfig`。

- [ ] **Step 7: main.go 透传 cfg.UI**

`cmd/main.go` 构造 `ui.Config` 处增一行：

```go
	t := ui.NewTUI(ui.Config{
		Logger:          sugar,
		Version:         version,
		Commit:          gitCommit,
		UIConfig:        cfg.UI, // 透传颜色阈值等 UI 配置
		LoadInitial:     refreshAll,
		// ... 其余不变
```

- [ ] **Step 8: 全量构建+测试**

Run: `go build ./... && go test ./...`
Expected: 全绿。

- [ ] **Step 9: 提交**

```bash
git add internal/adapters/ui/account_list.go internal/adapters/ui/account_list_test.go internal/adapters/ui/tui.go cmd/main.go
git commit -m "feat(ui): wire configurable colors into list balance dot and boot"
```

---

### Task 4: sub2api provider

**Files:**
- Create: `internal/adapters/providers/sub2api/sub2api.go`
- Test: `internal/adapters/providers/sub2api/sub2api_test.go`

**Interfaces:**
- Consumes: `ports.UsageProvider`、`domain.Account`/`ProviderUsage`/`UsageDimension`。
- Produces: `sub2api.New() *Provider`（`Provider()`=`"sub2api"`）。Task 6 在 `NewRegistry` 注册。

- [ ] **Step 1: 写失败测试**

新建 `internal/adapters/providers/sub2api/sub2api_test.go`：

```go
package sub2api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenUsage 是 sub2api /v1/usage 响应金样本（字段名按社区实现假设；
// 若真实实例字段不同，仅改 apiResp 的 json tag 与本 golden 即可）。
const goldenUsage = `{"balance":42.5,"used":7.5}`

func TestProviderReturnsSlug(t *testing.T) {
	if got := New().Provider(); got != "sub2api" {
		t.Fatalf("Provider() = %q, want sub2api", got)
	}
}

// TestFetchUsageGolden：(a) Authorization=Bearer KEY；(b) 路径=/v1/usage；
// (c) Balance=42.5、Currency=USD、PercentUsed=-1；(d) Primary 指向余额维度。
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenUsage)
	}))
	defer srv.Close()

	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "MyRelay", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotAuth != "Bearer KEY" {
		t.Errorf("Authorization = %q, want Bearer KEY", gotAuth)
	}
	if gotPath != "/v1/usage" {
		t.Errorf("path = %q, want /v1/usage", gotPath)
	}
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Balance != 42.5 {
		t.Errorf("Balance = %v, want 42.5", d.Balance)
	}
	if d.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("PercentUsed = %v, want -1", d.PercentUsed)
	}
	if d.Source != "sub2api" {
		t.Errorf("Source = %q, want sub2api", d.Source)
	}
	if u.Primary == nil || u.Primary.Balance != 42.5 {
		t.Errorf("Primary wrong: %+v", u.Primary)
	}
	if u.Endpoint != "/v1/usage" || u.BaseURL != srv.URL {
		t.Errorf("basic info wrong: %+v", u)
	}
}

// TestFetchUsage_NegativeBalance 余额可为负（订阅+余额并存场景），仍正常返回。
func TestFetchUsage_NegativeBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"balance":-3.25,"used":0}`)
	}))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "K")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "neg", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != -3.25 {
		t.Errorf("Balance = %v, want -3.25", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_BaseURLRequired 自部署平台无默认 base；缺失即报错。
func TestFetchUsage_BaseURLRequired(t *testing.T) {
	t.Setenv("SUB2API_KEY", "K")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "x", TokenEnv: "SUB2API_KEY", BaseURL: ""}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for missing base_url")
	}
	if u.Err == nil {
		t.Error("u.Err should be set")
	}
}

// TestFetchUsage_Non200 非 2xx 被状态守卫拦截。
func TestFetchUsage_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "BAD")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "x", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/adapters/providers/sub2api/ -v`
Expected: 编译失败（包不存在）。

- [ ] **Step 3: 实现 provider**

新建 `internal/adapters/providers/sub2api/sub2api.go`：

```go
// Package sub2api 实现 ports.UsageProvider，对接 sub2api 中转平台的余额接口。
//
// 真实接口契约（社区实现 KonataAPI 等确认）：GET {BaseURL}/v1/usage，
// 鉴权 Authorization: Bearer <sk-api-key>。响应含余额（USD，可为负）。
// sub2api 为自部署平台，BaseURL 必填（无官方默认）。
//
// 注：/v1/usage 的精确字段社区文档未完全公开，此处按 {balance, used} 假设；
// 若真实实例字段名不同，仅需调整 apiResp 的 json tag。
package sub2api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	usagePath  = "/v1/usage"
	httpTimeout = 10 * time.Second

	nameAvailable = "Available balance"
	sourceTag     = "sub2api"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 sub2api 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

func New() *Provider { return &Provider{hc: &http.Client{Timeout: httpTimeout}} }

func (p *Provider) Provider() string { return "sub2api" }

// apiResp 是 /v1/usage 响应结构（字段名按社区实现假设）。
type apiResp struct {
	Balance float64 `json:"balance"`
	Used    float64 `json:"used"`
}

func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "sub2api",
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}

	if acc.BaseURL == "" {
		u.Err = fmt.Errorf("sub2api: base_url is required (self-hosted, no default)")
		return u, u.Err
	}
	key := os.Getenv(acc.TokenEnv)
	u.BaseURL = acc.BaseURL
	u.Endpoint = usagePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, acc.BaseURL+usagePath, nil)
	if err != nil {
		u.Err = fmt.Errorf("sub2api: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("sub2api: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("sub2api: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("sub2api: decode response: %w", err)
		return u, u.Err
	}

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     r.Balance,
		Currency:    "USD",
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/adapters/providers/sub2api/ -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/adapters/providers/sub2api/
git commit -m "feat(providers): add sub2api relay platform balance provider"
```

---

### Task 5: new-api provider

**Files:**
- Create: `internal/adapters/providers/newapi/newapi.go`
- Test: `internal/adapters/providers/newapi/newapi_test.go`

**Interfaces:**
- Consumes: `ports.UsageProvider`、`domain.*`。
- Produces: `newapi.New() *Provider`（`Provider()`=`"newapi"`）。Task 6 在 `NewRegistry` 注册。

- [ ] **Step 1: 写失败测试**

新建 `internal/adapters/providers/newapi/newapi_test.go`：

```go
package newapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

const (
	subscriptionPayload = `{"object":"billing_subscription","hard_limit_usd":100,"system_hard_limit_usd":100,"has_payment_method":true}`
	usagePayload        = `{"object":"list","total_usage":12.5}`
)

func TestProviderReturnsSlug(t *testing.T) {
	if got := New().Provider(); got != "newapi" {
		t.Fatalf("Provider() = %q, want newapi", got)
	}
}

// TestFetchUsageGolden：余额 = system_hard_limit_usd(100) - total_usage(12.5) = 87.5；
// 鉴权 Bearer；命中两个 billing 端点。
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth string
	var hitSub, hitUsage bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case strings.HasSuffix(r.URL.Path, "/dashboard/billing/subscription"):
			hitSub = true
			fmt.Fprint(w, subscriptionPayload)
		case strings.HasSuffix(r.URL.Path, "/dashboard/billing/usage"):
			hitUsage = true
			fmt.Fprint(w, usagePayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("NEWAPI_KEY", "KEY")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "NewAPI", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotAuth != "Bearer KEY" {
		t.Errorf("Authorization = %q, want Bearer KEY", gotAuth)
	}
	if !hitSub || !hitUsage {
		t.Errorf("both endpoints must be hit: sub=%v usage=%v", hitSub, hitUsage)
	}
	if len(u.Dimensions) != 1 || u.Dimensions[0].Balance != 87.5 {
		t.Errorf("Balance = %v, want 87.5 (100-12.5)", u.Dimensions[0].Balance)
	}
	if u.Dimensions[0].Currency != "USD" || u.Dimensions[0].PercentUsed != -1 {
		t.Errorf("dim wrong: %+v", u.Dimensions[0])
	}
	if u.Primary == nil || u.Primary.Balance != 87.5 {
		t.Errorf("Primary wrong: %+v", u.Primary)
	}
}

// TestFetchUsage_LimitFallback system_hard_limit_usd 缺失时回退 hard_limit_usd。
func TestFetchUsage_LimitFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscription") {
			fmt.Fprint(w, `{"hard_limit_usd":50,"system_hard_limit_usd":0}`)
		} else {
			fmt.Fprint(w, `{"total_usage":10}`)
		}
	}))
	defer srv.Close()
	t.Setenv("NEWAPI_KEY", "K")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != 40 { // 50-10
		t.Errorf("Balance = %v, want 40 (fallback hard_limit_usd)", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_UsageDegraded usage 端点失败时降级 used=0（余额=总额），不报错。
func TestFetchUsage_UsageDegraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscription") {
			fmt.Fprint(w, subscriptionPayload)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("NEWAPI_KEY", "K")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("usage 404 should degrade, not error: %v", err)
	}
	if u.Dimensions[0].Balance != 100 { // 总额 - 0
		t.Errorf("Balance = %v, want 100 (degraded used=0)", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_SubscriptionFails subscription 端点失败 → 报错（无法取总额）。
func TestFetchUsage_SubscriptionFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscription") {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			fmt.Fprint(w, usagePayload)
		}
	}))
	defer srv.Close()
	t.Setenv("NEWAPI_KEY", "BAD")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("subscription failure should error")
	}
}

// TestFetchUsage_BaseURLRequired 自部署无默认 base。
func TestFetchUsage_BaseURLRequired(t *testing.T) {
	t.Setenv("NEWAPI_KEY", "K")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: ""}
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("expected error for missing base_url")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/adapters/providers/newapi/ -v`
Expected: 编译失败。

- [ ] **Step 3: 实现 provider**

新建 `internal/adapters/providers/newapi/newapi.go`：

```go
// Package newapi 实现 ports.UsageProvider，对接 new-api（one-api fork）中转平台余额接口。
//
// 真实接口契约（OpenAI 兼容 billing，QuantumNous/new-api-docs）：
//   - GET {BaseURL}/v1/dashboard/billing/subscription → system_hard_limit_usd（总额，回退 hard_limit_usd）
//   - GET {BaseURL}/v1/dashboard/billing/usage        → total_usage（已用，美元）
//   - 鉴权 Authorization: Bearer <sk-api-key>（单凭证）
// 余额 = 总额 - 已用。usage 端点失败时降级 used=0（某些版本已弃用该端点）。
// new-api 自部署，BaseURL 必填。
package newapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	subscriptionPath = "/v1/dashboard/billing/subscription"
	usagePath        = "/v1/dashboard/billing/usage"
	httpTimeout      = 10 * time.Second

	nameAvailable = "Available balance"
	sourceTag     = "newapi"
)

var _ ports.UsageProvider = (*Provider)(nil)

type Provider struct {
	hc *http.Client
}

func New() *Provider { return &Provider{hc: &http.Client{Timeout: httpTimeout}} }

func (p *Provider) Provider() string { return "newapi" }

type subscriptionResp struct {
	SystemHardLimitUSD float64 `json:"system_hard_limit_usd"`
	HardLimitUSD       float64 `json:"hard_limit_usd"`
}

type usageResp struct {
	TotalUsage float64 `json:"total_usage"`
}

func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "newapi",
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}
	if acc.BaseURL == "" {
		u.Err = fmt.Errorf("newapi: base_url is required (self-hosted, no default)")
		return u, u.Err
	}
	key := os.Getenv(acc.TokenEnv)
	u.BaseURL = acc.BaseURL
	u.Endpoint = subscriptionPath

	total, err := p.getJSON(ctx, acc.BaseURL+subscriptionPath, key, &subscriptionResp{})
	if err != nil {
		u.Err = fmt.Errorf("newapi: subscription: %w", err)
		return u, u.Err
	}
	limit := total.SystemHardLimitUSD
	if limit == 0 {
		limit = total.HardLimitUSD // 字段回退
	}

	var used float64
	if ur, err := p.getJSON(ctx, acc.BaseURL+usagePath, key, &usageResp{}); err == nil {
		used = ur.TotalUsage
	}
	// usage 失败：降级 used=0（端点可能被弃用），余额=总额。

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     limit - used,
		Currency:    "USD",
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}

// getJSON 发 GET 并解码进 out；非 2xx 或解码失败返回错误。
func (p *Provider) getJSON(ctx context.Context, url, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/adapters/providers/newapi/ -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/adapters/providers/newapi/
git commit -m "feat(providers): add new-api relay platform balance provider"
```

---

### Task 6: 装配两 provider + 下拉选项 + 品牌色

**Files:**
- Modify: `cmd/main.go`（`NewRegistry`）
- Modify: `internal/adapters/ui/account_form.go`（`providerOptions`）
- Modify: `internal/adapters/ui/theme.go`（`providerColor`）
- Test: `internal/adapters/ui/theme_test.go`（`TestProviderTag_KnownProviders`）

**Interfaces:**
- Consumes: `sub2api.New()`、`newapi.New()`（Task 4/5 产出）。
- Produces: 两个新 provider 在 registry 与表单可用；品牌色经 `ProviderTag` 渲染。

- [ ] **Step 1: 写失败测试（品牌色表）**

`internal/adapters/ui/theme_test.go` 的 `TestProviderTag_KnownProviders` 的 `cases` 末尾追加两行：

```go
		{"sub2api", "#8B5CF6", "#FFFFFF"},
		{"newapi", "#10B981", "#FFFFFF"},
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./internal/adapters/ui/ -run TestProviderTag_KnownProviders -v`
Expected: FAIL（两 slug 未知，回退灰色）。

- [ ] **Step 3: 加品牌色**

`internal/adapters/ui/theme.go` 的 `providerColor` map 追加：

```go
	"sub2api": {"#8B5CF6", "#FFFFFF"}, // 紫
	"newapi":  {"#10B981", "#FFFFFF"}, // 翠绿
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./internal/adapters/ui/ -run TestProviderTag_KnownProviders -v`
Expected: PASS。

- [ ] **Step 5: 注册 provider**

`cmd/main.go` 顶部 import 增：

```go
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/newapi"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/sub2api"
```

`NewRegistry(...)` 改为：

```go
	reg := providers.NewRegistry(glm.New(), minimax.New(), kimi.New(), deepseek.New(), sub2api.New(), newapi.New())
```

- [ ] **Step 6: 表单下拉增选项**

`internal/adapters/ui/account_form.go`：

```go
var providerOptions = []string{"glm", "minimax", "kimi", "deepseek", "sub2api", "newapi"}
```

- [ ] **Step 7: 全量构建+测试**

Run: `go build ./... && go test ./...`
Expected: 全绿。

- [ ] **Step 8: 提交**

```bash
git add cmd/main.go internal/adapters/ui/account_form.go internal/adapters/ui/theme.go internal/adapters/ui/theme_test.go
git commit -m "feat: register sub2api & new-api providers with brand colors"
```

---

## Self-Review

**1. Spec coverage：**
- §2 高亮撑满 → Task 1 ✓
- §3.1 BalanceColor 收口 → Task 2/3 ✓
- §3.2–3.5 配置结构/解析/注入 → Task 2 + Task 3 ✓
- §4.1 sub2api → Task 4 ✓
- §4.2 newapi → Task 5 ✓
- §4.3 装配 → Task 6 ✓
- §5 验证 → 各 Task 的 test 步骤 + 真机步骤 ✓
- §6 非目标（不改 Account、不动 details/search padding）→ 计划未触碰 ✓

**2. Placeholder scan：** 无 TBD/TODO；所有代码块完整；sub2api 字段假设已在代码注释与测试 golden 中显式标注「若真实字段不同则改 tag」。

**3. Type consistency：**
- `applyColorScheme(domain.ColorsConfig)`、`BalanceColor(float64, string) string`、`StatusColor(float64) string` 在 Task 2 定义，Task 3 消费——签名一致 ✓
- `domain.ThresholdColors{Thresholds []float64, Colors []string}` 跨 domain/ui 一致 ✓
- `sub2api.New()`/`newapi.New()` 在 Task 4/5 定义，Task 6 消费 ✓
- `tui.Config.UIConfig domain.UIConfig` 在 Task 3 定义，main.go 消费 ✓

## 执行注意

- **字段校准风险**：sub2api `/v1/usage` 与 new-api `total_usage` 单位需用真实实例 `curl` 比对（spec §5）。若不符，仅改对应 `json` tag / golden，不影响架构。
- **Commit**：本计划按 TDD 节奏给出 commit 步骤；若你不希望自动提交，执行时每 Task 末跳过 commit 即可，或集中提交。
