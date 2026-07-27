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
type RefreshAllFunc func() []domain.VendorUsage

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
type RefreshSelectedFunc func(accountID string) []domain.VendorUsage

// Config parameterizes the TUI. RefreshSelected/RefreshAll are optional: when
// nil, r/R flash a "not wired" status instead of no-op-ing silently, so a
// misconfigured assembly makes its own wiring state obvious during smoke.
type Config struct {
	Logger          *zap.SugaredLogger
	Version         string
	Commit          string
	InitialData     []domain.VendorUsage
	RefreshSelected RefreshSelectedFunc // r — refresh the currently-selected account
	RefreshAll      RefreshAllFunc      // R — refresh every account
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
	allCache []domain.VendorUsage
	// selectedID is preserved across refreshes so the user's selection survives
	// a re-render (mirrors lazytmux's SelectByName-after-UpdateSessions flow).
	selectedID string

	refreshSelected RefreshSelectedFunc
	refreshAll      RefreshAllFunc

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
		allCache:        cfg.InitialData,
		refreshSelected: cfg.RefreshSelected,
		refreshAll:      cfg.RefreshAll,
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
func (t *TUI) Render(usages []domain.VendorUsage) {
	if t.queueDraw == nil {
		// Run() hasn't started yet (e.g. unit test); paint synchronously.
		t.allCache = usages
		t.applyCacheToViews()
		return
	}
	t.queueDraw(func() {
		t.allCache = usages
		t.applyCacheToViews()
	})
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
	visible := t.visibleUsages()
	t.accountList.UpdateUsages(visible)
	if t.selectedID != "" {
		t.accountList.SelectByAccountID(t.selectedID)
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
func (t *TUI) handleSelectionChange(u domain.VendorUsage) {
	t.selectedID = u.AccountID
	t.details.Render(u)
}

// handleSearchInput re-renders the filtered list on every keystroke. The
// selection is re-resolved after the filter so the details pane tracks the top
// match rather than going stale.
func (t *TUI) handleSearchInput(_ string) {
	visible := t.visibleUsages()
	t.accountList.UpdateUsages(visible)
	if t.selectedID != "" {
		t.accountList.SelectByAccountID(t.selectedID)
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
// label or vendor) to allCache. Empty query = everything.
func (t *TUI) visibleUsages() []domain.VendorUsage {
	q := t.currentSearchQuery()
	if q == "" {
		return t.allCache
	}
	needle := strings.ToLower(q)
	out := make([]domain.VendorUsage, 0, len(t.allCache))
	for _, u := range t.allCache {
		if strings.Contains(strings.ToLower(u.Label), needle) ||
			strings.Contains(strings.ToLower(u.Vendor), needle) {
			out = append(out, u)
		}
	}
	return out
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
	case 'a', 'e', 'd', 's':
		// CRUD/sort are out of scope for the UI shell (Task 9/12 wiring); surface
		// that explicitly rather than swallowing the key silently.
		t.setStatusTemporary("[" + colorYellow + "]not wired yet[-]")
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
		t.setStatusTemporary("[" + colorYellow + "]refresh not wired[-]")
		return
	}
	t.setStatusTemporary("[" + colorCyan + "]refreshing selected…[-]")
	selectedID := t.selectedID
	go func() {
		usages := t.refreshSelected(selectedID)
		if usages != nil {
			t.Render(usages)
		}
		t.queueDraw(func() { t.setStatusTemporary("[" + colorGreen + "]refreshed selected[-]") })
	}()
}

// doRefreshAll is the R analogue — refresh every account.
func (t *TUI) doRefreshAll() {
	if t.refreshAll == nil {
		t.setStatusTemporary("[" + colorYellow + "]refresh not wired[-]")
		return
	}
	t.setStatusTemporary("[" + colorCyan + "]refreshing all…[-]")
	go func() {
		usages := t.refreshAll()
		if usages != nil {
			t.Render(usages)
		}
		t.queueDraw(func() { t.setStatusTemporary(fmt.Sprintf("["+colorGreen+"]refreshed %d accounts[-]", len(usages))) })
	}()
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
