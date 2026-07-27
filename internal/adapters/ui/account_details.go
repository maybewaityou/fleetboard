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
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// barWidth is the number of cells in a details progress bar. The bar sits on its
// own line under the dimension name, so 20 fits a 2/5-width pane at 80 cols.
const barWidth = 20

// AccountDetails is the right-hand pane showing the selected account's header
// (label + provider tag + optional error) followed by one progress-bar line per
// quota dimension (GLM exposes several tiers), and a trailing "拉取" footer
// carrying the fetch timestamp and data source. It is the fleetboard analogue
// of lazytmux's SessionDetails.
type AccountDetails struct {
	*tview.TextView
}

func NewAccountDetails() *AccountDetails {
	d := &AccountDetails{TextView: tview.NewTextView().SetDynamicColors(true).SetWrap(true)}
	d.SetBorder(true).
		SetTitle(" Details ").
		SetTitleColor(tcell.GetColor(colorTitle)).
		SetBorderColor(tcell.GetColor(colorBorder)).
		SetBorderPadding(0, 0, 1, 1) // 左右各 1 空格，内容不再贴边框（与列表一致）
	// Placeholder states (initial + empty) read better centered; Render flips
	// back to left alignment for multi-line content (mirrors lazytmux).
	d.SetTextAlign(tview.AlignCenter)
	d.SetText("[" + colorSecondary + "]select an account[-]")
	return d
}

// Render fills the pane. If the account has any dimensions they are all listed
// (one bar each); per the task-7 err-transparency contract, a non-nil Err does
// NOT suppress them — the error is surfaced as a red ⚠ in the header instead.
func (d *AccountDetails) Render(u domain.ProviderUsage) {
	d.SetTextAlign(tview.AlignLeft)
	var b strings.Builder

	// Header: label only (accent bold) + 可选 err ⚠（provider chip 移除，信息下放到 Basic Info）。
	b.WriteString("[" + colorAccent + "::b]" + u.Label + "[-]")
	if u.Err != nil {
		msg := u.Err.Error()
		if msg == "" {
			msg = "fetch failed"
		}
		b.WriteString("  [" + colorRed + "]⚠ " + msg + "[-]")
	}
	b.WriteString("\n\n")

	// Basic Info：账号基本信息（adapter 填充）。Plan 优先 PlanLevel(GLM)，否则 Model(MiniMax)。
	b.WriteString("[" + colorTitle + "::b]Basic Info[-]\n")
	plan := firstNonEmpty(u.PlanLevel, u.Model, "—")
	refreshed := "—"
	if !u.FetchedAt.IsZero() {
		refreshed = u.FetchedAt.Local().Format("2006-01-02 15:04")
	}
	// 字段顺序参考 lazytmux：主体标识在前，时间次之，Pinned（布尔状态）置末。
	b.WriteString(basicInfoLine("Plan", plan))
	b.WriteString(providerInfoLine(u.Provider))
	b.WriteString(basicInfoLine("BaseURL", firstNonEmpty(u.BaseURL, "—")))
	b.WriteString(basicInfoLine("Endpoint", firstNonEmpty(u.Endpoint, "—")))
	b.WriteString(basicInfoLine("Refreshed", refreshed))
	b.WriteString(basicInfoLine("Pinned", pinnedStr(u.Pinned)))

	// Quota Dimensions：短期额度优先——按 ResetsAt 升序稳定排序（零值置后），
	// 让 5h 滚动窗口排在 weekly/monthly 之前。各维度由 renderDimension 渲染为独立多行块。
	b.WriteString("\n[" + colorTitle + "::b]Quota Dimensions[-]\n")
	dims := make([]domain.UsageDimension, len(u.Dimensions))
	copy(dims, u.Dimensions)
	sort.SliceStable(dims, func(i, j int) bool {
		ti, tj := dims[i].ResetsAt, dims[j].ResetsAt
		if ti.IsZero() {
			return false // 无重置信息的维度排最后
		}
		if tj.IsZero() {
			return true
		}
		return ti.Before(tj)
	})
	if len(dims) == 0 {
		b.WriteString("[" + colorSecondary + "]no quota dimensions[-]\n")
	}
	for _, dim := range dims {
		b.WriteString(renderDimension(dim))
	}

	// Recent 消耗摘要（仅余额型 provider 填充；nil 时跳过，其他 provider 无感）。
	if u.Recent != nil {
		b.WriteString(renderRecent(*u.Recent))
	}

	d.SetText(b.String())
}

