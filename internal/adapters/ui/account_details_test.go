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
	"time"

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

// TestRenderBar_SubCell 验证阴影渐变边界：4 格宽度下 23% 落在首格的 7/8 处，
// shadeOf(7)=▓，故渲染为单个深阴影 ▓ + 3 个浅阴影 ░；颜色随进度（green<70）。
func TestRenderBar_SubCell(t *testing.T) {
	got := renderBar(23, 4) // 23% × 4格 × 8 = 7.36 → subs=7 → ▓ + 3 hollow
	if !strings.Contains(got, "▓") {
		t.Errorf("23%% at width 4 should render a ▓ shade boundary, got: %q", got)
	}
	if !strings.Contains(got, "["+colorGreen+"]") {
		t.Errorf("23%% bar should be green: %q", got)
	}
	if strings.Contains(got, "█") {
		t.Errorf("23%% at width 4 should have no full block, got: %q", got)
	}
}

// TestRenderBar_FullCells 验证整格边界与颜色梯度：50%=2 深▓+2 浅░（green），95%=红色。
func TestRenderBar_FullCells(t *testing.T) {
	if got := renderBar(50, 4); !strings.Contains(got, "["+colorGreen+"]▓▓░░") {
		t.Errorf("50%% at width 4 = 2 dark + 2 light shade, got: %q", got)
	}
	if got := renderBar(95, 4); !strings.Contains(got, "["+colorRed+"]") {
		t.Errorf("95%% bar should be red: %q", got)
	}
}

// TestRenderBar_HalfIsCentered 锁定 spec 需求：50% 在偶数宽度下必须正到中间——
// rem==0 不产生边界格，深浅严格各半。details=20 → ▓×10░×10；list=4 → ▓×2░×2。
func TestRenderBar_HalfIsCentered(t *testing.T) {
	for _, w := range []int{4, 20} {
		got := renderBar(50, w)
		want := "[" + colorGreen + "]" + strings.Repeat("▓", w/2) + strings.Repeat("░", w/2) + "[-]"
		if got != want {
			t.Errorf("50%% at width %d = %q, want %q", w, got, want)
		}
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
	dim := domain.UsageDimension{Name: "Available balance", Balance: 49.58894, Currency: "CNY", PercentUsed: -1}
	got := renderDimension(dim)

	if !strings.Contains(got, "Available balance") {
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
	if strings.Contains(got, "Granted:") {
		t.Errorf("zero Granted should not render Granted line, got: %q", got)
	}
}

// TestRenderDimensionBalanceBreakdown 验证余额型维度含非零细分时输出 Granted/Topped up 行。
func TestRenderDimensionBalanceBreakdown(t *testing.T) {
	dim := domain.UsageDimension{
		Name: "Available balance", Balance: 110, Currency: "CNY",
		Granted: 10, ToppedUp: 100, PercentUsed: -1,
	}
	got := renderDimension(dim)
	for _, want := range []string{"Balance:", "¥110.00", "Granted:", "¥10.00", "Topped up:", "¥100.00"} {
		if !strings.Contains(got, want) {
			t.Errorf("breakdown missing %q, got: %q", want, got)
		}
	}
}

// TestRenderStatusLine 验证 Status 非空时 Basic Info 含 Status 行；空时不渲染。
func TestRenderStatusLine(t *testing.T) {
	d := NewAccountDetails()
	u := domain.ProviderUsage{
		Provider: "deepseek", Label: "DS", Status: "active",
		Dimensions: []domain.UsageDimension{{Name: "Available balance", Balance: 1, Currency: "CNY", PercentUsed: -1}},
	}
	u.Primary = &u.Dimensions[0]

	d.Render(u)
	if got := d.GetText(true); !strings.Contains(got, "Status:") || !strings.Contains(got, "active") {
		t.Errorf("should render 'Status: active', got: %q", got)
	}

	u.Status = "" // 空 Status 不渲染该行
	d.Render(u)
	if strings.Contains(d.GetText(true), "Status:") {
		t.Error("empty Status should not render Status line")
	}
}

// TestRenderRecent 验证 Recent 区块渲染键值行。
func TestRenderRecent(t *testing.T) {
	// 有 Recent
	got := renderRecent(domain.RecentUsage{Window7d: 51.2, Window30d: 138.56, RPM: 3, TPM: 1200, Currency: "USD"})
	for _, want := range []string{"Usage (recent)", "7-day:", "$51.20", "30-day:", "$138.56", "Live:", "3 rpm / 1200 tpm"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderRecent missing %q, got: %q", want, got)
		}
	}
}

// TestRenderSkipsRecentWhenNil 验证 Render 在 Recent=nil 时不输出 Usage 区块，非空时输出。
// AccountDetails 内嵌 *tview.TextView，用 GetText(true) 读取渲染后的文本断言。
func TestRenderSkipsRecentWhenNil(t *testing.T) {
	d := NewAccountDetails()
	u := domain.ProviderUsage{
		Provider:   "newapi",
		Label:      "x",
		Dimensions: []domain.UsageDimension{{Name: "Available balance", Balance: 1, Currency: "USD", PercentUsed: -1}},
	}
	u.Primary = &u.Dimensions[0]

	// Recent=nil → 不输出 Usage 区块。
	d.Render(u)
	if strings.Contains(d.GetText(true), "Usage (recent)") {
		t.Error("Render should NOT output Usage block when Recent is nil")
	}

	// Recent 非空 → 输出 Usage 区块。
	u.Recent = &domain.RecentUsage{Window7d: 5, Currency: "USD"}
	d.Render(u)
	if !strings.Contains(d.GetText(true), "Usage (recent)") {
		t.Error("Render should output Usage block when Recent is set")
	}
}

// TestBuildDeleteConfirmMessage 验证删除确认文案含账号名称与 provider、以及"不可撤销"提示。
func TestBuildDeleteConfirmMessage(t *testing.T) {
	got := buildDeleteConfirmMessage("GLM main", "glm")
	for _, want := range []string{"GLM main", "glm", "cannot be undone"} {
		if !strings.Contains(got, want) {
			t.Errorf("delete message missing %q, got: %q", want, got)
		}
	}
}

// TestRenderDimensionMoneyQuota 验证金额配额维度：含 $used/$limit 与进度条，且不走纯 Balance 余额分支。
func TestRenderDimensionMoneyQuota(t *testing.T) {
	dim := domain.UsageDimension{Name: "5h window", MoneyLimit: 20, MoneyUsed: 7, Balance: 13, Currency: "USD", PercentUsed: 35, ResetsAt: time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)}
	got := renderDimension(dim)
	for _, want := range []string{"5h window", "$7.00 / $20.00", "35%", "Resets:"} {
		if !strings.Contains(got, want) {
			t.Errorf("money-quota dim missing %q, got: %q", want, got)
		}
	}
}

