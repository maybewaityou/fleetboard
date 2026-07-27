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

	// Header: label (accent bold) + vendor chip (same pill style as the list row).
	b.WriteString("[" + colorAccent + "::b]" + u.Label + "[-]  ")
	b.WriteString(fmt.Sprintf("[black:%s] %s [-:-:-]", colorAccent, u.Vendor))
	if u.Err != nil {
		// Surface the failure without hiding the dimensions below it.
		msg := u.Err.Error()
		if msg == "" {
			msg = "fetch failed"
		}
		b.WriteString("  [" + colorRed + "]⚠ " + msg + "[-]")
	}
	b.WriteString("\n\n")

	if len(u.Dimensions) == 0 {
		b.WriteString("[" + colorSecondary + "]no quota dimensions[-]\n")
	} else {
		b.WriteString("[" + colorTitle + "::b]Quota Dimensions[-]\n")
		for _, dim := range u.Dimensions {
			b.WriteString(renderDimension(dim))
		}
	}

	// Footer: "拉取 <time> · <source>". Source comes from the primary dimension
	// when available (the one the list percent reflects), else the first dim.
	src := ""
	if u.Primary != nil {
		src = u.Primary.Source
	} else if len(u.Dimensions) > 0 {
		src = u.Dimensions[0].Source
	}
	if !u.FetchedAt.IsZero() || src != "" {
		b.WriteString("\n")
		parts := make([]string, 0, 2)
		if !u.FetchedAt.IsZero() {
			parts = append(parts, "["+colorSecondary+"]拉取 "+u.FetchedAt.Format("15:04")+"[-]")
		} else {
			parts = append(parts, "["+colorSecondary+"]拉取 —[-]")
		}
		if src != "" {
			parts = append(parts, "["+colorSecondary+"]"+src+"[-]")
		}
		b.WriteString(strings.Join(parts, " ["+colorSecondary+"]·[-] ") + "\n")
	}

	d.SetText(b.String())
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
	bar := renderBar(pct)

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

// renderBar draws a barWidth-cell bar filled proportionally to pct, colored by
// StatusColor. pct<0 (N/A) yields an all-gray hollow bar.
func renderBar(pct float64) string {
	if pct < 0 {
		return "[" + colorGray + "]" + strings.Repeat("░", barWidth) + "[-]"
	}
	n := int(pct / 100.0 * float64(barWidth))
	if n > barWidth {
		n = barWidth
	}
	if n < 0 {
		n = 0
	}
	col := StatusColor(pct)
	return "[" + col + "]" + strings.Repeat("█", n) + strings.Repeat("░", barWidth-n) + "[-]"
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
