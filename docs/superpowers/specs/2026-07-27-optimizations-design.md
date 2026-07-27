# fleetboard 优化批次设计规格

> 状态：已确认 · 日期：2026-07-27 · 作者：MeePwn
> 范围：8 项优化（方向键焦点 / 排序 / 最近窗口百分比 / placeholder 颜色 / 英文化 / vendor→provider 改名 / Homebrew 发布 / 双语 README）。
> 参考：兄弟项目 `lazytmux`（同栈 Go + cobra + tview/tcell + Tokyo Night + 六边形架构）。

## 0. 决策摘要（已与用户确认）

| 任务 | 决策 |
|------|------|
| 5 列表百分比 | 显示「重置时间最近」那一档（nearest reset），而非百分比最高档 |
| 8 vendor→provider | 彻底改名：字段 + 方法 + 类型（`VendorUsage`→`ProviderUsage`）+ 所有标识符 + YAML key `vendor:`→`provider:`（破坏现有 config.yaml，v0.1.0 可接受）|
| 2 排序字段 | `Name ↑ → Usage % ↓ → Refreshed ↓` 三模式循环；`s` 进一步、`S` 两步；置顶浮顶；列表标题显示当前模式 |

执行顺序：**先 Task 8 改名**（全局机械替换，先做避免后续二次改名）→ Task 1/2/5/6/7（UI + providers，含测试）→ Task 3（发布）→ Task 4（README/LICENSE）。每步 `go build` + `make test` 绿。

## 1. Task 1 — 方向键 ←/→ 切换焦点

移植 lazytmux 的全局焦点模型。`handleGlobalKeys`（`internal/adapters/ui/tui.go`）的 `switch e.Key()` 增补：

```go
case tcell.KeyRight:
    if t.listHasFocus() { t.focusDetails(); return nil }
case tcell.KeyLeft:
    if t.detailsHasFocus() { t.focusList(); return nil }
```

- 列表自身的 `SetInputCapture` 已吞掉 `Left/Backspace/Esc`（→ search bar），放行 `Right` 上浮到全局 → 聚焦 details；details 无吞键捕获，`Left` 上浮 → 聚焦 list。**与 lazytmux 完全一致，列表零改动。**
- 焦点判定从 `app.GetFocus() == x` 改为 `x.HasFocus()`（规避 tview v0.42 鼠标焦点内部 primitive 的指针比较脆弱性），新增 `listHasFocus()` / `detailsHasFocus()`。
- **footer**（`status_bar.go`）：`defaultHints()` 在 Navigate 后补 `←/→ Focus`。
- **help**（`keybindings.go`）：`{"Navigate","←/→","Focus list/details"}` 已存在，无需改。

## 2. Task 2 — 排序 s/S

新增 `internal/adapters/ui/sort.go`（移植 lazytmux `sort.go`）：

```go
type SortMode int
const (
    SortByNameAsc SortMode = iota // Name ↑
    SortByUsageDesc               // Usage % ↓
    SortByRefreshedDesc           // Last Refreshed ↓
)
func (s SortMode) String() string // "Name ↑" / "Usage ↓" / "Refreshed ↓"
func (s SortMode) Next() SortMode { return (s + 1) % 3 }

func sortUsagesForUI(usages []domain.ProviderUsage, mode SortMode) {
    sort.SliceStable(usages, func(i, j int) bool {
        if usages[i].Pinned != usages[j].Pinned { return usages[i].Pinned }
        switch mode {
        case SortByUsageDesc:     return displayPercent(usages[i]) > displayPercent(usages[j])
        case SortByRefreshedDesc: return usages[i].FetchedAt.After(usages[j].FetchedAt)
        default:                  return usages[i].Label < usages[j].Label
        }
    })
}
```