// TestRenderRecentTodayTotal 验证 sub2api 今日/累计统计行（Window7d/30d 为零时不显示）。
func TestRenderRecentTodayTotal(t *testing.T) {
	got := renderRecent(domain.RecentUsage{TodayCost: 1.5, TotalCost: 15.0, TodayTokens: 3050, TotalTokens: 30000, RPM: 5, TPM: 1500, AvgDurationMs: 2500, Currency: "USD"})
	for _, want := range []string{"Today:", "$1.50", "Total:", "$15.00", "Live:", "5 rpm / 1500 tpm", "Avg:", "2500ms"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderRecent(today/total) missing %q, got: %q", want, got)
		}
	}
	if strings.Contains(got, "7-day:") || strings.Contains(got, "30-day:") {
		t.Errorf("zero Window7d/30d should not render, got: %q", got)
	}
}

// TestRender_DimensionsOrderedByOrder 验证详情按 Order 为主键排序：即便 5h 缺 ResetsAt
// （GLM 偶发），它也因 Order=1 排在 weekly(Order=2)/MCP(Order=3) 之前。
func TestRender_DimensionsOrderedByOrder(t *testing.T) {
	d := NewAccountDetails()
	now := time.Now()
	// 故意把 weekly 放在 Dimensions 切片最前，验证 Render 会按 Order 重排而非保持切片序。
	u := domain.ProviderUsage{
		Provider: "glm",
		Label:    "智谱",
		Dimensions: []domain.UsageDimension{
			{Name: "Weekly Quota", PercentUsed: 53, Order: 2, ResetsAt: now.Add(7 * 24 * time.Hour)},
			{Name: "MCP Monthly", PercentUsed: 7, Order: 3, ResetsAt: now.Add(30 * 24 * time.Hour)},
			{Name: "5h Quota", PercentUsed: 44, Order: 1}, // 无 ResetsAt
		},
	}
	d.Render(u)
	got := d.GetText(true)

	// 三个维度名在输出中应按 5h → weekly → MCP 的顺序出现。
	i5h := strings.Index(got, "5h Quota")
	iWeekly := strings.Index(got, "Weekly Quota")
	iMCP := strings.Index(got, "MCP Monthly")
	if i5h < 0 || iWeekly < 0 || iMCP < 0 {
		t.Fatalf("missing dim names in render: 5h=%d weekly=%d mcp=%d\n%s", i5h, iWeekly, iMCP, got)
	}
	if i5h >= iWeekly || iWeekly >= iMCP {
		t.Errorf("dim order wrong: 5h=%d weekly=%d mcp=%d, want 5h<weekly<mcp", i5h, iWeekly, iMCP)
	}
}

// TestRenderDimensionSiliconFlowBreakdown 验证 SiliconFlow 余额维度输出 Charged/Total 行。
func TestRenderDimensionSiliconFlowBreakdown(t *testing.T) {
	dim := domain.UsageDimension{
		Name: "Available balance", Balance: 0.88, Currency: "CNY",
		ChargeBalance: 88.0, TotalBalance: 88.88, PercentUsed: -1,
	}
	got := renderDimension(dim)
	for _, want := range []string{"Balance:", "¥0.88", "Charged:", "¥88.00", "Total:", "¥88.88"} {
		if !strings.Contains(got, want) {
			t.Errorf("siliconflow breakdown missing %q, got: %q", want, got)
		}
	}
}

// TestRenderDimensionSiliconFlowZeroBreakdown 验证零值细分不渲染 Charged/Total 行。
func TestRenderDimensionSiliconFlowZeroBreakdown(t *testing.T) {
	dim := domain.UsageDimension{
		Name: "Available balance", Balance: 5.0, Currency: "CNY", PercentUsed: -1,
	}
	got := renderDimension(dim)
	if strings.Contains(got, "Charged:") {
		t.Errorf("zero ChargeBalance should not render Charged line, got: %q", got)
	}
	if strings.Contains(got, "Total:") {
		t.Errorf("zero TotalBalance should not render Total line, got: %q", got)
	}
}
