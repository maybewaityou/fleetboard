// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package ui hosts fleetboard's tview-based TUI. The assembly (buildLayout)
// mirrors lazytmux/internal/adapters/ui/tui.go so the two tools share a layout
// language: a 2-row root { header(2) · content · statusbar(1) } with the content
// a 3:2 column split { left=list | right=details }.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

// Compile-time assertion that *TUI satisfies ports.View. Render + Run are the
// two methods the service layer (Task 9/12) drives the screen through.
var _ ports.View = (*TUI)(nil)

// RefreshAllFunc is the shape of the "refresh all" callback (R). It returns the
// full post-refresh dataset so the TUI can re-render atomically; a nil return is
// treated as "no change". Task 12 wires this to services.Aggregator.FetchAll.
type RefreshAllFunc func() []domain.ProviderUsage

// RefreshSelectedFunc is the shape of the "refresh selected" callback (r). It
// receives the currently-selected AccountID and returns the full post-refresh
// dataset (the selected account re-fetched and merged back into the rest) so the
// TUI re-renders atomically; a nil return is treated as "no change".
//
// Race-safety: doRefreshSelected reads t.selectedID on the tview main loop and
// passes the string *by value* into this callback, which runs on a background
// goroutine. That keeps the refresh path data-race-free under `go test -race`
// without adding a lock — the same discipline Render already follows by
// marshalling its writes through queueDraw. Task 12 wires this to
// services.Aggregator.FetchOne.
type RefreshSelectedFunc func(accountID string) []domain.ProviderUsage

// Config parameterizes the TUI. RefreshSelected/RefreshAll are optional: when
// nil, r/R flash a "not wired" status instead of no-op-ing silently, so a
// misconfigured assembly makes its own wiring state obvious during smoke.
type Config struct {
	Logger          *zap.SugaredLogger
	Version         string
	Commit          string
	InitialData     []domain.ProviderUsage
	RefreshSelected RefreshSelectedFunc // r — refresh the currently-selected account
	RefreshAll      RefreshAllFunc      // R — refresh every account

	// CRUD 回调（a/e/d）。均返回更新后的完整数据集，TUI 直接 Render，不碰 store（六边形）。
	OnSaveAccount   func(domain.Account) []domain.ProviderUsage                // a — 新增账号
	OnDeleteAccount func(id string) []domain.ProviderUsage                     // d — 删除账号
	OnEditAccount   func(id string, acc domain.Account) []domain.ProviderUsage // e — 编辑账号
	OnLoadAccount   func(id string) (domain.Account, bool)                   // 编辑时反查账号预填表单
	OnTogglePin     func(id string) []domain.ProviderUsage                     // p — 置顶/取消置顶
}

// TUI is the runnable tview application. It implements ports.View (Run + Render).
type TUI struct {
	logger *zap.SugaredLogger

	version string
	commit  string

	app *tview.Application

	header      *AppHeader
	searchBar   *SearchBar
	accountList *AccountList
	details     *AccountDetails
	statusBar   *StatusBar

	root    *tview.Flex
	content *tview.Flex

	// allCache is the last dataset Render gave us; the search box and list both
	// read from it (filtered by visibleUsages) so search and refresh compose.
	allCache []domain.ProviderUsage
	// selectedID is preserved across refreshes so the user's selection survives
	// a re-render (mirrors lazytmux's SelectByName-after-UpdateSessions flow).
	selectedID string
	// sortMode is the active list sort, cycled by s/S.
	sortMode SortMode

	refreshSelected RefreshSelectedFunc
	refreshAll      RefreshAllFunc

	onSaveAccount   func(domain.Account) []domain.ProviderUsage
	onDeleteAccount func(id string) []domain.ProviderUsage
	onEditAccount   func(id string, acc domain.Account) []domain.ProviderUsage
	onLoadAccount   func(id string) (domain.Account, bool)
	onTogglePin     func(id string) []domain.ProviderUsage

	// statusTimer reverts a transient footer message back to the default hints.
	statusTimer *time.Timer
	// queueDraw schedules a func on the tview main loop; overridable in tests so
	// an async Render can be driven synchronously.
	queueDraw func(func())
}

// NewTUI constructs the TUI. Sub-components are built in Run() via the builder
// chain (same pattern as lazytmux), so NewTUI itself stays allocation-only and
// side-effect-free.
func NewTUI(cfg Config) *TUI {
	return &TUI{
		logger:          cfg.Logger,
		version:         cfg.Version,
		commit:          cfg.Commit,
		sortMode:        SortByNameAsc,
		allCache:        cfg.InitialData,
		refreshSelected: cfg.RefreshSelected,
		refreshAll:      cfg.RefreshAll,
		onSaveAccount:   cfg.OnSaveAccount,
		onDeleteAccount: cfg.OnDeleteAccount,
		onEditAccount:   cfg.OnEditAccount,
		onLoadAccount:   cfg.OnLoadAccount,
		onTogglePin:     cfg.OnTogglePin,
	}
}