// firstNonEmpty 返回第一个非空字符串，都空则返回最后一个（作 fallback）。
func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// basicInfoLine 渲染 "  key(pad10): value\n"（键 secondary 色，冒号 pad 对齐；值 primary 色）。
func basicInfoLine(key, val string) string {
	return fmt.Sprintf("  [%s]%-10s[-]  [%s]%s[-]\n", colorSecondary, key+":", colorPrimary, val)
}

// providerInfoLine 渲染 Basic Info 中的 Provider 行：key 用与 basicInfoLine 一致的
// %-10s 对齐，value 则用与列表条目完全一致的 chip（accent 背景、黑字）而非纯文本。
func providerInfoLine(provider string) string {
	v := provider
	if v == "" {
		v = "—"
	}
	return fmt.Sprintf("  [%s]%-10s[-]  [black:%s] %s [-:-:-]\n", colorSecondary, "Provider:", colorAccent, v)
}

// RenderEmpty swaps the pane for a centered placeholder when nothing is
// selected (e.g. the account list is empty).
func (d *AccountDetails) RenderEmpty(msg string) {
	d.SetTextAlign(tview.AlignCenter)
	d.SetText("[" + colorSecondary + "]" + msg + "[-]")
}

// renderDimension emits one progress-bar line:
//
//	<name>  <20-cell bar colored by StatusColor>  <pct>%  <used>/<limit>  •  <left> left  •  resets <when>
//
// A N/A dimension (PercentUsed < 0) renders an all-gray hollow bar and "N/A"
// instead of a number, so a partially-failed account still reads cleanly.
func renderDimension(dim domain.UsageDimension) string {
	var b strings.Builder

	name := dim.Name
	if name == "" {
		name = "—"
	}

	// 维度名：独立一行，加粗主色。
	fmt.Fprintf(&b, "  [%s::b]%s[-]\n", colorPrimary, name)

	// 余额型：只显示 Balance 行，不画进度条（余额无进度语义）。
	if dim.Currency != "" {
		fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
			colorSecondary, "Balance:", colorPrimary, formatMoney(dim.Balance, dim.Currency))
		b.WriteString("\n")
		return b.String()
	}

	// 配额型（现有进度条逻辑不变）。
	pct := dim.PercentUsed
	bar := renderBar(pct, barWidth)
	pctStr := "N/A"
	if pct >= 0 {
		pctStr = fmt.Sprintf("%d%%", int(pct))
	}
	fmt.Fprintf(&b, "    %s  [%s]%s[-]\n", bar, colorPrimary, pctStr)

	// 已用/上限、剩余：仅有绝对额度（Limit>0）时显示，避免纯百分比维度出现无意义的 0/0。
	if dim.Limit > 0 {
		used := compactInt(dim.Used, dim.Unit)
		limit := compactInt(dim.Limit, dim.Unit)
		left := compactInt(dim.Remaining, dim.Unit)
		fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s / %s[-]\n", colorSecondary, "Used:", colorPrimary, used, limit)
		fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n", colorSecondary, "Remaining:", colorPrimary, left)
	}

	// 重置时间：独立一行，零值跳过。
	if !dim.ResetsAt.IsZero() {
		fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n", colorSecondary, "Resets:", colorPrimary, dim.ResetsAt.Local().Format("2006-01-02 15:04"))
	}

	// 维度块之间留一空行，便于扫读。
	b.WriteString("\n")
	return b.String()
}

