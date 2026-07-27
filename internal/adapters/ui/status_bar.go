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
// It surfaces the two refresh actions distinctly: r refreshes the selected
// account only, R refreshes every account. Per the task-8 brief, the footer
// never shows a "last refreshed" timestamp — that information lives on the
// details pane's "拉取" line instead, so the footer stays a pure key legend.
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
// the whole line is centered by the TextView's text alignment. The two refresh
// keys are intentionally adjacent so users can compare them at a glance.
func defaultHints() string {
	k := colorCyan
	return "[" + k + "]↑↓[-] Navigate  • " +
		"[" + k + "]a[-] New  • " +
		"[" + k + "]e[-] Edit  • " +
		"[" + k + "]d[-] Delete  • " +
		"[" + k + "]r[-] Refresh  • " + // refresh selected account
		"[" + k + "]R[-] Refresh All  • " + // refresh every account
		"[" + k + "]/[-] Search  • " +
		"[" + k + "]s[-] Sort  • " +
		"[" + k + "]?[-] Help  • " +
		"[" + k + "]q[-] Quit"
}

// emptyHints is the minimal footer for the no-accounts state: only the keys
// that still do something meaningful plus a lead-in label.
func emptyHints() string {
	k := colorCyan
	return "[" + k + "]No accounts[-]  •  " +
		"[" + k + "]a[-] New  •  " +
		"[" + k + "]R[-] Refresh All  •  " +
		"[" + k + "]?[-] Help  •  " +
		"[" + k + "]q[-] Quit"
}
