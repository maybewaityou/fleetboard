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

// openHelp 弹出帮助面板（lazytmux 风格：app.SetRoot 换根，非 Pages）。
// ESC / ? / q 关闭回到主仪表盘。
func (t *TUI) openHelp() {
	help := NewHelpModal()
	help.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyESC {
			t.closeModal()
			return nil
		}
		switch e.Rune() {
		case '?', 'q':
			t.closeModal()
			return nil
		}
		return e
	})
	// 三层 Flex 居中：外列留白 + 中行(上下留白 + help) + 右列留白。
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 1, 0, false).
			AddItem(help, 0, 1, true).
			AddItem(nil, 1, 0, false), 64, 0, true).
		AddItem(nil, 0, 1, false)
	t.app.SetRoot(flex, true)
	t.app.SetFocus(help.focus)
}

// closeModal 回到主仪表盘（恢复 root + 焦点回列表）。
func (t *TUI) closeModal() {
	t.app.SetRoot(t.root, true)
	t.focusList()
}
