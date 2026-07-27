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

// restoreColors 在测试结束后把活动配色复位为默认，防止跨用例污染（activeColors 是进程级状态）。
func restoreColors(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { applyColorScheme(domain.ColorsConfig{}) })
}

// TestResolveColor 预设名→palette hex（大小写不敏感）；#RRGGBB 原样；非法→false。
func TestResolveColor(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"green", colorGreen, true},
		{"RED", colorRed, true}, // 大小写不敏感
		{"#10B981", "#10B981", true},
		{"nope", "", false},
		{"#xyz", "", false},
	}
	for _, tc := range cases {
		got, ok := resolveColor(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("resolveColor(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestResolveThresholds 校验：颜色数≠阈值+1 或空阈值 → 回退（ok=false）。
func TestResolveThresholds(t *testing.T) {
	if _, ok := resolveThresholds(domain.ThresholdColors{Thresholds: []float64{70}}); ok {
		t.Error("colors length mismatch should fail")
	}
	if _, ok := resolveThresholds(domain.ThresholdColors{Thresholds: []float64{70, 90}, Colors: []string{"green", "yellow", "red"}}); !ok {
		t.Error("valid quota should resolve")
	}
	// 非法颜色 → 回退
	if _, ok := resolveThresholds(domain.ThresholdColors{Thresholds: []float64{1}, Colors: []string{"green", "nope"}}); ok {
		t.Error("illegal color should fail")
	}
}

// TestResolveColors_FallbackDefault 零值配置 → 默认 [70,90]/[10,1]。
func TestResolveColors_FallbackDefault(t *testing.T) {
	s := resolveColors(domain.ColorsConfig{})
	if len(s.quota.Thresholds) != 2 || s.quota.Thresholds[0] != 70 || s.quota.Thresholds[1] != 91 {
		t.Errorf("default quota thresholds = %v, want [70 91] (91 复现旧契约 90=last yellow)", s.quota.Thresholds)
	}
	if len(s.balance.Thresholds) != 2 || s.balance.Thresholds[0] != 10 || s.balance.Thresholds[1] != 1 {
		t.Errorf("default balance thresholds = %v, want [10 1]", s.balance.Thresholds)
	}
}

// TestPickByQuota 升序：<70 绿、[70,90) 黄、>=90 红。
func TestPickByQuota(t *testing.T) {
	tc := domain.ThresholdColors{Thresholds: []float64{70, 91}, Colors: []string{colorGreen, colorYellow, colorRed}}
	cases := []struct {
		pct  float64
		want string
	}{
		{0, colorGreen},
		{69, colorGreen},
		{70, colorYellow},
		{89, colorYellow},
		{90, colorYellow},
		{91, colorRed},
		{120, colorRed},
	}
	for _, c := range cases {
		if got := pickByQuota(tc, c.pct); got != c.want {
			t.Errorf("pickByQuota(%v) = %q, want %q", c.pct, got, c.want)
		}
	}
}

// TestPickByBalance 降序（含负值）：>=10 绿、[1,10) 黄、<1 红。
func TestPickByBalance(t *testing.T) {
	tc := domain.ThresholdColors{Thresholds: []float64{10, 1}, Colors: []string{colorGreen, colorYellow, colorRed}}
	cases := []struct {
		bal  float64
		want string
	}{
		{100, colorGreen},
		{10, colorGreen},
		{9.99, colorYellow},
		{1, colorYellow},
		{0.99, colorRed},
		{0, colorRed},
		{-5, colorRed},
	}
	for _, c := range cases {
		if got := pickByBalance(tc, c.bal); got != c.want {
			t.Errorf("pickByBalance(%v) = %q, want %q", c.bal, got, c.want)
		}
	}
}

// TestApplyColorScheme_OverridesDefault 自定义配置经 applyColorScheme 后 StatusColor/BalanceColor 生效。
func TestApplyColorScheme_OverridesDefault(t *testing.T) {
	restoreColors(t)
	applyColorScheme(domain.ColorsConfig{
		Quota:   domain.ThresholdColors{Thresholds: []float64{50}, Colors: []string{"green", "red"}},
		Balance: domain.ThresholdColors{Thresholds: []float64{100}, Colors: []string{"red", "green"}},
	})
	// quota: <50 green；50 不 <50 → 兜底 red
	if got := StatusColor(40); got != colorGreen {
		t.Errorf("StatusColor(40) = %q, want green", got)
	}
	if got := StatusColor(60); got != colorRed {
		t.Errorf("StatusColor(60) = %q, want red", got)
	}
	// balance: >=100 red（降序首档）；<100 兜底 green
	if got := BalanceColor(150, "USD"); got != colorRed {
		t.Errorf("BalanceColor(150) = %q, want red", got)
	}
	if got := BalanceColor(50, "USD"); got != colorGreen {
		t.Errorf("BalanceColor(50) = %q, want green", got)
	}
}

// TestStatusColor_NAStillGray 验证 pct<0（N/A）仍固定灰，不受阈值配置影响。
func TestStatusColor_NAStillGray(t *testing.T) {
	restoreColors(t)
	applyColorScheme(domain.ColorsConfig{
		Quota: domain.ThresholdColors{Thresholds: []float64{50}, Colors: []string{"red", "red"}},
	})
	if got := StatusColor(-1); got != colorGray {
		t.Errorf("StatusColor(-1) = %q, want gray (N/A must stay gray regardless of config)", got)
	}
}
