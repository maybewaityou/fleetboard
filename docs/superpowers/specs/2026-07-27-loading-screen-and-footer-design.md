# fleetboard 加载界面 + Footer 精简 设计规格

> 状态：已确认 · 日期：2026-07-27 · 作者：MeePwn
> 范围：2 项优化——(1) 启动时异步加载界面，告别"卡在终端"；(2) footer 按键精简，低频键收敛到 `?` Help。
> 参考：兄弟项目 `lazytmux`（footer 的"内容精简而非渲染裁剪"策略）。

## 0. 决策摘要（已与用户确认）

| 项 | 决策 |
|----|------|
| Loading 形态 | 全屏居中：`fleetboard` 标题 + braille spinner 动画 + `Loading accounts…`；数据回来后整体切到主界面 |
| Loading 实现 | 方案 A：`Config.LoadInitial` 回调异步引导（与 `RefreshAll`/`RefreshSelected` 同构），UI 自持加载视图，`main.go` 仅把 `FetchAll` 挪进回调 |
| Footer 精简策略 | 砍低频键（lazytmux 真实做法），**不**做宽度响应式裁剪 |
| `←/→ Focus` | **砍掉**（与 `Tab` 焦点循环功能重叠；footer 从 12 → 8 键）。注：此键由 `optimizations` 批次 Task 1 加入，本次按简化要求反转——该键仍保留在 `handleGlobalKeys` 与 `keyBindings`（Help 可查），仅从 footer 提示行移除 |

执行顺序：先 §2 footer（单文件机械改动，零风险）→ 再 §1 加载界面（跨 `ui/tui.go` + `cmd/main.go` + 新增 `loading.go`）。每步 `go build` + `go test ./...` 绿。

## 1. 背景：为什么启动会卡在终端

`cmd/main.go:133` 在 `ui.NewTUI` **之前**同步调用 `agg.FetchAll(ctx, cfg.Accounts)`：

```go
initial := agg.FetchAll(ctx, cfg.Accounts)  // 阻塞：所有 provider HTTP 并发，各自 10s 超时，wg.Wait()
cache.replaceAll(initial)
...
t := ui.NewTUI(ui.Config{InitialData: cache.snapshot(), ...})
t.Run()  // 此刻才进入 tview 主循环、才画出第一帧
```

后果：请求期间 tview 主循环尚未启动，终端"没进应用"。账号多 / 网络慢时最坏 ~10s 黑屏。修法：先让主循环跑起来画加载界面，再在后台 goroutine 拉数据，回来后换根到主界面。

> 技术栈修正：本项目是 **tview/tcell**（`go.mod` 仅 `rivo/tview` + `gdamore/tcell`），**非** Bubble Tea。无 `Init()/Update()/View()`、`tea.Cmd`、`WindowSizeMsg`；异步刷新只能靠 `app.QueueUpdateDraw`（本仓 `queueDraw`）驱动。

## 2. Task A — Footer 精简

仅改 `internal/adapters/ui/status_bar.go`，`keybindings.go` **不动**（Help 面板内容来自单一真源 `keyBindings`，已含全量 12 键，故砍掉的键仍可在 `?` Help 查到，永不漂移）。

`defaultHints()` 12 → 8 键：

```go
func defaultHints() string {
    k := colorCyan
    return "[" + k + "]↑↓[-] Navigate  • " +
        "[" + k + "]a[-] New  • " +
        "[" + k + "]e[-] Edit  • " +
        "[" + k + "]d[-] Delete  • " +
        "[" + k + "]r[-] Refresh  • " +
        "[" + k + "]/[-] Search  • " +
        "[" + k + "]?[-] Help  • " +
        "[" + k + "]q[-] Quit"
}
```

砍掉：`←/→ Focus`（Tab 等价）、`p Pin`、`R Refresh All`、`s Sort`（低频，Help 可查）。

`emptyHints()` 5 → 4 键（空态下 `R` 重新拉取空集无意义）：

```go
func emptyHints() string {
    k := colorCyan
    return "[" + k + "]No accounts[-]  •  " +
        "[" + k + "]a[-] New  •  " +
        "[" + k + "]?[-] Help  •  " +
        "[" + k + "]q[-] Quit"
}
```

## 3. Task B — 加载界面（异步引导）

### 3.1 `ui.Config` 新增引导回调

```go
type Config struct {
    ...
    InitialData []domain.ProviderUsage          // 保留：无 LoadInitial 时的同步种子（单测用）
    LoadInitial func() []domain.ProviderUsage   // 新增：非 nil 时走异步加载流程
    ...
}
```

`LoadInitial != nil` → 异步加载；`== nil` → 退回现有同步 `InitialData` 路径（`loadInitialData()` 直接画 `allCache`）。**单测与简单装配不受影响。**

### 3.2 新增 `internal/adapters/ui/loading.go`

```go
const spinnerInterval = 120 * time.Millisecond

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// LoadingView 是启动占位：居中的 app 名 + 动画 spinner + 标签。
type LoadingView struct{ *tview.TextView }

func NewLoadingView() *LoadingView { /* AlignCenter + DynamicColors + Default bg */ }

// SetFrame 由 spinner ticker 调用，推进帧并重设整段文本。
func (l *LoadingView) SetFrame(frame int, label string) { /* spinnerFrames[frame%len] + label */ }

// newLoadingRoot 把 LoadingView 套进上下 nil spacer 的 Flex 做垂直居中（复用 openHelp 套路），
// 并挂 InputCapture：q / Ctrl-C → app.Stop（加载期间主 handleGlobalKeys 尚未挂载）。
func newLoadingRoot(app *tview.Application, lv *LoadingView) *tview.Flex
```