- `TUI` 增 `sortMode SortMode` 字段，初值 `SortByNameAsc`。
- `handleGlobalKeys` 的 `s` 分支替换为 `t.sortMode = t.sortMode.Next(); t.applySortAndRender()`，新增 `S`（`.Next().Next()`）。
- `visibleSorted()` 由「仅置顶排序」改为 `sortUsagesForUI(visible, t.sortMode)`——所有渲染路径（applyCacheToViews / handleSearchInput）自动按当前模式排序，搜索/刷新后顺序稳定。
- `AccountList` 增 `SetSortTitle(mode string)`：标题渲染为 ` Accounts — Sort: <mode> `。
- `applySortAndRender()`：设标题 + `applyCacheToViews()` + `setStatusTemporary("Sort: <mode>")`。
- **footer**：`s Sort` 已存在。**help**：补 `{"Usage","s/S","Cycle sort"}`（当前缺失）。

`displayPercent` 与 Task 5 共用「显示维度」选择，保证排序键与列表展示一致。

## 3. Task 5 — 列表显示最近重置窗口的百分比

新增 `account_list.go` 辅助：

```go
// displayDimension 取 ResetsAt 最近（soonest）的有效维度；无任何重置时间时
// 回退 Primary（余额型 kimi/deepseek），再回退 nil。
func displayDimension(u domain.ProviderUsage) *domain.UsageDimension {
    var nearest *domain.UsageDimension
    for i := range u.Dimensions {
        d := &u.Dimensions[i]
        if d.ResetsAt.IsZero() { continue }
        if nearest == nil || d.ResetsAt.Before(nearest.ResetsAt) { nearest = d }
    }
    if nearest != nil { return nearest }
    return u.Primary
}
```

- `formatAccountLine` 与 `displayPercent`（原 `primaryPercent`）改用 `displayDimension(u)`。
- 效果：GLM 显示 5 小时滚动窗口百分比；kimi/deepseek 仍显示余额（无 ResetsAt → 回退 Primary）；minimax（单窗口有 reset）不变。
- `SelectPrimary` / `Primary` 保留（providers 仍设置、作回退）。详情页不变（仍按 ResetsAt 升序列全部维度）。
- 受影响测试：`account_list_test.go` golden 输出 → 同步更新。

## 4. Task 6 — placeholder 字体颜色

对齐 lazytmux `session_form.go`：account form 每个 `InputField` 在 `SetPlaceholder(...)` 后显式 `SetPlaceholderTextColor(tcell.GetColor(colorSecondary))`（`#565f89`）。Provider `DropDown` 的 noSelection 文本已由 `initializeTheme` 的 `TertiaryTextColor = colorSecondary` 全局继承，无需额外处理。

## 5. Task 7 — 应用内英文化

仅翻译用户可见字符串（代码注释保持中文——Task 7 针对「应用内」即运行时 UI）：

| 文件 | 旧 | 新 |
|------|----|----|
| `account_form.go` | `phLabel "e.g. 智谱编码-主力"` | `"e.g. GLM main"` |
| | `phVendor "选择厂商"` | `"Select provider"` |
| | `phBaseURL "留空使用默认"` | `"leave empty for default"` |
| | 提交提示 `"Enter 提交 · ESC 取消"` | `"Enter submit · ESC cancel"` |
| `help.go` | 标题 `"(Esc / ? / q 关闭)"` | `"(Esc / ? / q to close)"` |
| `glm/glm.go` | `"5小时额度"`/`"每周额度"`/`"MCP每月"`/`"额度#%d"`/`unitCount "次"` | `"5h Quota"`/`"Weekly Quota"`/`"MCP Monthly"`/`"Quota #%d"`/`"uses"` |
| `kimi/kimi.go` | `nameAvailable "可用余额"` | `"Available balance"` |
| `deepseek/deepseek.go` | `nameAvailable "可用余额"` | `"Available balance"` |

判断依据：上述维度名由 adapter 硬编码，**非** API 返回（GLM 返回 TOKENS_LIMIT/TIME_LIMIT 类型码，kimi/deepseek 返回余额数字），故按「除非后端接口返回中文否则翻译」规则翻译。status/footer/details/list 已为英文。

## 6. Task 8 — vendor → provider 彻底改名

全仓 `.go`（prod + test）机械改名，**slug 值（`"glm"`/`"kimi"`/`"deepseek"`/`"minimax"`）与 registry map key 不变**，仅改标识符/字段/类型/YAML key：