// renderBar draws a width-cell bar filled proportionally to pct, colored by
// StatusColor. It uses the three shade glyphs ░▒▓ (U+2591..U+2593) for a soft
// dotted gradient instead of hard block edges: each filled cell is ▓, the
// single boundary cell fades by sub-cell progress (shadeOf → ▓/▒/░), and the
// remaining tail is ░. pct<0 (N/A) yields an all-gray ░ bar. Width is
// parameterized so the list's miniBar (4) and details' bar (20) share one
// implementation.
//
// Trade-off vs the previous 1/8 block glyphs (▏▎▍▌▋▊▉): shades give a uniform
// dotted texture and soften the fill edge, but carry only 3 levels of sub-cell
// resolution instead of 8. On the 4-cell list bar ~23% now reads as one ▓
// rather than a near-full ▉; the 20-cell details bar loses nothing practical
// (5% per cell, 3 levels is plenty). 50% has rem==0 so no boundary cell is
// emitted — the bar splits exactly in half (▓×N ░×N), centered by construction.
func renderBar(pct float64, width int) string {
	if pct < 0 {
		return "[" + colorGray + "]" + strings.Repeat("░", width) + "[-]"
	}
	if pct > 100 {
		pct = 100
	}
	subs := int(pct / 100.0 * float64(width) * 8)
	if subs > width*8 {
		subs = width * 8
	}
	if subs < 0 {
		subs = 0
	}
	full := subs / 8
	rem := subs % 8
	filled := full
	col := StatusColor(pct)
	var b strings.Builder
	b.WriteString("[" + col + "]")
	b.WriteString(strings.Repeat("▓", full))
	if rem > 0 {
		b.WriteString(shadeOf(rem))
		filled++
	}
	b.WriteString(strings.Repeat("░", width-filled))
	b.WriteString("[-]")
	return b.String()
}

// shadeOf maps a sub-cell remainder (1..7 eighths) to a shade glyph so the
// boundary cell fades rather than showing a hard █|░ edge:
//
//	1–2 eighths → ░  (almost empty, blends into the hollow tail)
//	3–5 eighths → ▒  (half)
//	6–7 eighths → ▓  (almost full, blends into the filled run)
func shadeOf(rem int) string {
	switch {
	case rem <= 2:
		return "░"
	case rem <= 5:
		return "▒"
	default:
		return "▓"
	}
}

// pinnedStr renders the Pinned bool as true/false for the Basic Info table,
// mirroring lazytmux's pinnedStr (which itself matches lazyssh).
func pinnedStr(p bool) string {
	if p {
		return "true"
	}
	return "false"
}

// compactInt renders an int64 with k/M suffixes for readability and appends the
// unit (e.g. "tok") when present. Used for used/limit/remaining columns.
func compactInt(n int64, unit string) string {
	var s string
	switch {
	case n >= 1_000_000:
		s = fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		s = fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		s = fmt.Sprintf("%d", n)
	}
	if unit != "" {
		s += unit
	}
	return s
}

// currencySymbol 返回货币符号：CNY→¥、USD→$、未知→空。列表的 formatMoneyShort
// 与详情页（Task 5）共用此映射，避免两处符号表漂移。
func currencySymbol(currency string) string {
	switch currency {
	case "CNY":
		return "¥"
	case "USD":
		return "$"
	default:
		return ""
	}
}

// formatMoney 余额详情格式（2 位小数）。负值把负号置于符号之前（-¥50.00 而非 ¥-50.00），
// 未知币别按 spec §5.2 降级为 "100.00 EUR"（symbol 为空时拼上币别代码）。
func formatMoney(balance float64, currency string) string {
	sym := currencySymbol(currency)
	if sym == "" {
		return fmt.Sprintf("%.2f %s", balance, currency)
	}
	if balance < 0 {
		return "-" + sym + fmt.Sprintf("%.2f", -balance)
	}
	return fmt.Sprintf("%s%.2f", sym, balance)
}

// renderRecent 渲染近窗口消耗摘要区块：
//
//	Usage (recent)
//	  7-day:        $51.20
//	  30-day:       $138.56
//	  Live:         3 rpm / 1200 tpm
//
// 复用 basicInfoLine（pad10 对齐）与 formatMoney（带货币符号），风格与 Basic Info 一致。
func renderRecent(r domain.RecentUsage) string {
	var b strings.Builder
	b.WriteString("\n[" + colorTitle + "::b]Usage (recent)[-]\n")
	b.WriteString(basicInfoLine("7-day", formatMoney(r.Window7d, r.Currency)))
	b.WriteString(basicInfoLine("30-day", formatMoney(r.Window30d, r.Currency)))
	b.WriteString(basicInfoLine("Live", fmt.Sprintf("%d rpm / %d tpm", r.RPM, r.TPM)))
	return b.String()
}
