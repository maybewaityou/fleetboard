# fleetboard Optimizations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship 8 optimizations — arrow-key focus, working sort, nearest-window list %, placeholder color, English UI, `vendor`→`provider` rename, Homebrew release pipeline, bilingual README.

**Architecture:** Faithful port of sibling project `lazytmux`'s patterns (same stack) + a mechanical domain rename + i18n + release/docs scaffolding. No architectural changes to the hexagonal layout.

**Tech Stack:** Go 1.24, cobra, tview/tcell, zap, Tokyo Night palette; goreleaser v2 + GitHub Actions for release; Homebrew tap `maybewaityou/homebrew-tap`.

**Spec:** `docs/superpowers/specs/2026-07-27-optimizations-design.md`

## Global Constraints

- Go module: `github.com/maybewaityou/fleetboard`; binary `fleetboard`; GitHub `maybewaityou/fleetboard`.
- Palette is single-sourced in `internal/adapters/ui/const.go` (Tokyo Night); placeholder color = `colorSecondary` (`#565f89`).
- UI strings are English; only API-returned Chinese is kept. Code comments stay Chinese (Task 7 is about the running app, not source).
- Provider slug VALUES (`"glm"`,`"kimi"`,`"deepseek"`,`"minimax"`) and the registry map keys are NOT renamed — only identifiers/fields/types/YAML-key.
- Each task ends with `go build ./...` green; run `make test` (`go test -race -cover ./...`) and `make quality` at least at the end of the UI tasks and at the very end.
- Commit style matches repo: `type(scope): desc`; end messages with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. (Confirm commit cadence with the user before committing if unsure.)

---

## File Structure

- `internal/core/domain/account.go` — `Account.Vendor`→`Provider` field (Task 1).
- `internal/core/domain/vendor_usage.go` — `VendorUsage`→`ProviderUsage` type + `Vendor`→`Provider` field (Task 1).
- `internal/core/ports/usage_provider.go` — `Vendor()`→`Provider()` method, `Get(vendor)`→`Get(provider)` (Task 1).
- `internal/core/services/aggregator.go` — `ErrUnknownVendor`→`ErrUnknownProvider` (Task 1).
- `internal/adapters/providers/` — registry + glm/minimax/kimi/deepseek/mock: method rename + `Vendor:`→`Provider:` (Task 1); GLM/kimi/deepseek English names (Task 6).
- `internal/adapters/config/yaml/store.go` + tests — YAML key `vendor:`→`provider:` (Task 1).
- `internal/adapters/ui/account_list.go` — `displayDimension`/`displayPercent` (Task 2); rename identifiers (Task 1).
- `internal/adapters/ui/sort.go` — NEW, pure sort (Task 3).
- `internal/adapters/ui/tui.go` — arrows, sort wiring, rename (Tasks 1,3,4).
- `internal/adapters/ui/status_bar.go` — footer `←/→` hint (Task 4); rename (Task 1).
- `internal/adapters/ui/keybindings.go` — add sort binding (Task 3); rename (Task 1).
- `internal/adapters/ui/help.go` — English title (Task 6); rename (Task 1).
- `internal/adapters/ui/account_form.go` — placeholder color (Task 5), English (Task 6), rename (Task 1).
- `internal/adapters/ui/const.go`, `theme.go`, `account_details.go`, `handlers.go` — rename identifiers (Task 1).
- `.goreleaser.yaml` — NEW (Task 7).
- `.github/workflows/release.yml` — NEW (Task 7).
- `README.md`, `README.zh-CN.md` — NEW (Task 8).
- `LICENSE` — NEW Apache-2.0 (Task 8).
- `docs/resources/donate-wechat.jpg`, `docs/resources/donate-alipay.jpg` — copied from lazytmux (Task 8).

---

## Task 1: Rename `vendor` → `provider` everywhere (spec §6)

Do this FIRST so later tasks edit the final identifiers.

**Files:** all `.go` under `internal/` and `cmd/` (prod + `*_test.go`).

**Interfaces:**
- Produces: `domain.Account.Provider` (`yaml:"provider"`), type `domain.ProviderUsage` (field `Provider`), `ports.UsageProvider.Provider()`, `services.ErrUnknownProvider`, `providers.Registry.byProvider`, `ui.providerColor`/`ProviderTag`/`providerOptions`/`providerInfoLine`/`afFieldProvider`/`unknownProviderBG`/`unknownProviderFG`, mock `ProviderName`.

- [ ] **Step 1: Verify no unexpected `vendor` substrings**

Run:
```bash
cd /home/either/workspace/repos/tools/fleetboard
grep -rni "vendor" --include="*.go" internal cmd | grep -vi "provider"
```
Expected: every hit is the concept being renamed (field/method/type/map/identifier/comment) or a YAML `vendor:` tag. There must be NO provider slug value containing "vendor" (slugs are glm/kimi/deepseek/minimax — confirmed none). If any unrelated word contains "vendor", stop and handle it manually.

- [ ] **Step 2: Capital pass — `Vendor` → `Provider`**

Run:
```bash
grep -rl "" --include="*.go" internal cmd | xargs sed -i 's/Vendor/Provider/g'
```
This renames: `Vendor`→`Provider` (field), `VendorUsage`→`ProviderUsage` (type), `VendorTag`→`ProviderTag`, `ErrUnknownVendor`→`ErrUnknownProvider`, `byVendor`→`byProvider`, `afFieldVendor`→`afFieldProvider`, `unknownVendorBG/FG`→`unknownProviderBG/FG`, `VendorName`→`ProviderName` (mock), `Vendor()`→`Provider()` (interface + impls).

- [ ] **Step 3: Lowercase pass — `vendor` → `provider`**