| 旧 | 新 |
|----|----|
| `domain.Account.Vendor` (`yaml:"vendor"`) | `Provider` (`yaml:"provider"`) |
| 类型 `domain.VendorUsage` | `ProviderUsage` |
| 字段 `VendorUsage.Vendor` | `Provider` |
| 接口方法 `ports.UsageProvider.Vendor()` | `Provider()` |
| `ports.ProviderLookup.Get(vendor)` | `Get(provider)`（形参） |
| `services.ErrUnknownVendor` + 消息 | `ErrUnknownProvider` + `"unknown provider"` |
| `providers.Registry.byVendor` | `byProvider` |
| `ui.vendorColor` / `VendorTag` | `providerColor` / `ProviderTag` |
| `ui.vendorOptions` / `vendorInfoLine` / `afFieldVendor` | `providerOptions` / `providerInfoLine` / `afFieldProvider` |
| `ui.unknownVendorBG/FG` | `unknownProviderBG/FG` |
| YAML key `vendor:` | `provider:`（含 store 测试断言） |

步骤：`grep -rni vendor` 人工核对无意外命中 → 两遍 `sed`（先 `Vendor`→`Provider`，再 `vendor`→`provider`）→ `go build ./...` → `go test ./...` → 修测试 golden/断言（store_test、registry_test、各 provider_test、ui *_test）。**破坏现有 `~/.fleetboard/config.yaml`**（v0.1.0 预发布，可接受；README 注明）。

## 7. Task 3 — GitHub 发布工作流 + Homebrew

新增（移植 lazytmux）：

- **`.goreleaser.yaml`**：`project_name: fleetboard`；`builds` main `./cmd`、`CGO_ENABLED=0`、`goos:[linux,darwin]`/`goarch:[amd64,arm64]`；ldflags `-X main.version/gitCommit`；`archives` tar.gz `fleetboard_{{.Os}}_{{.Arch}}`；`checksum`/`changelog`；`brews:` → `owner: maybewaityou`/`name: homebrew-tap`（复用同 tap）/`token: {{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}`/`directory: Formula`/`homepage: https://github.com/maybewaityou/fleetboard`/`license: Apache-2.0`/`test: fleetboard --help`。**去掉** lazytmux 的 `dependencies:[tmux]`。
- **`.github/workflows/release.yml`**：照搬（on `v*` tag，goreleaser-action v6，Go 1.24，`GITHUB_TOKEN` + `HOMEBREW_TAP_GITHUB_TOKEN`）。
- **手动步骤（用户）**：在 fleetboard 仓库设置 secret `HOMEBREW_TAP_GITHUB_TOKEN`（对 `maybewaityou/homebrew-tap` 有 `contents:write` 的 PAT）。README 注明。

## 8. Task 4 — README（中英）+ LICENSE + 二维码

- **`README.md`**（英）/ **`README.zh-CN.md`**（中），骨架对齐 lazytmux：标题+语言切换 → Features → How it works → Installation（`brew install maybewaityou/tap/fleetboard` + `brew trust maybewaityou/tap`、二进制下载脚本、`make build`）→ Key Bindings（取自 `keybindings.go`）→ Architecture（六边形树）→ Contributing + 语义提交 → Support/Sponsor（Buy Me A Coffee + 微信/支付宝二维码 table）→ Acknowledgments。
- 复制 `donate-wechat.jpg` / `donate-alipay.jpg`（lazytmux/docs/resources/ → fleetboard/docs/resources/）。
- 新增 Apache-2.0 **`LICENSE`**（与文件头一致；goreleaser 声明 Apache-2.0）。
- **Screenshots 暂略**（无截图），README 注明可后续补。

## 9. 验证

- 每步 `go build ./...` 通过。
- 终态 `make test`（`go test -race -cover ./...`）全绿，`make quality`（gofumpt + go vet）通过。
- 排序/焦点/最近窗口百分比：补/改对应 `*_test.go` golden。
- README 安装命令与 `.goreleaser.yaml` 产物名一致（`fleetboard_<os>_<arch>.tar.gz`）。

## 10. 非目标

- 不翻译代码注释（Task 7 仅 UI）。
- 不加 CI lint 工作流（lazytmux 也只有 release，保持对齐；可选后续）。
- 不做 config.yaml 从 `vendor:`→`provider:` 的自动迁移（破坏式，文档注明）。