// Run builds the components, lays them out, binds keys, and blocks on the tview
// main loop. Implements ports.View.
func (t *TUI) Run() error {
	defer func() {
		if r := recover(); r != nil {
			if t.logger != nil {
				t.logger.Errorw("panic recovered", "error", r)
			}
		}
	}()
	t.app = initializeTheme()
	t.app.EnableMouse(true)
	t.queueDraw = func(f func()) { t.app.QueueUpdateDraw(f) }
	t.buildComponents().buildLayout().bindEvents().loadInitialData()
	t.accountList.SetSortTitle(t.sortMode.String())

	// clock ticker：每 clockTickInterval 重渲列表，让 "Last Refreshed: Xm ago" 相对
	// 时间持续推进，而非停在首次渲染时的 "just now"。tick 走 queueDraw，与 Render 同一
	// 主循环通道；defer Stop 保证 TUI 退出时 goroutine 不泄漏。
	ticker := time.NewTicker(clockTickInterval)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			t.queueDraw(t.applyCacheToViews)
		}
	}()

	t.app.SetRoot(t.root, true)
	t.focusList()
	if t.logger != nil {
		t.logger.Infow("starting TUI", "version", t.version, "commit", t.commit)
	}
	if err := t.app.Run(); err != nil {
		if t.logger != nil {
			t.logger.Errorw("application run error", "error", err)
		}
		return err
	}
	return nil
}

// Render is the ports.View write path. It may be called from any goroutine
// (the refresh callbacks in doRefreshSelected/doRefreshAll spawn one; the Task
// 12 service will too), so EVERY field write it causes must be marshalled onto
// the tview main loop. In particular t.allCache is read by visibleUsages() on
// the main loop (via handleSearchInput/applyCacheToViews), so writing it here
// on the caller's goroutine would race with a concurrent keystroke. We avoid
// that by performing both the cache assignment and the repaint inside a single
// queueDraw callback — the main loop is the only goroutine that touches
// allCache, so no synchronization primitive is needed.
//
// The queueDraw == nil branch covers pre-Run() callers (e.g. unit tests that
// drive the TUI synchronously): there is no main loop yet, so we paint inline.
func (t *TUI) Render(usages []domain.ProviderUsage) {
	if t.queueDraw == nil {
		// Run() hasn't started yet (e.g. unit test); paint synchronously.
		t.applyDataset(usages)
		return
	}
	t.queueDraw(func() {
		t.applyDataset(usages)
	})
}

// applyDataset synchronously writes the dataset to allCache and refreshes the
// views. Callers already running on the tview main loop (input-capture handlers,
// modal/form button callbacks) MUST use this instead of Render(): Render routes
// the repaint through QueueUpdateDraw, which blocks until the main loop runs the
// queued func — but the main loop is busy in that very handler, so it deadlocks
// and freezes the UI (the 'p' freeze). Here we mutate directly; tview redraws
// automatically once the handler returns (Run's event loop calls a.draw() right
// after input capture / InputHandler returns).
func (t *TUI) applyDataset(usages []domain.ProviderUsage) {
	t.allCache = usages
	t.applyCacheToViews()
}

func (t *TUI) buildComponents() *TUI {
	t.header = NewAppHeader(t.version, t.commit, RepoURL)
	t.searchBar = NewSearchBar().
		OnSearch(t.handleSearchInput).
		OnEscape(t.blurSearchBar).
		OnNavigate(func() { t.focusList() })
	t.accountList = NewAccountList().
		OnSelectionChange(t.handleSelectionChange).
		OnReturnToSearch(func() { t.app.SetFocus(t.searchBar) })
	t.details = NewAccountDetails()
	t.statusBar = NewStatusBar()
	return t
}

// buildLayout assembles the screen. The 3:2 column ratio is the hard
// requirement from the task-8 brief and is aligned verbatim with
// lazytmux/internal/adapters/ui/tui.go:122-138:
//
//	root = FlexRow{ header(2) · content · statusbar(1) }
//	content = FlexColumn{ left(3) · right(2) }
//	left   = FlexRow{ searchBar(3) · accountList }
//	right  = FlexRow{ accountDetails }
func (t *TUI) buildLayout() *TUI {
	left := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.searchBar, 3, 0, true).
		AddItem(t.accountList, 0, 1, false)
	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.details, 0, 1, false)

	t.content = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 0, 3, true).  // 3 parts — strict
		AddItem(right, 0, 2, false) // 2 parts — strict

	t.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.header, 2, 0, false).
		AddItem(t.content, 0, 1, true).
		AddItem(t.statusBar, 1, 0, false)
	return t
}

