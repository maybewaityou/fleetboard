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

package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// AccountList wraps tview.List and keeps the slice of usages behind it so a
// selection index can be resolved back to a domain.VendorUsage. It is the
// fleetboard analogue of lazytmux's SessionList, but each row is a single line
// (no mini progress bar) carrying: label, vendor tag, primary percent, status
// dot. See formatAccountLine for the exact row grammar.
type AccountList struct {
	*tview.List
	usages            []domain.VendorUsage
	onSelectionChange func(domain.VendorUsage)
	onReturnToSearch  func()
}

func NewAccountList() *AccountList {
	al := &AccountList{List: tview.NewList()}
	al.build()
	return al
}

func (al *AccountList) build() {
	al.List.ShowSecondaryText(false)
	al.List.SetBorder(true).
		SetTitle(" Accounts ").
		SetTitleAlign(tview.AlignCenter).
		SetBorderColor(tcell.GetColor(colorBorder)).
		SetTitleColor(tcell.GetColor(colorTitle))
	al.List.
		SetSelectedBackgroundColor(tcell.GetColor(colorSelected)).
		SetSelectedTextColor(tcell.GetColor(colorPrimary)).
		SetHighlightFullLine(true)

	al.List.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if index >= 0 && index < len(al.usages) && al.onSelectionChange != nil {
			al.onSelectionChange(al.usages[index])
		}
	})

	al.List.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		// Left/Backspace/ESC hand focus back to the search bar, mirroring
		// lazytmux. Right is reserved for List → Details focus and is handled
		// by the global key capture, so it is intentionally not swallowed.
		case tcell.KeyLeft, tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyESC:
			if al.onReturnToSearch != nil {
				al.onReturnToSearch()
			}
			return nil
		}
		return e
	})
}

// UpdateUsages re-renders the list. The cursor is reset to the first item, so a
// refresh-driven caller should follow up with SelectByAccountID to preserve the
// user's selection (same flow as lazytmux's UpdateSessions + SelectByName).
func (al *AccountList) UpdateUsages(usages []domain.VendorUsage) {
	al.usages = usages
	al.List.Clear()
	for i := range usages {
		al.List.AddItem(formatAccountLine(usages[i]), "", 0, nil)
	}
	if al.List.GetItemCount() > 0 {
		al.List.SetCurrentItem(0)
	}
}

// GetSelected resolves the current cursor position to its VendorUsage.
func (al *AccountList) GetSelected() (domain.VendorUsage, bool) {
	idx := al.List.GetCurrentItem()
	if idx >= 0 && idx < len(al.usages) {
		return al.usages[idx], true
	}
	return domain.VendorUsage{}, false
}

// SelectByAccountID moves the cursor to the first usage with the given account
// ID. Used after UpdateUsages to keep the user's selection stable across
// refreshes; if the id is gone the cursor is left where UpdateUsages put it.
func (al *AccountList) SelectByAccountID(id string) {
	for i, u := range al.usages {
		if u.AccountID == id {
			al.List.SetCurrentItem(i)
			return
		}
	}
}

func (al *AccountList) OnSelectionChange(fn func(domain.VendorUsage)) *AccountList {
	al.onSelectionChange = fn
	return al
}

func (al *AccountList) OnReturnToSearch(fn func()) *AccountList {
	al.onReturnToSearch = fn
	return al
}

// primaryPercent returns Primary.PercentUsed, or -1 when there is no primary
// dimension (N/A). -1 is the sentinel StatusColor reads as "gray", so the list
// dot and details bar both degrade consistently for accounts with no usable
// data.
func primaryPercent(u domain.VendorUsage) float64 {
	if u.Primary == nil {
		return -1
	}
	return u.Primary.PercentUsed
}

// formatAccountLine renders one aligned list row:
//
//	<⚠?> <label pad22>  [black:accent] <vendor pad7> [-:-:-]  <pct pad5>  <dot>  <HH:MM>
//
// Columns are fixed-width so vendor / percent / dot / fetched-time align across
// rows regardless of label length. label uses padDisplay (TaggedStringWidth) so
// CJK labels align by display width, not byte/rune count. The vendor chip uses
// lazytmux's tagChip style (black on unified accent). err rows prefix a red ⚠.
func formatAccountLine(u domain.VendorUsage) string {
	pctStr, dot := "N/A", "○"
	if u.Primary != nil {
		pctStr = fmt.Sprintf("%d%%", int(u.Primary.PercentUsed))
		dot = "●"
	}
	dotCol := StatusColor(primaryPercent(u))

	label := u.Label
	if u.Err != nil {
		label = "[" + colorRed + "]⚠[-] " + u.Label
	}

	fetched := "—"
	if !u.FetchedAt.IsZero() {
		fetched = u.FetchedAt.Format("15:04")
	}

	// 列宽固定：label(pad22, CJK 按显示宽度) | vendor chip(%-7s 在 tag 内) | pct(%-5s) | dot | fetched
	return fmt.Sprintf("%s  [black:%s] %-7s [-:-:-]  %-5s  [%s]%s[-]  [%s]%s[-]",
		padDisplay(label, 22), colorAccent, u.Vendor, pctStr, dotCol, dot, colorSecondary, fetched)
}

// padDisplay right-pads s to a fixed DISPLAY width (CJK chars count as 2),
// preserving tview color tags (tags don't count toward width). Keeps list
// columns aligned across rows of varying label length.
func padDisplay(s string, width int) string {
	if w := tview.TaggedStringWidth(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}
