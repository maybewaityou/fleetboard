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

// 字段在 form 中的下标（用于按位读取）。Vendor 是 DropDown，其余 InputField。
const (
	afFieldID = iota
	afFieldVendor
	afFieldLabel
	afFieldBaseURL
	afFieldTokenEnv
)

// vendorOptions 是 Vendor 下拉的可选项（与 cmd/main 注册的 adapter 对应）。
var vendorOptions = []string{"glm", "minimax", "kimi", "deepseek"}

// AccountForm 是新增/编辑账号的模态表单（仿 lazytmux session_multifield_form）。
type AccountForm struct {
	form     *tview.Form
	onSubmit func(domain.Account)
	onCancel func()
}

// NewAccountForm 构造表单。SetBorderPadding(0,0,1,1) 是关键坑：tview.Form 默认四周
// padding 1，多字段时会偷两行导致聚焦末字段时首字段滚出视图。
func NewAccountForm() *AccountForm {
	f := &AccountForm{form: tview.NewForm()}
	f.form.SetBorder(true).
		SetTitle(" Account ").
		SetTitleColor(tcell.GetColor(colorTitle)).
		SetBorderColor(tcell.GetColor(colorBorder))
	f.form.AddInputField("ID", "", 0, nil, nil)
	f.form.AddDropDown("Vendor", vendorOptions, 0, nil)
	f.form.AddInputField("Label", "", 0, nil, nil)
	f.form.AddInputField("BaseURL", "", 0, nil, nil)
	f.form.AddInputField("TokenEnv", "", 0, nil, nil)
	f.form.SetBorderPadding(0, 0, 1, 1)
	f.form.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyESC:
			if f.onCancel != nil {
				f.onCancel()
			}
			return nil
		case tcell.KeyEnter:
			// 焦点在 Vendor DropDown 时放行 Enter，让 tview 内部确认下拉选项
			// （否则 Enter 被全局拦截成 submit，下拉无法选定 —— #4 bug 根因）。
			// GetFocusedItemIndex 返回 (formItem, button)，只看 formItem。
			item, _ := f.form.GetFocusedItemIndex()
			if item == afFieldVendor {
				return e
			}
			f.submit()
			return nil
		}
		return e
	})
	return f
}

// OnSubmit / OnCancel 是 builder 式回调 setter。
func (f *AccountForm) OnSubmit(fn func(domain.Account)) *AccountForm { f.onSubmit = fn; return f }
func (f *AccountForm) OnCancel(fn func()) *AccountForm               { f.onCancel = fn; return f }

// Form 返回内部 *tview.Form，供调用方 SetFocus。
func (f *AccountForm) Form() *tview.Form { return f.form }

// Prefill 用现有账号预填（编辑场景）。
func (f *AccountForm) Prefill(acc domain.Account) {
	f.input(afFieldID).SetText(acc.ID)
	for i, v := range vendorOptions {
		if v == acc.Vendor {
			f.vendorDropDown().SetCurrentOption(i)
		}
	}
	f.input(afFieldLabel).SetText(acc.Label)
	f.input(afFieldBaseURL).SetText(acc.BaseURL)
	f.input(afFieldTokenEnv).SetText(acc.TokenEnv)
}

// submit 校验并提交。ID/Vendor/TokenEnv 必填；校验失败保持表单打开（不调 onSubmit）。
func (f *AccountForm) submit() {
	id := f.text(afFieldID)
	_, vendor := f.vendorDropDown().GetCurrentOption()
	if id == "" || vendor == "" || f.text(afFieldTokenEnv) == "" {
		return
	}
	if f.onSubmit != nil {
		f.onSubmit(domain.Account{
			ID:       id,
			Vendor:   vendor,
			Label:    f.text(afFieldLabel),
			BaseURL:  f.text(afFieldBaseURL),
			TokenEnv: f.text(afFieldTokenEnv),
		})
	}
}

func (f *AccountForm) input(idx int) *tview.InputField {
	return f.form.GetFormItem(idx).(*tview.InputField)
}

func (f *AccountForm) vendorDropDown() *tview.DropDown {
	return f.form.GetFormItem(afFieldVendor).(*tview.DropDown)
}

func (f *AccountForm) text(idx int) string {
	return f.input(idx).GetText()
}

// Primitive 返回居中模态布局（三层 Flex：外列留白 + 中行[上下留白+表单] + 右列留白）。
func (f *AccountForm) Primitive() tview.Primitive {
	hint := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[" + colorSecondary + "]Enter 提交 · ESC 取消[-]")
	column := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(f.form, 0, 1, true).
		AddItem(hint, 1, 0, false)
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(column, 14, 0, true).
			AddItem(nil, 0, 1, false), 62, 0, true).
		AddItem(nil, 0, 1, false)
}
