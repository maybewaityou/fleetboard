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
	"math"
	"strings"
	"time"

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
		SetTitleColor(tcell.GetColor(colorTitle)).
		SetBorderPadding(0, 0, 1, 1) // 左右各 1 空格：条目与选中高亮不再紧贴边框
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
//	<icon> <label pad22>    <vendor chip>    <miniBar8> <pct> <dot>    <lastRefreshed>
//
// icon = vendor 首字母(品牌色), 与左边框留 1 空格(参考 lazytmux marker 固定列)。
// vendor 与 miniBar 之间留宽间距；miniBar+pct+dot 紧凑；dot 与 fetched 之间留宽间距。
// fetched 是相对时间(humanizeAgo)。label 用 padDisplay(CJK 显示宽度对齐)。
func formatAccountLine(u domain.VendorUsage) string {
	pctStr, dot := "N/A", "○"
	dotCol := colorGray // N/A 默认灰点
	if u.Primary != nil && u.Primary.Currency != "" {
		// 余额型：显示余额 + 绿/红点（按余额正负）
		pctStr = formatMoneyShort(u.Primary.Balance, u.Primary.Currency)
		dot = "●"
		if u.Primary.Balance > 0 {
			dotCol = colorGreen
		} else {
			dotCol = colorRed
		}
	} else if u.Primary != nil {
		// 配额型：百分比 + StatusColor
		pctStr = fmt.Sprintf("%d%%", int(u.Primary.PercentUsed))
		dot = "●"
		dotCol = StatusColor(u.Primary.PercentUsed)
	}
	pct := primaryPercent(u) // 余额型 PercentUsed=-1 → renderBar(-1,4) 自然灰条

	// icon: vendor 首字母大写, 品牌色（VendorTag 的 fg）。
	_, iconFg := VendorTag(u.Vendor)
	icon := "?"
	if u.Vendor != "" {
		icon = strings.ToUpper(u.Vendor[:1])
	}

	label := u.Label
	if u.Err != nil {
		label = "[" + colorRed + "]⚠[-] " + u.Label
	}

	// pin marker：置顶显示绿色 📌，否则两空格保持列对齐（emoji 显示宽 2）。
	pin := "  "
	if u.Pinned {
		pin = "[" + colorGreen + "]📌[-]"
	}

	fetched := humanizeAgo(u.FetchedAt)

	// 列布局：pin(2) icon(1) sp | label pad16 | 1sp | vendor chip | 2sp | miniBar4 sp pct(pad4) dot | 4sp | Last Refreshed: fetched
	return fmt.Sprintf("%s [%s]%s[-] %s [black:%s] %-7s [-:-:-]  %s [%s]%-4s[-][%s]%s[-]    [%s]Last Refreshed: %s[-]",
		pin,
		iconFg, icon,
		padDisplay(label, 16),
		colorAccent, u.Vendor,
		renderBar(pct, 4),
		colorPrimary, pctStr,
		dotCol, dot,
		colorSecondary, fetched)
}

// formatMoneyShort 余额短格式（列表用，1 位小数，>1000 缩写 k）。负值把负号置于符号之前
// （-¥50.0 而非 ¥-50.0），spec §3 容许负余额场景。
func formatMoneyShort(balance float64, currency string) string {
	sym := currencySymbol(currency)
	if balance < 0 {
		if math.Abs(balance) >= 1000 {
			return "-" + sym + fmt.Sprintf("%.1fk", -balance/1000)
		}
		return "-" + sym + fmt.Sprintf("%.1f", -balance)
	}
	if math.Abs(balance) >= 1000 {
		return fmt.Sprintf("%s%.1fk", sym, balance/1000)
	}
	return fmt.Sprintf("%s%.1f", sym, balance)
}

// humanizeAgo 把时间渲染为相对时长（"5m ago"/"3h ago"/"2d ago"），零值→"—"。
// 移植自 lazytmux humanizeDuration。
func humanizeAgo(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	default:
		months := int(d.Hours()) / (24 * 30)
		if months < 1 {
			months = 1
		}
		return fmt.Sprintf("%dmo ago", months)
	}
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
