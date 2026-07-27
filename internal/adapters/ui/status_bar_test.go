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

// TestDefaultHints_HasHighFreqKeys 验证 footer 保留了高频键。动作词（New/Quit 等）
// 不含 tview 颜色 tag 字符，故可直接在带 tag 的原串上做子串断言。
func TestDefaultHints_HasHighFreqKeys(t *testing.T) {
	h := defaultHints()
	for _, want := range []string{"Navigate", "New", "Edit", "Delete", "Refresh", "Search", "Help", "Quit"} {
		if !strings.Contains(h, want) {
			t.Errorf("defaultHints missing high-freq key %q", want)
		}
	}
}

// TestDefaultHints_OmitsLowFreqKeys 验证低频键已从 footer 移除（仍可在 ? Help 查到，
// 因 keybindings.go 未改）。注意 "Refresh All" 是完整短语——"Refresh"（选中刷新）仍在。
func TestDefaultHints_OmitsLowFreqKeys(t *testing.T) {
	h := defaultHints()
	for _, unwanted := range []string{"Focus", "Pin", "Sort", "Refresh All"} {
		if strings.Contains(h, unwanted) {
			t.Errorf("defaultHints should omit %q (moved to ? Help), got: %s", unwanted, h)
		}
	}
}

// TestDefaultHints_KeyCount 用分隔符数量验证键数：8 键 → 7 个 "•"。
func TestDefaultHints_KeyCount(t *testing.T) {
	h := defaultHints()
	if n := strings.Count(h, "•"); n != 7 {
		t.Errorf("defaultHints want 7 separators (8 keys), got %d: %s", n, h)
	}
}

// TestEmptyHints_Minimal 验证空态 footer 只剩 4 个有意义键，且不含 Refresh（空态重新拉取
// 空集无意义，初始拉取已由加载界面承担）。
func TestEmptyHints_Minimal(t *testing.T) {
	h := emptyHints()
	for _, want := range []string{"No accounts", "New", "Help", "Quit"} {
		if !strings.Contains(h, want) {
			t.Errorf("emptyHints missing %q", want)
		}
	}
	for _, unwanted := range []string{"Refresh", "Pin", "Sort", "Focus"} {
		if strings.Contains(h, unwanted) {
			t.Errorf("emptyHints should omit %q, got: %s", unwanted, h)
		}
	}
	if n := strings.Count(h, "•"); n != 3 {
		t.Errorf("emptyHints want 3 separators (4 keys), got %d: %s", n, h)
	}
}
