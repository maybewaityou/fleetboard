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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// vendorColor maps a vendor slug (domain.Account.Vendor) to its brand colors as
// {bg, fg}, used by the platform tag rendered in the list view (spec §9.2).
// Vendors absent from this map fall back to unknownVendorBG/FG via VendorTag.
var vendorColor = map[string][2]string{ // {bg, fg}
	"glm":       {"#7C3AED", "#FFFFFF"},
	"minimax":   {"#EF4444", "#FFFFFF"},
	"kimi":      {"#06B6D4", "#001016"},
	"anthropic": {"#D97757", "#FFFFFF"},
	"openai":    {"#10A37F", "#FFFFFF"},
	"cursor":    {"#6366F1", "#FFFFFF"},
	"copilot":   {"#0969DA", "#FFFFFF"},
}

// Unknown-vendor fallback colors: a neutral gray that keeps the tag readable
// even when an account references a vendor fleetboard has never heard of.
const (
	unknownVendorBG = "#6B7280"
	unknownVendorFG = "#FFFFFF"
)

// VendorTag returns the {bg, fg} brand colors for a vendor slug. The caller
// feeds the pair straight into tview's [fg:bg] color syntax. Unknown vendors
// resolve to a gray tag so the list never renders a broken/empty color block.
//
// Vendor matching is exact and case-sensitive: "GLM" is not "glm". Account
// vendors are authored by the config layer, which normalizes to the lowercase
// slugs in vendorColor, so callers should not need to lowercase themselves.
func VendorTag(vendor string) (bg, fg string) {
	if c, ok := vendorColor[vendor]; ok {
		return c[0], c[1]
	}
	return unknownVendorBG, unknownVendorFG
}

// StatusColor maps a usage percentage to the status indicator color (spec §9.2):
//
//	percent < 0         → gray   (N/A or fetch failed)
//	percent < 70        → green
//	70 <= percent <= 90 → yellow
//	percent > 90        → red
//
// Boundaries follow spec §9.2/§9.3 verbatim (the task-6 brief's ">=90 red"
// wording was a typo — 90 is the last yellow value, 91 is the first red).
// Check <0 first: a negative percent is also <70, so the negative branch must
// win to surface "no data" distinctly.
func StatusColor(percent float64) string {
	switch {
	case percent < 0:
		return colorGray
	case percent < 70:
		return colorGreen
	case percent <= 90: // [70,90] yellow (含 90)
		return colorYellow
	default: // >90 red
		return colorRed
	}
}

// initializeTheme applies the Tokyo Night palette to tview's global Styles and
// returns a fresh *tview.Application ready to drive the TUI. Ported verbatim
// from lazytmux/internal/adapters/ui/theme.go; the color values live in
// const.go so a future palette swap only touches one file.
//
// Setting tview.Styles is process-global by design — tview reads it from
// package-level state when rendering primitives, so "global" is the only way to
// theme. Call this once at startup before constructing any primitive.
func initializeTheme() *tview.Application {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.BorderColor = tcell.GetColor(colorBorder)
	tview.Styles.TitleColor = tcell.GetColor(colorTitle)
	tview.Styles.PrimaryTextColor = tcell.GetColor(colorPrimary)
	tview.Styles.SecondaryTextColor = tcell.GetColor(colorSecondary)
	tview.Styles.TertiaryTextColor = tcell.GetColor(colorSecondary)
	tview.Styles.GraphicsColor = tcell.GetColor(colorBorder)
	return tview.NewApplication()
}