func (t *TUI) bindEvents() *TUI {
	t.root.SetInputCapture(t.handleGlobalKeys)
	return t
}

// loadInitialData paints the seed dataset (if any) so the first frame already
// shows accounts instead of the "select an account" placeholder.
func (t *TUI) loadInitialData() *TUI {
	t.applyCacheToViews()
	return t
}

// applyCacheToViews pushes allCache through the search filter into the list and
// refreshes the details pane for the current selection. It is the single render
// entry point used by both Render() (external) and loadInitialData() (startup),
// so search filtering and selection preservation always compose.
func (t *TUI) applyCacheToViews() {
	visible := t.visibleSorted()
	sel := t.selectedID // snapshot: UpdateUsages 的 SetCurrentItem(0) 经 SetChangedFunc 改写 selectedID（#6）
	t.accountList.UpdateUsages(visible)
	if sel != "" {
		t.accountList.SelectByAccountID(sel)
	}
	if u, ok := t.accountList.GetSelected(); ok {
		t.details.Render(u)
	} else if len(visible) > 0 {
		t.details.Render(visible[0])
	} else if len(t.allCache) == 0 {
		t.details.RenderEmpty("no accounts configured")
	} else {
		t.details.RenderEmpty("no matching accounts")
	}
	if len(t.allCache) == 0 {
		t.statusBar.SetText(emptyHints())
	} else {
		t.statusBar.ResetHints()
	}
}

// handleSelectionChange fires when the list cursor moves. We record the id (so a
// later refresh restores it) and paint the details pane.
func (t *TUI) handleSelectionChange(u domain.ProviderUsage) {
	t.selectedID = u.AccountID
	t.details.Render(u)
}

// handleSearchInput re-renders the filtered list on every keystroke. The
// selection is re-resolved after the filter so the details pane tracks the top
// match rather than going stale.
func (t *TUI) handleSearchInput(_ string) {
	visible := t.visibleSorted()
	sel := t.selectedID // snapshot: UpdateUsages 的 SetCurrentItem(0) 经 SetChangedFunc 改写 selectedID（#6）
	t.accountList.UpdateUsages(visible)
	if sel != "" {
		t.accountList.SelectByAccountID(sel)
	}
	if u, ok := t.accountList.GetSelected(); ok {
		t.details.Render(u)
	} else if len(visible) > 0 {
		t.details.Render(visible[0])
	} else {
		t.details.RenderEmpty("no matching accounts")
	}
}

// visibleUsages applies the current search query (case-insensitive substring on
// label or provider) to allCache. Empty query = everything.
func (t *TUI) visibleUsages() []domain.ProviderUsage {
	q := t.currentSearchQuery()
	if q == "" {
		return t.allCache
	}
	needle := strings.ToLower(q)
	out := make([]domain.ProviderUsage, 0, len(t.allCache))
	for _, u := range t.allCache {
		if strings.Contains(strings.ToLower(u.Label), needle) ||
			strings.Contains(strings.ToLower(u.Provider), needle) {
			out = append(out, u)
		}
	}
	return out
}

// visibleSorted 返回过滤后的可见账号并按置顶优先稳定排序（pinned 排前，其余保持原序）。
// 供所有渲染路径（applyCacheToViews/handleSearchInput）共用，确保 pin 状态变化或搜索
// 过滤后，置顶项始终钉在列表顶部。
func (t *TUI) visibleSorted() []domain.ProviderUsage {
	visible := t.visibleUsages()
	sortUsagesForUI(visible, t.sortMode)
	return visible
}

// applySortAndRender updates the list border title to the active mode and
// re-renders. Runs on the tview main loop (input handler), so it repaints
// synchronously rather than via Render/QueueUpdateDraw (which would deadlock).
func (t *TUI) applySortAndRender() {
	t.accountList.SetSortTitle(t.sortMode.String())
	t.applyCacheToViews()
	t.setStatusTemporary("[" + colorCyan + "]Sort: " + t.sortMode.String() + "[-]")
}

func (t *TUI) currentSearchQuery() string {
	if t.searchBar == nil {
		return ""
	}
	return t.searchBar.GetText()
}

// handleGlobalKeys routes top-level keys. When the search bar has focus we pass
// every key through so typing is uninterrupted (the search bar's own capture
// handles ESC/arrows); the list handles its own navigation too.
func (t *TUI) handleGlobalKeys(e *tcell.EventKey) *tcell.EventKey {
	if t.searchBarHasFocus() {
		return e
	}
	switch e.Key() {
	case tcell.KeyTab:
		t.cycleFocus()
		return nil
	case tcell.KeyCtrlC:
		t.app.Stop()
		return nil
	}
	switch e.Rune() {
	case '/':
		t.app.SetFocus(t.searchBar)
		return nil
	case 'q':
		t.app.Stop()
		return nil
	case 'r':
		t.doRefreshSelected()
		return nil
	case 'R':
		t.doRefreshAll()
		return nil
	case '?':
		t.openHelp()
		return nil
	case 'a':
		t.openAccountForm(false)
		return nil
	case 'e':
		t.openAccountForm(true)
		return nil
	case 'd':
		t.confirmDelete()
		return nil
	case 'p':
		t.doTogglePin()
		return nil
	case 's':
		t.sortMode = t.sortMode.Next()
		t.applySortAndRender()
		return nil
	case 'S':
		t.sortMode = t.sortMode.Next().Next()
		t.applySortAndRender()
		return nil
	}
	return e
}

