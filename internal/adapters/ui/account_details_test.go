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

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// TestBasicInfoLine 验证键值行含 key 与 value。
func TestBasicInfoLine(t *testing.T) {
	got := basicInfoLine("Plan", "pro")
	if !strings.Contains(got, "Plan:") {
		t.Errorf("basicInfoLine missing key 'Plan:': %q", got)
	}
	if !strings.Contains(got, "pro") {
		t.Errorf("basicInfoLine missing val 'pro': %q", got)
	}
}

// TestPinnedStr 验证 Pinned 渲染为 true/false（对齐 lazytmux 的 pinnedStr）。
func TestPinnedStr(t *testing.T) {
	if got := pinnedStr(true); got != "true" {
		t.Errorf("pinnedStr(true) = %q, want true", got)
	}
	if got := pinnedStr(false); got != "false" {
		t.Errorf("pinnedStr(false) = %q, want false", got)
	}
}

// TestFirstNonEmpty 验证取首个非空，都空返回最后参数作 fallback。
func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("firstNonEmpty(a,b) = %q, want a", got)
	}
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Errorf("firstNonEmpty('',b,c) = %q, want b", got)
	}
	if got := firstNonEmpty("", "", "—"); got != "—" {
		t.Errorf("firstNonEmpty('','','—') fallback = %q, want —", got)
	}
}

// TestRenderBar_SubCell 验证亚格子精度：4 格宽度下 23% 不再被 int() 取整为 0，
// 而是渲染出单个部分块 ▉（eighths[7]）+ 空格；颜色随进度（green<70）。
func TestRenderBar_SubCell(t *testing.T) {
	got := renderBar(23, 4) // 23% × 4格 × 8 = 7.36 → subs=7 → ▉ + 3 hollow
	if !strings.Contains(got, "▉") {
		t.Errorf("23%% at width 4 should render a ▉ sub-cell, got: %q", got)
	}
	if !strings.Contains(got, "["+colorGreen+"]") {
		t.Errorf("23%% bar should be green: %q", got)
	}
	if strings.Contains(got, "█") {
		t.Errorf("23%% at width 4 should have no full cell, got: %q", got)
	}
}

// TestRenderBar_FullCells 验证整格边界与颜色梯度：50%=2 整格(green)，95%=红色。
func TestRenderBar_FullCells(t *testing.T) {
	if got := renderBar(50, 4); !strings.Contains(got, "["+colorGreen+"]██░░") {
		t.Errorf("50%% at width 4 = 2 full + 2 hollow green, got: %q", got)
	}
	if got := renderBar(95, 4); !strings.Contains(got, "["+colorRed+"]") {
		t.Errorf("95%% bar should be red: %q", got)
	}
}

// TestFormatMoney 验证余额详情格式（2 位小数）；未知币别按 spec §5.2 拼币别代码；
// 负值负号置于符号之前（spec §3）。
func TestFormatMoney(t *testing.T) {
	if got := formatMoney(49.58894, "CNY"); got != "¥49.59" {
		t.Errorf("formatMoney(49.58894,CNY) = %q, want ¥49.59", got)
	}
	if got := formatMoney(3.0, "USD"); got != "$3.00" {
		t.Errorf("formatMoney(3.0,USD) = %q, want $3.00", got)
	}
	// M1: 未知币别 — symbol 为空时降级为 "100.00 EUR"（spec §5.2），而非裸数字。
	if got := formatMoney(100.0, "EUR"); got != "100.00 EUR" {
		t.Errorf("formatMoney(100.0,EUR) = %q, want \"100.00 EUR\"", got)
	}
	// M2: 负值负号在符号前。
	if got := formatMoney(-50.0, "CNY"); got != "-¥50.00" {
		t.Errorf("formatMoney(-50.0,CNY) = %q, want \"-¥50.00\"", got)
	}
}

// TestRenderDimensionBalance 验证余额型维度：显示 Balance 行，不画进度条（无 █/░/N/A%）。
func TestRenderDimensionBalance(t *testing.T) {
	dim := domain.UsageDimension{Name: "可用余额", Balance: 49.58894, Currency: "CNY", PercentUsed: -1}
	got := renderDimension(dim)

	if !strings.Contains(got, "可用余额") {
		t.Errorf("should contain dim name, got: %q", got)
	}
	if !strings.Contains(got, "¥49.59") {
		t.Errorf("should contain Balance ¥49.59, got: %q", got)
	}
	if strings.Contains(got, "█") || strings.Contains(got, "░") {
		t.Errorf("balance dim should NOT render progress bar, got: %q", got)
	}
	if strings.Contains(got, "N/A") {
		t.Errorf("balance dim should NOT show N/A percent, got: %q", got)
	}
}
