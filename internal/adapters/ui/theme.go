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

// providerColor maps a provider slug (domain.Account.Provider) to its brand colors as
// {bg, fg}, used by the platform tag rendered in the list view (spec §9.2).
// Providers absent from this map fall back to unknownProviderBG/FG via ProviderTag.
var providerColor = map[string][2]string{ // {bg, fg}
	"glm":       {"#7C3AED", "#FFFFFF"},
	"minimax":   {"#EF4444", "#FFFFFF"},
	"kimi":      {"#06B6D4", "#001016"},
	"anthropic": {"#D97757", "#FFFFFF"},
	"openai":    {"#10A37F", "#FFFFFF"},
	"cursor":    {"#6366F1", "#FFFFFF"},
	"copilot":   {"#0969DA", "#FFFFFF"},
	"deepseek":  {"#2563EB", "#FFFFFF"},
	"sub2api":   {"#8B5CF6", "#FFFFFF"}, // 紫
	"newapi":    {"#10B981", "#FFFFFF"}, // 翠绿
}

// Unknown-provider fallback colors: a neutral gray that keeps the tag readable
// even when an account references a provider fleetboard has never heard of.
const (
	unknownProviderBG = "#6B7280"
	unknownProviderFG = "#FFFFFF"
)

// ProviderTag returns the {bg, fg} brand colors for a provider slug. The caller
// feeds the pair straight into tview's [fg:bg] color syntax. Unknown providers
// resolve to a gray tag so the list never renders a broken/empty color block.
//
// Provider matching is exact and case-sensitive: "GLM" is not "glm". Account
// providers are authored by the config layer, which normalizes to the lowercase
// slugs in providerColor, so callers should not need to lowercase themselves.
func ProviderTag(provider string) (bg, fg string) {
	if c, ok := providerColor[provider]; ok {
		return c[0], c[1]
	}
	return unknownProviderBG, unknownProviderFG
}

// StatusColor 把用量百分比映射到状态色。读活动配色（默认 <70 绿 / [70,90] 黄 /
// >90 红，可经 config.yaml ui.colors.quota 覆盖）。pct<0 固定灰（N/A 或拉取失败），
// 先于阈值判断——负值也 <70，故负分支必须先命中以区分"无数据"。
func StatusColor(percent float64) string {
	if percent < 0 {
		return colorGray
	}
	return pickByQuota(activeColors.Load().quota, percent)
}

// BalanceColor 把余额数值映射到状态色（余额越低越危险）。读活动配色（默认 >=10 绿 /
// >=1 黄 / <1 红，可经 config.yaml ui.colors.balance 覆盖；支持负余额）。
// currency 暂未参与选色，保留参数供未来按币别分档。
func BalanceColor(balance float64, currency string) string {
	_ = currency
	return pickByBalance(activeColors.Load().balance, balance)
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