Run:
```bash
grep -rl "" --include="*.go" internal cmd | xargs sed -i 's/vendor/provider/g'
```
This renames: `vendorColor`→`providerColor`, `vendorOptions`→`providerOptions`, `vendorInfoLine`→`providerInfoLine`, `vendorDropDown`→`providerDropDown`, the YAML tag `yaml:"vendor"`→`yaml:"provider"`, the `Get(vendor)`/`New(vendor)` params, `acc.Vendor`→`acc.Provider` call sites, and Chinese comments mentioning vendor. (The package dir `providers` and words `UsageProvider`/`ProviderLookup` contain no `vendor` substring, so they are untouched.)

- [ ] **Step 4: Build to catch anything the compiler disagrees with**

Run: `go build ./...`
Expected: PASS. If FAIL (e.g. a test-only identifier or a YAML-fixture string), read the error, fix that spot, rebuild.

- [ ] **Step 5: Find remaining `vendor` in YAML test fixtures / error strings**

Run:
```bash
grep -rni "vendor" --include="*.go" internal cmd
grep -rn "unknown vendor" --include="*_test.go" internal
```
Expected: zero hits. Any leftover (e.g. a test asserting the substring `"unknown vendor"`, or a hardcoded `vendor:` YAML literal in a store test) must be updated to `provider`/`"unknown provider"`.

- [ ] **Step 6: Run tests**

Run: `go test ./...`
Expected: PASS. Fix any golden/assertion fallout from the rename (most tests reference the renamed identifiers uniformly via the same sed, so they should already compile and pass).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: rename vendor -> provider (field, type, method, YAML key)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: List shows the nearest-reset window's % (spec §3, user Task 5)

**Files:**
- Modify: `internal/adapters/ui/account_list.go` (replace `primaryPercent`; add `displayDimension`/`displayPercent`; rewrite the percent/dot block of `formatAccountLine`).
- Modify: `internal/adapters/ui/account_list_test.go` (rename `TestPrimaryPercent`→`TestDisplayPercent`, call `displayPercent`).