文本两行：`fleetboard`（`colorAccent` + `::b`）+ 空行 + `[cyan]<spinner>[-]  Loading accounts…`。

> braille spinner 在 Termux/Android 终端可正常渲染（用户主力机为 OPPO Pad + Termux）。如个别终端字体缺字，回退方案：换 ASCII `|/-\`（实现时若反馈异常再换，不在本批做）。

### 3.3 `ui/tui.go` — `Run()` 分流 + `bootAsync()`

```go
func (t *TUI) Run() error {
    defer recover...
    t.app = initializeTheme()
    t.app.EnableMouse(true)
    t.queueDraw = func(f func()) { t.app.QueueUpdateDraw(f) }
    t.buildComponents().buildLayout().bindEvents()
    t.accountList.SetSortTitle(t.sortMode.String())

    // clock ticker（相对时间推进）保持原样：加载期间 allCache 为空，applyCacheToViews 无副作用。
    ticker := time.NewTicker(clockTickInterval)
    defer ticker.Stop()
    go func() { for range ticker.C { t.queueDraw(t.applyCacheToViews) } }()

    if t.loadInitial != nil {
        t.bootAsync()                  // SetRoot(loading) → spinner ticker → 后台拉取 → swap
    } else {
        t.loadInitialData()            // 同步路径（单测）：直接画 allCache
        t.app.SetRoot(t.root, true)
        t.focusList()
    }

    if err := t.app.Run(); err != nil { ... }
    return nil
}
```

```go
func (t *TUI) bootAsync() {
    lv := NewLoadingView()
    lv.SetFrame(0, "Loading accounts…")
    root := newLoadingRoot(t.app, lv)
    t.app.SetRoot(root, true)

    spinTick := time.NewTicker(spinnerInterval)
    go func() {
        i := 0
        for range spinTick.C {
            f := i
            i++
            t.queueDraw(func() { lv.SetFrame(f, "Loading accounts…") })
        }
    }()

    go func() {
        usages := t.loadInitial()
        t.queueDraw(func() {
            spinTick.Stop()
            t.applyDataset(usages)     // 同步写 allCache + applyCacheToViews（见 3.5）
            t.app.SetRoot(t.root, true)
            t.focusList()
        })
    }()
}
```

> 关键陷阱：`swap` 闭包在主循环执行（经 `queueDraw`），**必须**用 `applyDataset`（同步写）而非 `Render`（内部 `QueueUpdateDraw` 会等主循环 → 死锁）。与现有 `doTogglePin` / 表单提交的纪律一致。

### 3.4 `cmd/main.go` — 删同步预取，改为回调

```go
// 删除：
//   initial := agg.FetchAll(ctx, cfg.Accounts)
//   cache.replaceAll(initial)

// Config 改传 LoadInitial：
LoadInitial: func() []domain.ProviderUsage {
    usages := agg.FetchAll(ctx, cfg.Accounts)
    cache.replaceAll(usages)
    return cache.snapshot()
},
// InitialData 留空（或删除该字段赋值）
```

### 3.5 数据流与边界

- 引导回调在**后台 goroutine** 执行（`agg.FetchAll` 内部并发 + `wg.Wait`），返回快照经 `queueDraw` marshal 到主循环 → `applyDataset` 同步落盘 → `SetRoot` 换到主界面。全程 `allCache` 只在主循环写，无锁无 race（与 `Render` 同纪律）。
- 换根用 `SetRoot(t.root, true)`，与 `closeModal`/`closeForm` 回主界面完全同构。

## 4. 错误处理

- 单账号拉取失败：`ProviderUsage.Err` 透传（既有契约），UI 标红行，不阻断——加载完成后照常进主界面，红行可见。
- 整体超时：各 provider `http.Client.Timeout = 10s`，`FetchAll` 并发，最坏 ~10s spinner 后进主界面（空或部分数据）。可接受；本批不加超时 UI。
- 引导回调 panic：由 `Run()` 顶层 `defer recover` 兜底（仅记日志）；可接受，因 provider 已在内部吞错返回 `.Err`。

## 5. 测试

- `defaultHints()` / `emptyHints()`：断言含/不含特定键（如断言不含 `Pin`/`Sort`/`Refresh All`/`Focus`，含 `New`/`Quit`）。若仓库已有 `status_bar_test.go` 则同步其断言。
- `bootAsync` swap 路径：利用现有 `queueDraw == nil` 同步驱动技巧不便（`app` 已建）。改为：把 swap 的核心抽成纯函数 `applyDataset`（已有），单测直接验证 `applyDataset` 后 `allCache` 与视图状态；`bootAsync` 的编排靠 `go test -race` 烟测 + 手动跑 `fleetboard`。
- `make test` / `go test ./...` 全绿。

## 6. 不在本批（Out of Scope）

- 加载进度计数（`Loading 2/5…`）：需 `FetchAll` 暴露进度回调，改动较大，用户未选。
- footer 宽度响应式省略：lazytmux 无此规则，用户选"精简内容"路线，不做。
- spinner 回退 ASCII：仅在反馈渲染异常时再做。
