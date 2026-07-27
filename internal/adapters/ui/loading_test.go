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
	"strings"
	"testing"
)

// TestLoadingView_InitialFrame 验证 NewLoadingView 首帧即画出 app 名 + 第 0 帧 spinner + 标签，
// 而非空屏（首帧由构造器立即 SetFrame(0,…)）。断言目标子串都不含 tview tag 字符，故直接在原串上匹配。
func TestLoadingView_InitialFrame(t *testing.T) {
	lv := NewLoadingView()
	raw := lv.GetText(false)
	for _, want := range []string{"fleetboard", "Loading accounts…"} {
		if !strings.Contains(raw, want) {
			t.Errorf("loading view missing %q, got: %q", want, raw)
		}
	}
	if !strings.Contains(raw, spinnerFrames[0]) {
		t.Errorf("loading view missing initial spinner frame %q, got: %q", spinnerFrames[0], raw)
	}
}

// TestLoadingView_SetFrameCycles 验证 SetFrame 按取模推进帧，含回绕（idx=10 应等于第 0 帧）。
func TestLoadingView_SetFrameCycles(t *testing.T) {
	lv := NewLoadingView()
	for _, idx := range []int{0, 3, 7, 10, 13} { // 10 与 13 覆盖 %len 回绕
		lv.SetFrame(idx, "Loading accounts…")
		raw := lv.GetText(false)
		want := spinnerFrames[idx%len(spinnerFrames)]
		if !strings.Contains(raw, want) {
			t.Errorf("frame %d: want spinner %q, got: %q", idx, want, raw)
		}
	}
}

// TestLoadingView_LabelSwappable 验证 label 可被替换（为将来 "Loading N/M…" 进度复用留口子）。
func TestLoadingView_LabelSwappable(t *testing.T) {
	lv := NewLoadingView()
	lv.SetFrame(0, "Booting…")
	if !strings.Contains(lv.GetText(false), "Booting…") {
		t.Error("label should be swappable")
	}
}