**Interfaces:**
- Produces: `displayDimension(u domain.ProviderUsage) *domain.UsageDimension`, `displayPercent(u domain.ProviderUsage) float64`. (Task 3's `sortUsagesForUI` consumes `displayPercent`.)

- [ ] **Step 1: Write the failing test for `displayDimension`**

Append to `internal/adapters/ui/account_list_test.go`:
```go
// TestDisplayDimension_NearestReset verifies the list surfaces the dimension
// whose ResetsAt is soonest (the nearest reset window), not the max-% one.
func TestDisplayDimension_NearestReset(t *testing.T) {
	now := time.Now()
	weekly := domain.UsageDimension{Name: "Weekly", PercentUsed: 80, ResetsAt: now.Add(7 * 24 * time.Hour)}
	fiveH := domain.UsageDimension{Name: "5h", PercentUsed: 30, ResetsAt: now.Add(5 * time.Hour)}
	u := domain.ProviderUsage{
		Provider:   "glm",
		Dimensions: []domain.UsageDimension{weekly, fiveH},
	}
	d := displayDimension(u)
	if d.Name != "5h" {
		t.Errorf("displayDimension = %q, want the soonest-reset \"5h\"", d.Name)
	}
	if got := displayPercent(u); got != 30 {
		t.Errorf("displayPercent = %v, want 30 (5h window), not 80 (weekly)", got)
	}
}

// TestDisplayDimension_FallbackPrimary verifies balance providers (no ResetsAt)
// fall back to Primary so the balance still shows.
func TestDisplayDimension_FallbackPrimary(t *testing.T) {
	bal := domain.UsageDimension{Name: "Available balance", Balance: 5, Currency: "CNY", PercentUsed: -1}
	u := domain.ProviderUsage{
		Provider: "kimi",
		Dimensions: []domain.UsageDimension{bal},
		Primary:   &bal,
	}
	d := displayDimension(u)
	if d.Name != "Available balance" {
		t.Errorf("fallback = %q, want Primary", d.Name)
	}
	if got := displayPercent(u); got != -1 {
		t.Errorf("balance displayPercent = %v, want -1", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/ui/ -run TestDisplayDimension -v`
Expected: FAIL (`undefined: displayDimension`).

- [ ] **Step 3: Implement `displayDimension` + `displayPercent`, replacing `primaryPercent`**

In `internal/adapters/ui/account_list.go`, REPLACE the existing `primaryPercent` function with:
```go
// displayDimension returns the dimension shown in the list: the one with the
// soonest non-zero ResetsAt (the nearest reset window — "最近时间"), falling back
// to Primary when no dimension carries a reset time (balance providers such as
// kimi/deepseek), then nil.
func displayDimension(u domain.ProviderUsage) *domain.UsageDimension {
	var nearest *domain.UsageDimension
	for i := range u.Dimensions {
		d := &u.Dimensions[i]
		if d.ResetsAt.IsZero() {
			continue
		}
		if nearest == nil || d.ResetsAt.Before(nearest.ResetsAt) {
			nearest = d
		}
	}
	if nearest != nil {
		return nearest
	}
	return u.Primary
}

// displayPercent is the usage key shared by the list dot/bar and the Usage-sort
// mode: the displayed dimension's PercentUsed, or -1 (N/A) when there is none.
func displayPercent(u domain.ProviderUsage) float64 {
	if d := displayDimension(u); d != nil {
		return d.PercentUsed
	}
	return -1
}
```

- [ ] **Step 4: Rewrite the percent/dot block of `formatAccountLine` to use `displayDimension`**

In `formatAccountLine`, REPLACE the block that reads `u.Primary` (the `pctStr, dot := "N/A", "○"` … `dotCol = StatusColor(u.Primary.PercentUsed)` block) with:
```go
	d := displayDimension(u)
	pctStr, dot := "N/A", "○"
	dotCol := colorGray // N/A 默认灰点
	if d != nil && d.Currency != "" {
		// 余额型：显示余额 + 绿/红点（按余额正负）
		pctStr = formatMoneyShort(d.Balance, d.Currency)
		dot = "●"
		if d.Balance > 0 {
			dotCol = colorGreen
		} else {
			dotCol = colorRed
		}
	} else if d != nil {
		// 配额型：百分比 + StatusColor
		pctStr = fmt.Sprintf("%d%%", int(d.PercentUsed))
		dot = "●"
		dotCol = StatusColor(d.PercentUsed)
	}
	pct := displayPercent(u) // 余额型 PercentUsed=-1 → renderBar(-1,4) 自然灰条
```
Also update the `icon`/`label` lines that still reference `u.Primary`? — they don't; they reference `u.Provider`/`u.Label`. The `pin`/`fetched` lines are unchanged. Leave the `return fmt.Sprintf(...)` as-is (it uses `pct`, `pctStr`, `dot`, `dotCol`, `iconFg`, `icon`, `u.Provider`, `padDisplay(label,16)`, `fetched`).

- [ ] **Step 5: Rename the old `primaryPercent` test**

In `account_list_test.go`, rename `TestPrimaryPercent` → `TestDisplayPercent` and change its body to call `displayPercent`:
```go
func TestDisplayPercent(t *testing.T) {
	if got := displayPercent(domain.ProviderUsage{}); got != -1 {
		t.Errorf("nil = %v, want -1", got)
	}
	u := domain.ProviderUsage{Primary: &domain.UsageDimension{PercentUsed: 42.5}}
	if got := displayPercent(u); got != 42.5 {
		t.Errorf("Primary.PercentUsed = %v, want 42.5", got)
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/adapters/ui/ -run "TestDisplay|TestFormatAccountLine" -v`
Expected: PASS (the existing `TestFormatAccountLine_*` tests set only `Primary` with empty `Dimensions`, so `displayDimension` falls back to `Primary` and outputs are unchanged).

- [ ] **Step 7: Build + full test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(ui): list shows nearest-reset window percent

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: Working sort `s`/`S` (spec §2, user Task 2)

**Files:**
- Create: `internal/adapters/ui/sort.go`.
- Create: `internal/adapters/ui/sort_test.go`.
- Modify: `internal/adapters/ui/tui.go` (add `sortMode` field + init, replace `s` handler, add `S`, add `applySortAndRender`, change `visibleSorted`, set initial sort title).
- Modify: `internal/adapters/ui/account_list.go` (add `SetSortTitle`).
- Modify: `internal/adapters/ui/keybindings.go` (add sort binding).

**Interfaces:**
- Consumes: `displayPercent` (Task 2).
- Produces: `SortMode`, `SortByNameAsc`/`SortByUsageDesc`/`SortByRefreshedDesc`, `sortUsagesForUI`.

- [ ] **Step 1: Write the failing sort test**

Create `internal/adapters/ui/sort_test.go`:
```go
package ui

import (
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func TestSortMode_Next(t *testing.T) {
	if got := SortByNameAsc.Next(); got != SortByUsageDesc {
		t.Errorf("Name.Next = %v, want Usage", got)
	}
	if got := SortByRefreshedDesc.Next(); got != SortByNameAsc {
		t.Errorf("Refreshed.Next = %v, want Name (wrap)", got)
	}
	if s := SortByNameAsc.Next().Next(); s != SortByRefreshedDesc {
		t.Errorf("Name.Next.Next = %v, want Refreshed", s)
	}
}

func TestSortUsagesByName(t *testing.T) {
	accts := []domain.ProviderUsage{{Label: "b"}, {Label: "a"}}
	sortUsagesForUI(accts, SortByNameAsc)
	if accts[0].Label != "a" || accts[1].Label != "b" {
		t.Errorf("Name asc order = %s,%s, want a,b", accts[0].Label, accts[1].Label)
	}
}

func TestSortUsagesByUsage(t *testing.T) {
	soon := time.Now().Add(time.Hour)
	accts := []domain.ProviderUsage{
		{Label: "lo", Dimensions: []domain.UsageDimension{{PercentUsed: 10, ResetsAt: soon}}},
		{Label: "hi", Dimensions: []domain.UsageDimension{{PercentUsed: 90, ResetsAt: soon}}},
	}
	sortUsagesForUI(accts, SortByUsageDesc)
	if accts[0].Label != "hi" {
		t.Errorf("Usage desc top = %s, want hi", accts[0].Label)
	}
}

func TestSortUsagesByRefreshed(t *testing.T) {
	now := time.Now()
	accts := []domain.ProviderUsage{
		{Label: "old", FetchedAt: now.Add(-2 * time.Hour)},
		{Label: "new", FetchedAt: now},
	}
	sortUsagesForUI(accts, SortByRefreshedDesc)
	if accts[0].Label != "new" {
		t.Errorf("Refreshed desc top = %s, want new", accts[0].Label)
	}
}

func TestSortUsagesPinnedFloat(t *testing.T) {
	accts := []domain.ProviderUsage{
		{Label: "z-pinned", Pinned: true},
		{Label: "a", Pinned: false},
	}
	sortUsagesForUI(accts, SortByNameAsc) // Name asc would put "a" first, but pinned wins
	if accts[0].Label != "z-pinned" {
		t.Errorf("pinned must float to top, got %s", accts[0].Label)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/ui/ -run TestSort -v`
Expected: FAIL (`undefined: SortByNameAsc`, `sortUsagesForUI`).

- [ ] **Step 3: Create `sort.go`**

Create `internal/adapters/ui/sort.go`:
```go
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

// sort.go 定义列表排序模式，移植自 lazytmux sort.go：一个小枚举，由 s/S 键
// 循环切换；置顶账号始终浮顶。比较器复用 account_list.displayPercent，保证
// 排序键与列表展示的是同一档（最近重置窗口）。
package ui

import (
	"sort"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// SortMode 是列表的排序模式。
type SortMode int

const (
	SortByNameAsc SortMode = iota // Name ↑
	SortByUsageDesc               // Usage % ↓
	SortByRefreshedDesc           // Last Refreshed ↓
)

// String 返回展示标签（列表标题与排序 toast 共用）。
func (s SortMode) String() string {
	switch s {
	case SortByUsageDesc:
		return "Usage ↓"
	case SortByRefreshedDesc:
		return "Refreshed ↓"
	default:
		return "Name ↑"
	}
}

// Next 前进一步，末尾回绕。
func (s SortMode) Next() SortMode { return (s + 1) % 3 }

// sortUsagesForUI 原地稳定排序：置顶优先，组内按 mode。稳定排序让等键行保持原序。
func sortUsagesForUI(usages []domain.ProviderUsage, mode SortMode) {
	sort.SliceStable(usages, func(i, j int) bool {
		if usages[i].Pinned != usages[j].Pinned {
			return usages[i].Pinned
		}
		switch mode {
		case SortByUsageDesc:
			return displayPercent(usages[i]) > displayPercent(usages[j])
		case SortByRefreshedDesc:
			return usages[i].FetchedAt.After(usages[j].FetchedAt)
		default:
			return usages[i].Label < usages[j].Label
		}
	})
}
```

- [ ] **Step 4: Run sort tests to verify they pass**

Run: `go test ./internal/adapters/ui/ -run TestSort -v`
Expected: PASS.

- [ ] **Step 5: Wire sort into the TUI**

In `internal/adapters/ui/tui.go`:

(a) Add a field to the `TUI` struct (near `selectedID`):
```go
	// sortMode is the active list sort, cycled by s/S.
	sortMode SortMode
```

(b) In `NewTUI`, initialize it (add to the returned struct literal):
```go
		sortMode: SortByNameAsc,
```

(c) In `handleGlobalKeys`, REPLACE the `case 's':` block that prints "Sort not wired yet" with:
```go
	case 's':
		t.sortMode = t.sortMode.Next()
		t.applySortAndRender()
		return nil
	case 'S':
		t.sortMode = t.sortMode.Next().Next()
		t.applySortAndRender()
		return nil
```

(d) REPLACE `visibleSorted` with:
```go
func (t *TUI) visibleSorted() []domain.ProviderUsage {
	visible := t.visibleUsages()
	sortUsagesForUI(visible, t.sortMode)
	return visible
}
```
(The old pin-only `sort.SliceStable` is now inside `sortUsagesForUI`. The `sort` import stays used.)

(e) Add `applySortAndRender` (near `visibleSorted`):
```go
// applySortAndRender updates the list title to the active mode and re-renders.
// Runs on the tview main loop (input handler), so it repaints synchronously.
func (t *TUI) applySortAndRender() {
	t.accountList.SetSortTitle(t.sortMode.String())
	t.applyCacheToViews()
	t.setStatusTemporary("[" + colorCyan + "]Sort: " + t.sortMode.String() + "[-]")
}
```

(f) In `Run()`, after `t.buildComponents().buildLayout().bindEvents().loadInitialData()` and before `t.app.SetRoot(t.root, true)`, set the initial sort title:
```go
	t.accountList.SetSortTitle(t.sortMode.String())
```

- [ ] **Step 6: Add `SetSortTitle` to AccountList**

In `internal/adapters/ui/account_list.go`, add:
```go
// SetSortTitle writes the active sort mode into the list border title.
func (al *AccountList) SetSortTitle(mode string) {
	al.List.SetTitle(" Accounts — Sort: " + mode + " ")
}
```

- [ ] **Step 7: Add the sort binding to the help/footer source of truth**

In `internal/adapters/ui/keybindings.go`, add a line in the `Usage` group (after the `R` Refresh all line, before `Other`):
```go
	{"Usage", "s/S", "Cycle sort"},
```

- [ ] **Step 8: Build + test**

Run: `go build ./... && go test ./...`
Expected: PASS. (If `help_test.go` golden output changed because of the new binding, update the expected string in that test.)

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat(ui): wire sort s/S (name/usage/refreshed, pinned float)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: Arrow-key ←/→ focus switching + footer hint (spec §1, user Task 1)

**Files:**
- Modify: `internal/adapters/ui/tui.go` (add `KeyRight`/`KeyLeft` to `handleGlobalKeys`; add `listHasFocus`/`detailsHasFocus`/`focusDetails`; switch `searchBarHasFocus`/`cycleFocus` to `HasFocus()`).
- Modify: `internal/adapters/ui/status_bar.go` (add `←/→ Focus` to `defaultHints`).

**Interfaces:** none new.

- [ ] **Step 1: Add arrow handling to `handleGlobalKeys`**

In `internal/adapters/ui/tui.go`, in `handleGlobalKeys`, the `switch e.Key()` block currently has `KeyTab` and `KeyCtrlC`. Add two cases:
```go
	case tcell.KeyRight:
		// List → Details: the list's own capture does NOT swallow Right, so it
		// bubbles up here (mirrors lazytmux).
		if t.listHasFocus() {
			t.focusDetails()
			return nil
		}
	case tcell.KeyLeft:
		// Details → List.
		if t.detailsHasFocus() {
			t.focusList()
			return nil
		}
```

- [ ] **Step 2: Add focus helpers and switch to `HasFocus()`**

In `internal/adapters/ui/tui.go`, REPLACE the existing `searchBarHasFocus` and `cycleFocus` and add the new helpers so the set reads:
```go
func (t *TUI) searchBarHasFocus() bool { return t.searchBar.HasFocus() }
func (t *TUI) listHasFocus() bool      { return t.accountList.HasFocus() }
func (t *TUI) detailsHasFocus() bool   { return t.details.HasFocus() }

func (t *TUI) cycleFocus() {
	if t.listHasFocus() {
		t.focusDetails()
	} else {
		t.focusList()
	}
}

func (t *TUI) focusList()    { t.app.SetFocus(t.accountList) }
func (t *TUI) focusDetails() { t.app.SetFocus(t.details) }
```
(Remove the old `focusList`-only definition if it duplicates; keep exactly one `focusList` and add `focusDetails`.)

- [ ] **Step 3: Add `←/→` to the footer**

In `internal/adapters/ui/status_bar.go`, in `defaultHints`, insert a `←/→ Focus` term right after the Navigate term:
```go
func defaultHints() string {
	k := colorCyan
	return "[" + k + "]↑↓[-] Navigate  • " +
		"[" + k + "]←/→[-] Focus  • " +
		"[" + k + "]a[-] New  • " +
		"[" + k + "]e[-] Edit  • " +
		"[" + k + "]d[-] Delete  • " +
		"[" + k + "]p[-] Pin  • " +
		"[" + k + "]r[-] Refresh  • " +
		"[" + k + "]R[-] Refresh All  • " +
		"[" + k + "]/[-] Search  • " +
		"[" + k + "]s[-] Sort  • " +
		"[" + k + "]?[-] Help  • " +
		"[" + k + "]q[-] Quit"
}
```

- [ ] **Step 4: Build + test**

Run: `go build ./... && go test ./...`
Expected: PASS. (If any test asserts the exact `defaultHints` string, update it; grep first: `grep -rn "Navigate" --include="*_test.go" internal/adapters/ui`.)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(ui): arrow keys ←/→ focus list/details + footer hint

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 5: Placeholder color (spec §4, user Task 6)

**Files:**
- Modify: `internal/adapters/ui/account_form.go` (add `SetPlaceholderTextColor(colorSecondary)` to each InputField).

- [ ] **Step 1: Set the placeholder text color on each InputField**

In `internal/adapters/ui/account_form.go`, the placeholder block currently reads:
```go
	f.input(afFieldLabel).SetPlaceholder(phLabel)
	f.providerDropDown().SetTextOptions("", "", "", "", phProvider)
	f.providerDropDown().SetCurrentOption(-1)
	f.input(afFieldBaseURL).SetPlaceholder(phBaseURL)
	f.input(afFieldTokenEnv).SetPlaceholder(phTokenEnv)
```
REPLACE with (adds `SetPlaceholderTextColor` to the three InputFields; the DropDown's noSelection text inherits `TertiaryTextColor=colorSecondary` from `initializeTheme`, so it needs nothing):
```go
	f.input(afFieldLabel).SetPlaceholder(phLabel).SetPlaceholderTextColor(tcell.GetColor(colorSecondary))
	f.providerDropDown().SetTextOptions("", "", "", "", phProvider)
	f.providerDropDown().SetCurrentOption(-1)
	f.input(afFieldBaseURL).SetPlaceholder(phBaseURL).SetPlaceholderTextColor(tcell.GetColor(colorSecondary))
	f.input(afFieldTokenEnv).SetPlaceholder(phTokenEnv).SetPlaceholderTextColor(tcell.GetColor(colorSecondary))
```
(`tcell` is already imported in this file.)

- [ ] **Step 2: Build + test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(ui): form placeholder color matches lazytmux (colorSecondary)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 6: English UI strings (spec §5, user Task 7)

**Files:**
- Modify: `internal/adapters/ui/account_form.go` (placeholders + submit hint).
- Modify: `internal/adapters/ui/help.go` (title close hint).
- Modify: `internal/adapters/providers/glm/glm.go` (dimension names + unit).
- Modify: `internal/adapters/providers/kimi/kimi.go` (balance name).
- Modify: `internal/adapters/providers/deepseek/deepseek.go` (balance name).
- Modify: any `*_test.go` golden asserting the old Chinese strings.

- [ ] **Step 1: Translate the form placeholders + hint**

In `internal/adapters/ui/account_form.go`, REPLACE the placeholder constants:
```go
const (
	phLabel    = "e.g. GLM main"
	phProvider = "Select provider"
	phBaseURL  = "leave empty for default"
	phTokenEnv = "e.g. GLM_API_KEY"
)
```
And in `Primitive()`, REPLACE the hint text:
```go
		SetText("[" + colorSecondary + "]Enter submit · ESC cancel[-]")
```

- [ ] **Step 2: Translate the help title close hint**

In `internal/adapters/ui/help.go`:
```go
	title := helpTextView("[" + colorAccent + "::b]fleetboard — Key Bindings  (Esc / ? / q to close)[-]")
```

- [ ] **Step 3: Translate GLM dimension names + unit**

In `internal/adapters/providers/glm/glm.go`, REPLACE:
```go
	unitCount   = "uses"

	nameTokens5h     = "5h Quota"
	nameTokensWeekly = "Weekly Quota"
	nameTimeMonthly  = "MCP Monthly"
```
(`unitPercent = "%"` unchanged.) And in `buildDimensions`, the fallback name:
```go
		name := fmt.Sprintf("Quota #%d", i+1)
```

- [ ] **Step 4: Translate kimi + deepseek balance names**

In `internal/adapters/providers/kimi/kimi.go`:
```go
	nameAvailable = "Available balance"
```
In `internal/adapters/providers/deepseek/deepseek.go`:
```go
	nameAvailable = "Available balance"
```

- [ ] **Step 5: Find and fix tests asserting old Chinese strings**

Run:
```bash
grep -rn "5小时额度\|每周额度\|MCP每月\|额度#\|可用余额\|选择厂商\|留空使用默认\|智谱编码-主力\|Enter 提交\|q 关闭" --include="*_test.go" internal
```
For each hit, update the expected string to its English equivalent from Steps 1–4 (`5h Quota`, `Weekly Quota`, `MCP Monthly`, `Quota #`, `Available balance`, `Select provider`, `leave empty for default`, `e.g. GLM main`, `Enter submit · ESC cancel`, `q to close`).

- [ ] **Step 6: Build + test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(ui): English UI strings (forms, help, provider dimension names)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 7: Homebrew release pipeline (spec §7, user Task 3)

**Files:**
- Create: `.goreleaser.yaml`.
- Create: `.github/workflows/release.yml`.

- [ ] **Step 1: Create `.goreleaser.yaml`**

Create `/home/either/workspace/repos/tools/fleetboard/.goreleaser.yaml`:
```yaml
version: 2

project_name: fleetboard

before:
  hooks:
    - go mod tidy

builds:
  - main: ./cmd
    ldflags:
      - -s -w -X main.version={{.Version}} -X main.gitCommit={{.ShortCommit}}
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin]
    goarch: [amd64, arm64]

archives:
  - formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

changelog:
  use: github

# Auto-generate the Homebrew formula and push it to the personal tap repo.
# Triggered by `goreleaser release` (see .github/workflows/release.yml).
brews:
  - name: fleetboard
    repository:
      owner: maybewaityou
      name: homebrew-tap
      # A cross-repo PAT is required: the default GITHUB_TOKEN cannot write to
      # another repository. Set HOMEBREW_TAP_GITHUB_TOKEN in CI.
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    homepage: https://github.com/maybewaityou/fleetboard
    description: "Terminal dashboard for AI coding-plan usage & balance across providers."
    license: Apache-2.0
    directory: Formula
    test: |
      system "#{bin}/fleetboard", "--help"
```

- [ ] **Step 2: Create the release workflow**

Create `/home/either/workspace/repos/tools/fleetboard/.github/workflows/release.yml`:
```yaml
name: release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          # Writes the GitHub Release + archives to THIS repo.
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          # Writes the formula to the homebrew-tap repo (needs a PAT with
          # contents:write on maybewaityou/homebrew-tap).
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

- [ ] **Step 3: Validate the goreleaser config (if goreleaser is installed)**

Run: `goreleaser check 2>/dev/null && echo OK || echo "goreleaser not installed — skipping (config is ported from lazytmux's working .goreleaser.yaml)"`
Expected: `OK`, or the skip message. If `goreleaser check` reports errors, fix them.

- [ ] **Step 4: Verify the snapshot build still works**

Run: `make build-all 2>/dev/null && echo OK || echo "goreleaser not installed — skipping snapshot"`
Expected: `OK` (builds `dist/`), or the skip message.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(ci): goreleaser + homebrew tap release workflow

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

> **Manual step for the user (document in README):** set the repo secret `HOMEBREW_TAP_GITHUB_TOKEN` = a PAT with `contents:write` on `maybewaityou/homebrew-tap`. Without it, tagging a release will publish the GitHub Release but fail to push the formula to the tap.

---

## Task 8: README (en + zh) + LICENSE + donate assets (spec §8, user Task 4)

**Files:**
- Create: `README.md`.
- Create: `README.zh-CN.md`.
- Create: `LICENSE` (Apache-2.0).
- Create: `docs/resources/donate-wechat.jpg`, `docs/resources/donate-alipay.jpg` (copied from lazytmux).

- [ ] **Step 1: Copy the donate QR images**

Run:
```bash
mkdir -p docs/resources
cp /home/either/workspace/repos/tools/lazytmux/docs/resources/donate-wechat.jpg docs/resources/
cp /home/either/workspace/repos/tools/lazytmux/docs/resources/donate-alipay.jpg docs/resources/
ls -l docs/resources/
```
Expected: both JPGs present.

- [ ] **Step 2: Create the Apache-2.0 LICENSE**

Run:
```bash
cp /home/either/workspace/repos/tools/lazytmux/LICENSE LICENSE
head -1 LICENSE
```
Expected: first line is the Apache License identifier. (lazytmux's LICENSE is Apache-2.0 and matches fleetboard's file headers.) If lazytmux's LICENSE has a copyright line specific to lazytmux, leave the standard Apache-2.0 text — it has no author-specific line by default.

- [ ] **Step 3: Create `README.md` (English)**

Create `/home/either/workspace/repos/tools/fleetboard/README.md`:
````markdown
<div align="center">

# fleetboard

**A terminal dashboard for AI coding-plan usage &amp; balance across providers.**

See quota and balance for GLM, MiniMax, Kimi, DeepSeek (and more) in one screen —
how much is used, when it resets, and which account still has headroom.

[English](README.md) · [简体中文](README.zh-CN.md)

</div>

## ✨ Features

- **One screen, all providers** — each account is a row: label, provider chip, usage %, status dot.
- **Quota + balance** — percentage windows (GLM 5h/weekly/monthly, MiniMax) **and** account balance (Kimi, DeepSeek).
- **Nearest-window priority** — the list surfaces the quota window that resets soonest, so the most urgent tier is always visible.
- **Two refresh granularities** — `r` re-fetches the selected account, `R` re-fetches all.
- **Manual CRUD** — add / edit / delete / pin accounts; config lives in `~/.fleetboard/config.yaml`.
- **Search & sort** — `/` to filter, `s`/`S` to cycle sort (name / usage / refreshed).
- **Tokyo Night themed** TUI, ported from the `lazytmux` / `lazyssh` tool family.

## 🔒 How it works

fleetboard reads your account config, calls each provider's official usage/balance API, normalizes the result, and renders it. Tokens are read from environment variables (named per account) and never written to disk or sent anywhere except the provider's own API. Local parsing of `~/.claude/` usage files is intentionally out of scope — the server is the source of truth.

## 📦 Installation

### Option 1: Homebrew (macOS)

```bash
brew install maybewaityou/tap/fleetboard
```

On newer Homebrew, if the first install warns about an untrusted tap:

```bash
brew trust maybewaityou/tap
```

### Option 2: Download a binary from Releases

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/maybewaityou/fleetboard/releases/latest | jq -r .tag_name)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
esac
curl -LJO "https://github.com/maybewaityou/fleetboard/releases/download/${LATEST_TAG}/fleetboard_${OS}_${ARCH}.tar.gz"
tar -xzf fleetboard_${OS}_${ARCH}.tar.gz
sudo mv fleetboard /usr/local/bin/
fleetboard
```

### Option 3: Build from source

```bash
git clone https://github.com/maybewaityou/fleetboard.git
cd fleetboard
make build
sudo mv bin/fleetboard /usr/local/bin/
# Or run it directly without installing
make run
```

### Configuration

Create `~/.fleetboard/config.yaml`:

```yaml
accounts:
  - id: glm-main
    provider: glm
    label: GLM main
    token_env: GLM_API_KEY
  - id: kimi-main
    provider: kimi
    label: Kimi
    token_env: MOONSHOT_API_KEY
refresh:
  on_start: true
  interval: 5m
ui:
  theme: tokyo-night
```

Export the tokens the accounts reference, then run `fleetboard`.

## ⌨️ Key Bindings

| Key | Action | Key | Action |
|-----|--------|-----|--------|
| `↑↓` | Move | `r` | Refresh selected |
| `←/→` | Focus list / details | `R` | Refresh all |
| `/` | Search | `a` | New account |
| `s`/`S` | Cycle sort | `e` | Edit account |
| `p` | Pin / unpin | `d` | Delete account |
| `?` | Help | `q` | Quit |

## 🏗 Architecture

fleetboard follows a hexagonal (ports &amp; adapters) layout, shared with `lazytmux`/`lazyssh`:

```
cmd/main.go                          → cobra root: load config + wire adapters
internal/core/domain/                → Account / ProviderUsage / UsageDimension
internal/core/ports/                 → UsageProvider / ConfigStore / View
internal/core/services/              → Aggregator: concurrent fetch, fault-isolated
internal/adapters/providers/         → one adapter per provider (glm, minimax, kimi, deepseek, ...)
internal/adapters/config/yaml/       → ~/.fleetboard/config.yaml (atomic write + backups)
internal/adapters/ui/                → tview TUI (Tokyo Night)
```

Adding a provider = drop one file in `internal/adapters/providers/<name>/` and register it in `cmd/main.go`.

## 🤝 Contributing

Semantic commit messages: `type(scope): short description`
(`feat`, `fix`, `improve`, `refactor`, `docs`, `test`, `ci`, `chore`).

## ⭐ Support

If fleetboard saves you some time, a star is appreciated.

### ☕ Sponsor

If you'd like to support development:

<a href="https://www.buymeacoffee.com/maybewaityou" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" width="200" /></a>

**WeChat Pay / Alipay**

<table>
  <tr>
    <td align="center">
      <img src="./docs/resources/donate-wechat.jpg" alt="WeChat Pay" width="180" />
      <br/>
      <b>WeChat Pay</b>
    </td>
    <td width="80"></td>
    <td align="center">
      <img src="./docs/resources/donate-alipay.jpg" alt="Alipay" width="180" />
      <br/>
      <b>Alipay</b>
    </td>
  </tr>
</table>

## 🙏 Acknowledgments

- [`lazytmux`](https://github.com/maybewaityou/lazytmux) / `lazyssh` — the TUI layout, theme, and architecture fleetboard is ported from.
- [`cc-switch`](https://github.com/farion1231/cc-switch) — reference for provider usage endpoints.

## License

Apache-2.0. See [LICENSE](LICENSE).
````

- [ ] **Step 4: Create `README.zh-CN.md` (Chinese mirror)**

Create `/home/either/workspace/repos/tools/fleetboard/README.zh-CN.md`:
````markdown
<div align="center">

# fleetboard

**终端里的 AI Coding 套餐额度 / 余额仪表盘。**

一屏聚合 GLM、MiniMax、Kimi、DeepSeek（及更多）的额度与余额——用了多少、何时重置、哪个号还能用。

[English](README.md) · [简体中文](README.zh-CN.md)

</div>

## ✨ 功能

- **一屏看全部厂商** —— 每行一个账号：标签、厂商色块、用量百分比、状态点。
- **额度 + 余额** —— 百分比窗口（GLM 5 小时 / 每周 / 每月、MiniMax）**与**账户余额（Kimi、DeepSeek）。
- **最近窗口优先** —— 列表展示重置时间最近的那一档，最紧迫的额度始终可见。
- **两级刷新** —— `r` 刷新选中账号，`R` 刷新全部账号。
- **手动增删改** —— 新增 / 编辑 / 删除 / 置顶账号；配置存于 `~/.fleetboard/config.yaml`。
- **搜索与排序** —— `/` 过滤，`s`/`S` 循环排序（名称 / 用量 / 最近刷新）。
- **Tokyo Night 主题** TUI，移植自 `lazytmux` / `lazyssh` 工具家族。

## 🔒 工作原理

fleetboard 读取账号配置，调用各厂商官方的用量 / 余额接口，归一化后渲染。token 从（账号指定的）环境变量读取，绝不落盘，也只发往该厂商自己的接口。本地解析 `~/.claude/` 用量文件不在范围内——服务端是唯一数据源。

## 📦 安装

### 方式一：Homebrew（macOS）

```bash
brew install maybewaityou/tap/fleetboard
```

较新版 Homebrew 首次安装若提示 tap 不可信：

```bash
brew trust maybewaityou/tap
```

### 方式二：从 Releases 下载二进制

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/maybewaityou/fleetboard/releases/latest | jq -r .tag_name)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
esac
curl -LJO "https://github.com/maybewaityou/fleetboard/releases/download/${LATEST_TAG}/fleetboard_${OS}_${ARCH}.tar.gz"
tar -xzf fleetboard_${OS}_${ARCH}.tar.gz
sudo mv fleetboard /usr/local/bin/
fleetboard
```

### 方式三：源码编译

```bash
git clone https://github.com/maybewaityou/fleetboard.git
cd fleetboard
make build
sudo mv bin/fleetboard /usr/local/bin/
# 或不安装直接运行
make run
```

### 配置

创建 `~/.fleetboard/config.yaml`：

```yaml
accounts:
  - id: glm-main
    provider: glm
    label: 智谱编码-主力
    token_env: GLM_API_KEY
  - id: kimi-main
    provider: kimi
    label: Kimi
    token_env: MOONSHOT_API_KEY
refresh:
  on_start: true
  interval: 5m
ui:
  theme: tokyo-night
```

导出账号引用的 token 环境变量，然后运行 `fleetboard`。

## ⌨️ 快捷键

| 键 | 动作 | 键 | 动作 |
|----|------|----|------|
| `↑↓` | 移动 | `r` | 刷新选中 |
| `←/→` | 列表 / 详情切换焦点 | `R` | 刷新全部 |
| `/` | 搜索 | `a` | 新增账号 |
| `s`/`S` | 循环排序 | `e` | 编辑账号 |
| `p` | 置顶 / 取消 | `d` | 删除账号 |
| `?` | 帮助 | `q` | 退出 |

## 🏗 架构

fleetboard 采用六边形（端口适配器）架构，与 `lazytmux`/`lazyssh` 一致：

```
cmd/main.go                          → cobra 根命令：加载配置 + 装配依赖
internal/core/domain/                → Account / ProviderUsage / UsageDimension
internal/core/ports/                 → UsageProvider / ConfigStore / View
internal/core/services/              → Aggregator：并发拉取，单点失败不连坐
internal/adapters/providers/         → 每家厂商一个 adapter（glm、minimax、kimi、deepseek …）
internal/adapters/config/yaml/       → ~/.fleetboard/config.yaml（原子写 + 备份）
internal/adapters/ui/                → tview TUI（Tokyo Night）
```

新增厂商 = 在 `internal/adapters/providers/<name>/` 放一个文件，并在 `cmd/main.go` 注册。

## 🤝 贡献

语义化提交：`type(scope): 简短描述`
（`feat`、`fix`、`improve`、`refactor`、`docs`、`test`、`ci`、`chore`）。

## ⭐ 支持

如果 fleetboard 帮到了你，欢迎点个 Star。

### ☕ 赞助

如果愿意支持开发：

<a href="https://www.buymeacoffee.com/maybewaityou" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" width="200" /></a>

**微信 / 支付宝**

<table>
  <tr>
    <td align="center">
      <img src="./docs/resources/donate-wechat.jpg" alt="微信" width="180" />
      <br/>
      <b>微信</b>
    </td>
    <td width="80"></td>
    <td align="center">
      <img src="./docs/resources/donate-alipay.jpg" alt="支付宝" width="180" />
      <br/>
      <b>支付宝</b>
    </td>
  </tr>
</table>

## 🙏 致谢

- [`lazytmux`](https://github.com/maybewaityou/lazytmux) / `lazyssh` —— fleetboard 移植的 TUI 布局、主题与架构。
- [`cc-switch`](https://github.com/farion1231/cc-switch) —— 厂商用量端点的参考。

## 许可证

Apache-2.0，详见 [LICENSE](LICENSE)。
````

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "docs: add bilingual README + LICENSE + donate assets

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Final Verification

- [ ] **Step 1: Full build + race tests + cover**

Run: `make test`
Expected: all packages PASS, no races, a coverage summary printed.

- [ ] **Step 2: Quality gate**

Run: `make quality`
Expected: `go vet` clean (gofumpt best-effort).

- [ ] **Step 3: Smoke the binary**

Run: `go build -o /tmp/fleetboard ./cmd && /tmp/fleetboard --help`
Expected: cobra prints the help text (root `Use: fleetboard`, Short "AI coding plan usage dashboard TUI") and exits 0. (Running without `--help` launches the TUI; skip that in CI.)

- [ ] **Step 4: Confirm no stray `vendor` remains**

Run: `grep -rni "vendor" --include="*.go" internal cmd`
Expected: zero hits.

- [ ] **Step 5: Confirm docs assets exist**

Run: `ls README.md README.zh-CN.md LICENSE docs/resources/donate-*.jpg .goreleaser.yaml .github/workflows/release.yml`
Expected: all listed files present.