// doRefreshSelected invokes the r callback. If it returns data we re-render;
// otherwise we flash a status so the shell always gives visible feedback.
//
// We snapshot t.selectedID on the main loop (here) and pass the string into the
// callback, which runs on a background goroutine. Reading t.selectedID from that
// goroutine would race with handleSelectionChange's writes on the main loop;
// passing the value keeps the refresh path lock-free and race-clean (same
// discipline Render uses via queueDraw).
func (t *TUI) doRefreshSelected() {
	if t.refreshSelected == nil {
		t.setStatusTemporary("[" + colorYellow + "]Refresh not wired[-]")
		return
	}
	t.setStatusTemporary("[" + colorCyan + "]Refreshing selected…[-]")
	selectedID := t.selectedID
	go func() {
		usages := t.refreshSelected(selectedID)
		if usages != nil {
			t.Render(usages)
		}
		t.queueDraw(func() { t.setStatusTemporary("[" + colorGreen + "]Refreshed selected[-]") })
	}()
}

// doRefreshAll is the R analogue — refresh every account.
func (t *TUI) doRefreshAll() {
	if t.refreshAll == nil {
		t.setStatusTemporary("[" + colorYellow + "]Refresh not wired[-]")
		return
	}
	t.setStatusTemporary("[" + colorCyan + "]Refreshing all…[-]")
	go func() {
		usages := t.refreshAll()
		if usages != nil {
			t.Render(usages)
		}
		t.queueDraw(func() { t.setStatusTemporary(fmt.Sprintf("["+colorGreen+"]Refreshed %d accounts[-]", len(usages))) })
	}()
}

// doTogglePin 切换当前选中账号的置顶状态。回调同步执行（只改配置 + 缓存，不联网），
// 返回的新数据集经 Render 重渲——置顶项经 visibleSorted 排到顶部、📌 marker 随之更新，
// 不做任何特判（与 lazytmux 的"单一 refresh 路径"一致）。
func (t *TUI) doTogglePin() {
	if t.onTogglePin == nil {
		t.setStatusTemporary("[" + colorYellow + "]Pin not wired[-]")
		return
	}
	if t.selectedID == "" {
		t.setStatusTemporary("[" + colorYellow + "]No account selected[-]")
		return
	}
	if usages := t.onTogglePin(t.selectedID); usages != nil {
		// doTogglePin runs on the tview main loop (input-capture handler), so we
		// apply the dataset synchronously rather than via Render() — Render would
		// QueueUpdateDraw and deadlock waiting for the main loop this handler is
		// already occupying (the 'p' freeze). tview redraws after we return.
		t.applyDataset(usages)
	}
}

func (t *TUI) searchBarHasFocus() bool {
	return t.app.GetFocus() == t.searchBar
}

func (t *TUI) cycleFocus() {
	if t.app.GetFocus() == t.accountList {
		t.app.SetFocus(t.details)
	} else {
		t.app.SetFocus(t.accountList)
	}
}

func (t *TUI) focusList() { t.app.SetFocus(t.accountList) }

func (t *TUI) blurSearchBar() {
	t.searchBar.SetText("")
	t.handleSearchInput("")
	t.focusList()
}

// setStatusTemporary shows msg on the footer for statusToastTimeout, then
// restores the default hints (or the empty-state hints when there is no data).
// Any previously-pending timer is stopped first so a rapid sequence of
// statuses does not fight itself.
const statusToastTimeout = 3 * time.Second

// clockTickInterval 是列表相对时间（"Last Refreshed: Xm ago"）的刷新粒度。30s 足以让
// "just now" 在一分钟后推进到 "1m ago"，又不至于频繁重绘整个列表。
const clockTickInterval = 30 * time.Second

func (t *TUI) setStatusTemporary(msg string) {
	if t.statusTimer != nil {
		t.statusTimer.Stop()
	}
	t.statusBar.SetStatus(msg)
	t.statusTimer = time.AfterFunc(statusToastTimeout, func() {
		t.queueDraw(func() {
			if len(t.allCache) == 0 {
				t.statusBar.SetText(emptyHints())
			} else {
				t.statusBar.ResetHints()
			}
		})
	})
}
