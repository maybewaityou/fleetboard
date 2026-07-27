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

	"github.com/maybewaityou/fleetboard/internal/core/domain"
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

// closeModal 回到主仪表盘（恢复 root + 焦点回列表）。help 与 delete-confirm 共用。
func (t *TUI) closeModal() {
	t.app.SetRoot(t.root, true)
	t.focusList()
}

// openAccountForm 打开账号表单：edit=false 新增（空表单），edit=true 编辑（用
// onLoadAccount 反查当前选中账号并 Prefill）。提交后按 editingID 是否非空决定
// 调 onEditAccount（保留 ID）还是 onSaveAccount（新建）。回调返回新数据集，TUI
// 直接 Render（与 RefreshAll 同构，TUI 不碰 store）。
func (t *TUI) openAccountForm(edit bool) {
	form := NewAccountForm()
	editingID := ""
	if edit {
		if t.selectedID == "" {
			t.setStatusTemporary("[" + colorYellow + "]No account selected[-]")
			return
		}
		if t.onLoadAccount != nil {
			if acc, ok := t.onLoadAccount(t.selectedID); ok {
				form.Prefill(acc)
				editingID = t.selectedID
			}
		}
	}
	id := editingID
	form.OnSubmit(func(acc domain.Account) {
		t.closeForm()
		var usages []domain.ProviderUsage
		switch {
		case id != "" && t.onEditAccount != nil:
			usages = t.onEditAccount(id, acc)
		case t.onSaveAccount != nil:
			usages = t.onSaveAccount(acc)
		}
		if usages != nil {
			// 表单提交回调在 tview 主循环执行——与 doTogglePin 同理，必须同步刷新，
			// 不能走 Render(QueueUpdateDraw)，否则主循环自死锁。
			t.applyDataset(usages)
		}
	}).OnCancel(t.closeForm)
	t.app.SetRoot(form.Primitive(), true)
	t.app.SetFocus(form.Form())
}

// closeForm 关闭账号表单，回到主仪表盘。
func (t *TUI) closeForm() {
	t.app.SetRoot(t.root, true)
	t.focusList()
}

// confirmDelete 弹出确认对话框；确认后调 onDeleteAccount(selectedID) 并 Render 新数据集。
func (t *TUI) confirmDelete() {
	if t.selectedID == "" {
		t.setStatusTemporary("[" + colorYellow + "]No account selected[-]")
		return
	}
	id := t.selectedID
	modal := tview.NewModal().
		SetText("Delete account " + id + "?").
		AddButtons([]string{"Delete", "Cancel"}).
		SetDoneFunc(func(_ int, buttonLabel string) {
			if buttonLabel == "Delete" && t.onDeleteAccount != nil {
				t.closeModal()
				if usages := t.onDeleteAccount(id); usages != nil {
					// modal 回调在 tview 主循环执行——与 doTogglePin 同理，必须同步刷新，
					// 不能走 Render(QueueUpdateDraw)，否则主循环自死锁。
					t.applyDataset(usages)
				}
				return
			}
			t.closeModal()
		})
	t.app.SetRoot(modal, true)
}
