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

// barWidth is the number of cells in a details progress bar. 20 keeps the whole
// "█░ 85% used/limit left resets" line within a 2/5-width pane at 80 cols.
const barWidth = 20

// AccountDetails is the right-hand pane showing the selected account's header
// (label + vendor tag + optional error) followed by one progress-bar line per
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
		SetBorderColor(tcell.GetColor(colorBorder))
	// Placeholder states (initial + empty) read better centered; Render flips
	// back to left alignment for multi-line content (mirrors lazytmux).
	d.SetTextAlign(tview.AlignCenter)
	d.SetText("[" + colorSecondary + "]select an account[-]")
	return d
}

// Render fills the pane. If the account has any dimensions they are all listed
// (one bar each); per the task-7 err-transparency contract, a non-nil Err does
// NOT suppress them — the error is surfaced as a red ⚠ in the header instead.
func (d *AccountDetails) Render(u domain.VendorUsage) {
	d.SetTextAlign(tview.AlignLeft)
	var b strings.Builder

	// Header: label only (accent bold) + 可选 err ⚠（vendor chip 移除，信息下放到 Basic Info）。
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
	window := "—"
	if !u.WindowStart.IsZero() && !u.WindowEnd.IsZero() {
		window = u.WindowStart.Local().Format("2006-01-02 15:04") + " → " + u.WindowEnd.Local().Format("2006-01-02 15:04")
	}
	refreshed := "—"
	if !u.FetchedAt.IsZero() {
		refreshed = u.FetchedAt.Local().Format("2006-01-02 15:04")
	}
	b.WriteString(basicInfoLine("Plan", plan))
	b.WriteString(basicInfoLine("Vendor", u.Vendor))
	b.WriteString(basicInfoLine("BaseURL", firstNonEmpty(u.BaseURL, "—")))
	b.WriteString(basicInfoLine("Endpoint", firstNonEmpty(u.Endpoint, "—")))
	b.WriteString(basicInfoLine("Window", window))
	b.WriteString(basicInfoLine("Refreshed", refreshed))

	// Quota Dimensions：各维度单独行（renderDimension 内 name|bar|pct 固定宽对齐）。
	b.WriteString("\n[" + colorTitle + "::b]Quota Dimensions[-]\n")
	if len(u.Dimensions) == 0 {
		b.WriteString("[" + colorSecondary + "]no quota dimensions[-]\n")
	}
	for _, dim := range u.Dimensions {
		b.WriteString(renderDimension(dim))
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
	pct := dim.PercentUsed
	bar := renderBar(pct, barWidth)

	pctStr := "N/A"
	if pct >= 0 {
		pctStr = fmt.Sprintf("%d%%", int(pct))
	}

	used := compactInt(dim.Used, dim.Unit)
	limit := compactInt(dim.Limit, dim.Unit)
	left := compactInt(dim.Remaining, dim.Unit)

	reset := "—"
	if !dim.ResetsAt.IsZero() {
		reset = dim.ResetsAt.Local().Format("2006-01-02 15:04")
	}

	name := dim.Name
	if name == "" {
		name = "—"
	}

	return fmt.Sprintf("  [%s::b]%-16s[-] %s  [%s]%-4s[-]  [%s]%s/%s[-]  [%s]•[-]  [%s]%s left[-]  [%s]•[-]  [%s]resets %s[-]\n",
		colorPrimary, name,
		bar,
		colorPrimary, pctStr,
		colorPrimary, used, limit,
		colorSecondary,
		colorPrimary, left,
		colorSecondary,
		colorSecondary, reset,
	)
}

// renderBar draws a width-cell bar filled proportionally to pct, colored by
// StatusColor. pct<0 (N/A) yields an all-gray hollow bar. Width is parameterized
// so the list's miniBar (8) and details' bar (20) share one implementation.
func renderBar(pct float64, width int) string {
	if pct < 0 {
		return "[" + colorGray + "]" + strings.Repeat("░", width) + "[-]"
	}
	n := int(pct / 100.0 * float64(width))
	if n > width {
		n = width
	}
	if n < 0 {
		n = 0
	}
	col := StatusColor(pct)
	return "[" + col + "]" + strings.Repeat("█", n) + strings.Repeat("░", width-n) + "[-]"
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
