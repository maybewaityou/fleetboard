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

// StatusBar is the centered keybinding hint line at the bottom of the screen.
// It advertises only the highest-frequency keys; low-frequency actions (Pin,
// Sort, Refresh All, focus cycling) live behind ? Help (see keybindings.go, the
// single source of truth the Help panel renders). Per the task-8 brief, the
// footer never shows a "last refreshed" timestamp — that information lives on
// the details pane's "拉取" line instead, so the footer stays a pure key legend.
type StatusBar struct {
	*tview.TextView
}

func NewStatusBar() *StatusBar {
	sb := &StatusBar{TextView: tview.NewTextView()}
	sb.SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetBackgroundColor(tcell.ColorDefault)
	sb.SetText(defaultHints())
	return sb
}

// SetStatus replaces the hint line with an arbitrary (often transient) message,
// e.g. "Refreshing..." or an error string.
func (s *StatusBar) SetStatus(msg string) { s.SetText(msg) }

// ResetHints restores the default keybinding hints after a transient SetStatus.
func (s *StatusBar) ResetHints() { s.SetText(defaultHints()) }

// defaultHints is the footer legend. Keys are cyan, items separated by "•", and
// the whole line is centered by the TextView's text alignment. Only the
// highest-frequency keys are shown; the rest (Pin/Sort/Refresh All/focus) are
// discoverable via ? Help — a "keep the line short by design" strategy ported
// from lazytmux, which never needed width-based truncation as a result.
func defaultHints() string {
	k := colorCyan
	return "[" + k + "]↑↓[-] Navigate  • " +
		"[" + k + "]a[-] New  • " +
		"[" + k + "]e[-] Edit  • " +
		"[" + k + "]d[-] Delete  • " +
		"[" + k + "]r[-] Refresh  • " + // refresh selected account
		"[" + k + "]/[-] Search  • " +
		"[" + k + "]?[-] Help  • " +
		"[" + k + "]q[-] Quit"
}

// emptyHints is the minimal footer for the no-accounts state: only the keys
// that still do something meaningful plus a lead-in label. R (refresh all) is
// omitted — re-fetching an empty config is a no-op, and the boot loading screen
// already covered the initial fetch.
func emptyHints() string {
	k := colorCyan
	return "[" + k + "]No accounts[-]  •  " +
		"[" + k + "]a[-] New  •  " +
		"[" + k + "]?[-] Help  •  " +
		"[" + k + "]q[-] Quit"
}
