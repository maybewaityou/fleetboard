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
// selection index can be resolved back to a domain.ProviderUsage. It is the
// fleetboard analogue of lazytmux's SessionList, but each row is a single line
// (no mini progress bar) carrying: label, provider tag, primary percent, status
// dot. See formatAccountLine for the exact row grammar.
type AccountList struct {
	*tview.List
	usages            []domain.ProviderUsage
	onSelectionChange func(domain.ProviderUsage)
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
		SetBorderPadding(0, 0, 0, 0) // 左右 padding 归零：选中高亮（SetHighlightFullLine）顶满边框；
		// 行首视觉缩进由 formatAccountLine 的 pin 占位（  /📌，显示宽 2）提供，内容不贴边。
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
func (al *AccountList) UpdateUsages(usages []domain.ProviderUsage) {
	al.usages = usages
	al.List.Clear()
	for i := range usages {
		al.List.AddItem(formatAccountLine(usages[i]), "", 0, nil)
	}
	if al.List.GetItemCount() > 0 {
		al.List.SetCurrentItem(0)
	}
}

// GetSelected resolves the current cursor position to its ProviderUsage.
func (al *AccountList) GetSelected() (domain.ProviderUsage, bool) {
	idx := al.List.GetCurrentItem()
	if idx >= 0 && idx < len(al.usages) {
		return al.usages[idx], true
	}
	return domain.ProviderUsage{}, false
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

func (al *AccountList) OnSelectionChange(fn func(domain.ProviderUsage)) *AccountList {
	al.onSelectionChange = fn
	return al
}

func (al *AccountList) OnReturnToSearch(fn func()) *AccountList {
	al.onReturnToSearch = fn
	return al
}

// SetSortTitle writes the active sort mode into the list border title.
func (al *AccountList) SetSortTitle(mode string) {
	al.List.SetTitle(" Accounts — Sort: " + mode + " ")
}

// displayDimension returns the dimension shown in the list: the one with the
// soonest non-zero ResetsAt (the nearest reset window — "最近时间"), falling back
// to Primary when no dimension carries a reset time (balance providers such as
// kimi/deepseek), then nil. -1 from PercentUsed is the sentinel StatusColor
// reads as "gray", so the list dot and details bar degrade consistently.
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

// formatAccountLine renders one aligned list row. Every column has a fixed
// DISPLAY width so the progress bar, the value, the status dot and the
// "Last Refreshed" text each start on the same column across all rows:
//
//	<pin2> <icon> <label pad16> <provider pad9> <miniBar4> <pct 左对齐7> <dot>    <lastRefreshed>
//
// icon = provider 首字母(品牌色). label/provider/pct 均用 padDisplay (CJK 显示宽度对齐):
// pct 左对齐紧贴进度条 (用量数值与进度条语义连贯), 列宽固定 7 使右边界不变, 状态点仍对齐.
// fetched 是相对时间 (humanizeAgo).
func formatAccountLine(u domain.ProviderUsage) string {
	d := displayDimension(u)
	pctStr, dot := "N/A", "○"
	dotCol := colorGray // N/A 默认灰点
	if d != nil && d.Currency != "" {
		// 余额型：显示余额 + BalanceColor 染色（阈值可配，默认 >=10 绿 / >=1 黄 / <1 红）。
		pctStr = formatMoneyShort(d.Balance, d.Currency)
		dot = "●"
		dotCol = BalanceColor(d.Balance, d.Currency)
	} else if d != nil {
		// 配额型：百分比 + StatusColor
		pctStr = fmt.Sprintf("%d%%", int(d.PercentUsed))
		dot = "●"
		dotCol = StatusColor(d.PercentUsed)
	}
	pct := displayPercent(u) // 余额型 PercentUsed=-1 → renderBar(-1,4) 自然灰条

	// icon: provider 首字母大写, 品牌色（ProviderTag 的 fg）。
	_, iconFg := ProviderTag(u.Provider)
	icon := "?"
	if u.Provider != "" {
		icon = strings.ToUpper(u.Provider[:1])
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

	// 列布局（每列固定显示宽度 → 进度条/数值/状态点/时间跨行严格对齐）：
	//   pin(2) icon(1) sp | label pad16 | sp | providerChip(pad9) | 2sp | miniBar(4) sp | pctStr 左对齐7 | sp dot | 4sp | Last Refreshed: fetched
	// 对齐要点：① provider pad 到 9 覆盖最长 slug "anthropic"，否则 anthropic/deepseek 行整条右半部右移；
	//          ② pctStr 紧贴 miniBar 左对齐（padDisplay 到 7）：数值与进度条语义连贯（都是用量），
	//            同时列宽固定 7 → 右边界不变 → 紧跟的状态点 ● 仍落在同一列；miniBar(4) 本身定宽。
	return fmt.Sprintf("%s [%s]%s[-] %s [black:%s] %s [-:-:-]  %s [%s]%s[-] [%s]%s[-]    [%s]Last Refreshed: %s[-]",
		pin,
		iconFg, icon,
		padDisplay(label, 16),
		colorAccent, padDisplay(u.Provider, 9),
		renderBar(pct, 4),
		colorPrimary, padDisplay(pctStr, 7),
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
