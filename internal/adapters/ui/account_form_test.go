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
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// TestAccountFormSubmitValid 验证必填字段齐全时 submit 触发 onSubmit 并带正确字段。
func TestAccountFormSubmitValid(t *testing.T) {
	f := NewAccountForm()
	f.input(afFieldID).SetText("glm-1")
	f.vendorDropDown().SetCurrentOption(0) // glm
	f.input(afFieldLabel).SetText("智谱")
	f.input(afFieldTokenEnv).SetText("GLM_API_KEY")

	var got domain.Account
	called := false
	f.OnSubmit(func(acc domain.Account) { got = acc; called = true })
	f.submit()

	if !called {
		t.Fatal("submit did not fire OnSubmit for valid input")
	}
	if got.ID != "glm-1" || got.Vendor != "glm" || got.TokenEnv != "GLM_API_KEY" || got.Label != "智谱" {
		t.Fatalf("submit fields wrong: %+v", got)
	}
}

// TestAccountFormSubmitRejectsMissingRequired 验证缺必填字段时 submit 不触发。
func TestAccountFormSubmitRejectsMissingRequired(t *testing.T) {
	f := NewAccountForm()
	f.input(afFieldID).SetText("glm-1")
	// Vendor 与 TokenEnv 留空
	called := false
	f.OnSubmit(func(domain.Account) { called = true })
	f.submit()
	if called {
		t.Fatal("submit must not fire when required fields (vendor/token_env) are empty")
	}
}

// TestAccountFormPrefill 验证编辑场景下 Prefill 正确回填所有字段（含 Vendor 下拉）。
func TestAccountFormPrefill(t *testing.T) {
	f := NewAccountForm()
	f.Prefill(domain.Account{ID: "x", Vendor: "minimax", Label: "l", BaseURL: "http://b", TokenEnv: "T"})

	if f.text(afFieldID) != "x" {
		t.Errorf("ID not prefilled: %q", f.text(afFieldID))
	}
	if idx, v := f.vendorDropDown().GetCurrentOption(); v != "minimax" || idx != 1 {
		t.Errorf("vendor not prefilled to minimax(idx1): idx=%d v=%q", idx, v)
	}
	if f.text(afFieldTokenEnv) != "T" {
		t.Errorf("TokenEnv not prefilled: %q", f.text(afFieldTokenEnv))
	}
	if f.text(afFieldBaseURL) != "http://b" {
		t.Errorf("BaseURL not prefilled: %q", f.text(afFieldBaseURL))
	}
}
