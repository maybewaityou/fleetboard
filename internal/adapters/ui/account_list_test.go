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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// TestFormatAccountLine_WithPrimary verifies the row carries the label, a
// vendor chip on the GLM brand color, the integer percent, and a solid status
// dot colored by StatusColor (green at 30%).
func TestFormatAccountLine_WithPrimary(t *testing.T) {
	u := domain.VendorUsage{
		AccountID: "a1",
		Vendor:    "glm",
		Label:     "prod-glm",
		Primary: &domain.UsageDimension{
			Name:        "GLM-4.5",
			Used:        3000,
			Limit:       10000,
			PercentUsed: 30,
		},
	}
	got := formatAccountLine(u)

	// label present
	if !strings.Contains(got, "prod-glm") {
		t.Errorf("missing label %q in: %q", "prod-glm", got)
	}
	// vendor chip: unified accent background (lazytmux tagChip style), black text
	if !strings.Contains(got, "[black:"+colorAccent+"]") {
		t.Errorf("missing vendor chip [black:%s] in: %q", colorAccent, got)
	}
	if !strings.Contains(got, "glm") {
		t.Errorf("missing vendor text in: %q", got)
	}
	// integer percent
	if !strings.Contains(got, "30%") {
		t.Errorf("missing percent in: %q", got)
	}
	// solid dot, colored green (StatusColor(30) == green)
	dotCol := StatusColor(30)
	if !strings.Contains(got, "["+dotCol+"]●[-]") {
		t.Errorf("missing green solid dot %q in: %q", "["+dotCol+"]●[-]", got)
	}
	// must NOT show N/A or hollow dot
	if strings.Contains(got, "N/A") {
		t.Errorf("must not show N/A when Primary set: %q", got)
	}
	if strings.Contains(got, "○") {
		t.Errorf("must not show hollow dot when Primary set: %q", got)
	}
	// no error marker when Err nil
	if strings.Contains(got, "⚠") {
		t.Errorf("must not show ⚠ when Err nil: %q", got)
	}
}

// TestFormatAccountLine_NoPrimary verifies the N/A branch: percent reads "N/A",
// the dot is the hollow ○, and its color is StatusColor(-1) (gray) so the dim
// state reads distinctly from a healthy account.
func TestFormatAccountLine_NoPrimary(t *testing.T) {
	u := domain.VendorUsage{
		AccountID: "a2",
		Vendor:    "kimi",
		Label:     "dev-kimi",
		Primary:   nil, // no usable dimension
	}
	got := formatAccountLine(u)

	if !strings.Contains(got, "N/A") {
		t.Errorf("missing N/A in: %q", got)
	}
	dotCol := StatusColor(-1) // gray
	if !strings.Contains(got, "["+dotCol+"]○[-]") {
		t.Errorf("missing gray hollow dot %q in: %q", "["+dotCol+"]○[-]", got)
	}
	if strings.Contains(got, "●") {
		t.Errorf("must not show solid dot when Primary nil: %q", got)
	}
}

// TestFormatAccountLine_ErrMarker verifies the err-transparency contract
// (task-7): a failed fetch prefixes a red ⚠ but still renders label/vendor so
// the account is not hidden — its existing dimensions remain explorable in the
// details pane.
func TestFormatAccountLine_ErrMarker(t *testing.T) {
	u := domain.VendorUsage{
		AccountID: "a3",
		Vendor:    "openai",
		Label:     "broken",
		Primary: &domain.UsageDimension{
			Name:        "gpt-4",
			PercentUsed: 95,
		},
		Err: errSentinel,
	}
	got := formatAccountLine(u)
	if !strings.Contains(got, "⚠") {
		t.Errorf("err marker missing in: %q", got)
	}
	if !strings.Contains(got, "broken") {
		t.Errorf("label must still render despite Err: %q", got)
	}
	// 95% → red dot
	if !strings.Contains(got, "["+colorRed+"]●[-]") {
		t.Errorf("95%% should be red dot: %q", got)
	}
}

// TestFormatAccountLine_UnknownVendor verifies an unrecognized vendor falls back
// to the gray tag (VendorTag contract) rather than a broken color block.
func TestFormatAccountLine_UnknownVendor(t *testing.T) {
	u := domain.VendorUsage{
		AccountID: "a4",
		Vendor:    "weird-vendor",
		Label:     "x",
		Primary:   &domain.UsageDimension{PercentUsed: 10},
	}
	got := formatAccountLine(u)
	// vendor chip is unified accent regardless of vendor identity
	if !strings.Contains(got, "[black:"+colorAccent+"]") {
		t.Errorf("vendor chip must be unified accent [black:%s]: %q", colorAccent, got)
	}
}

// TestFormatAccountLine_Pinned 验证置顶行首显示绿色 📌 marker；未置顶则无 📌。
func TestFormatAccountLine_Pinned(t *testing.T) {
	u := domain.VendorUsage{
		AccountID: "p1", Vendor: "glm", Label: "pinned-acct",
		Primary: &domain.UsageDimension{PercentUsed: 50},
		Pinned:  true,
	}
	got := formatAccountLine(u)
	if !strings.Contains(got, "["+colorGreen+"]📌[-]") {
		t.Errorf("pinned row missing 📌 marker: %q", got)
	}

	u.Pinned = false
	if got2 := formatAccountLine(u); strings.Contains(got2, "📌") {
		t.Errorf("unpinned row must not show 📌: %q", got2)
	}
}

// TestFormatAccountLine_FetchedTime 验证行尾显示最近刷新相对时间（Xm ago）。
func TestFormatAccountLine_FetchedTime(t *testing.T) {
	u := domain.VendorUsage{
		AccountID: "a", Vendor: "glm", Label: "l",
		Primary:   &domain.UsageDimension{PercentUsed: 50},
		FetchedAt: time.Now().Add(-5 * time.Minute),
	}
	got := formatAccountLine(u)
	if !strings.Contains(got, "m ago") {
		t.Errorf("fetched relative time missing in: %q", got)
	}
}

// TestFormatAccountLine_ColumnAlignment 守护跨行对齐不变量：进度条起点、状态点、
// "Last Refreshed" 文本起点必须在所有行落在同一列，无论 vendor slug 长短、数值是百分比
// 还是余额、正负、或 N/A。挡住两类旧回归：① %-Ns 左对齐让状态点随数值长度漂移；
// ② 长 vendor (anthropic/deepseek) 超出定宽把整条右半部右移。
func TestFormatAccountLine_ColumnAlignment(t *testing.T) {
	cases := []domain.VendorUsage{
		{AccountID: "1", Vendor: "glm", Label: "short", Primary: &domain.UsageDimension{PercentUsed: 7}},
		{AccountID: "2", Vendor: "anthropic", Label: "longer-label", Primary: &domain.UsageDimension{Balance: 49.6, Currency: "CNY", PercentUsed: -1}},
		{AccountID: "3", Vendor: "deepseek", Label: "x", Primary: &domain.UsageDimension{Balance: -1500, Currency: "USD", PercentUsed: -1}},
		{AccountID: "4", Vendor: "openai", Label: "y", Primary: &domain.UsageDimension{PercentUsed: 100}},
		{AccountID: "5", Vendor: "kimi", Label: "z", Primary: nil}, // N/A → 灰条 + ○
	}
	// 度量用"显示列"（tview.TaggedStringWidth）而非 byte/rune 索引：pctStr 列含
	// 多字节货币符号 ¥（2 byte 但显示宽 1），padDisplay 按显示宽度补齐，所以只有
	// 显示列恒定——byte 索引会因 ¥ 多出 1 字节而误报错位。
	barCol, dotCol, refreshCol := -1, -1, -1
	for _, u := range cases {
		raw := stripTags(formatAccountLine(u))

		// 进度条起点：第一个 shade 字形 ▓/░
		i := strings.IndexAny(raw, "▓░")
		if i < 0 {
			t.Fatalf("no bar cell in: %q", raw)
		}
		c := tview.TaggedStringWidth(raw[:i])
		if barCol == -1 {
			barCol = c
		} else if c != barCol {
			t.Errorf("bar start misaligned for %q: display col %d want %d\n  %q", u.Label, c, barCol, raw)
		}

		// 状态点 ●/○
		i = strings.IndexAny(raw, "●○")
		if i < 0 {
			t.Fatalf("no status dot in: %q", raw)
		}
		c = tview.TaggedStringWidth(raw[:i])
		if dotCol == -1 {
			dotCol = c
		} else if c != dotCol {
			t.Errorf("status dot misaligned for %q: display col %d want %d\n  %q", u.Label, c, dotCol, raw)
		}

		// "Last Refreshed" 文本起点
		i = strings.Index(raw, "Last Refreshed")
		if i < 0 {
			t.Fatalf("no Last Refreshed in: %q", raw)
		}
		c = tview.TaggedStringWidth(raw[:i])
		if refreshCol == -1 {
			refreshCol = c
		} else if c != refreshCol {
			t.Errorf("Last Refreshed misaligned for %q: display col %d want %d\n  %q", u.Label, c, refreshCol, raw)
		}
	}
}

// tagRe 匹配 tview 颜色/区域标签 ([...]，内部不含 ']'）。tview v0.42 无导出的
// StripTags，测试在此本地剥离以便对纯可见文本断言对齐。
var tagRe = regexp.MustCompile(`\[[^\]]*\]`)

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

// TestHumanizeAgo 验证相对时间格式（零值→—；过去→含 ago 单位）。
func TestHumanizeAgo(t *testing.T) {
	if got := humanizeAgo(time.Time{}); got != "—" {
		t.Errorf("zero → %q, want —", got)
	}
	if got := humanizeAgo(time.Now().Add(-2 * time.Hour)); !strings.Contains(got, "h ago") {
		t.Errorf("2h ago → %q, want contain 'h ago'", got)
	}
	if got := humanizeAgo(time.Now().Add(-3 * 24 * time.Hour)); !strings.Contains(got, "d ago") {
		t.Errorf("3d ago → %q, want contain 'd ago'", got)
	}
}

// TestPadDisplay_CJKAlignsByDisplayWidth 验证 padDisplay 按显示宽度 pad（中文每字 2 宽）。
func TestPadDisplay_CJKAlignsByDisplayWidth(t *testing.T) {
	got := padDisplay("智谱", 8) // 智谱=4 显示宽 → 补 4 空格到 8
	if n := strings.Count(got, " "); n != 4 {
		t.Errorf("padDisplay(\"智谱\",8) should add 4 spaces, got %d: %q", n, got)
	}
}

// TestPrimaryPercent covers the helper that StatusColor feeds on: nil → -1,
// otherwise the dimension's percent.
func TestPrimaryPercent(t *testing.T) {
	if got := primaryPercent(domain.VendorUsage{}); got != -1 {
		t.Errorf("nil Primary = %v, want -1", got)
	}
	u := domain.VendorUsage{Primary: &domain.UsageDimension{PercentUsed: 42.5}}
	if got := primaryPercent(u); got != 42.5 {
		t.Errorf("Primary.PercentUsed = %v, want 42.5", got)
	}
}

// TestSelectByAccountID verifies selection survives a refresh-driven
// UpdateUsages (the pattern Run/Render uses to keep the user's cursor stable).
func TestSelectByAccountID(t *testing.T) {
	al := NewAccountList()
	al.UpdateUsages([]domain.VendorUsage{
		{AccountID: "a1", Vendor: "glm", Label: "one"},
		{AccountID: "a2", Vendor: "glm", Label: "two"},
		{AccountID: "a3", Vendor: "glm", Label: "three"},
	})
	if got := al.GetCurrentItem(); got != 0 {
		t.Fatalf("initial cursor = %d, want 0", got)
	}
	al.SelectByAccountID("a2")
	if got := al.GetCurrentItem(); got != 1 {
		t.Fatalf("after SelectByAccountID(a2) = %d, want 1", got)
	}
	// reload snaps cursor to 0; SelectByAccountID restores it.
	al.UpdateUsages(al.usages)
	if got := al.GetCurrentItem(); got != 0 {
		t.Fatalf("cursor after reload = %d, want 0", got)
	}
	al.SelectByAccountID("a2")
	if got := al.GetCurrentItem(); got != 1 {
		t.Errorf("cursor after restore = %d, want 1", got)
	}
	if u, ok := al.GetSelected(); !ok || u.AccountID != "a2" {
		t.Errorf("GetSelected = (%+v, %v), want (a2, true)", u, ok)
	}
}

// TestRenderDimension_BarFill checks the bar fills proportionally and colors by
// StatusColor (red above 90).
func TestRenderDimension_BarFill(t *testing.T) {
	dim := domain.UsageDimension{
		Name:        "GLM-4.5",
		Used:        950,
		Limit:       1000,
		PercentUsed: 95,
		Remaining:   50,
		Unit:        "tok",
		ResetsAt:    time.Now().Add(2 * time.Hour),
		Source:      "api-balanced",
	}
	got := renderDimension(dim)
	if !strings.Contains(got, "▓") || !strings.Contains(got, "░") {
		t.Errorf("bar missing fill/hollow cells: %q", got)
	}
	// 95% → red bar prefix
	if !strings.Contains(got, "["+colorRed+"]") {
		t.Errorf("95%% bar should be red: %q", got)
	}
	if !strings.Contains(got, "95%") {
		t.Errorf("missing percent: %q", got)
	}
	if !strings.Contains(got, "tok") {
		t.Errorf("missing unit: %q", got)
	}
}

// TestRenderDimension_NA verifies a PercentUsed<0 dimension renders an all-gray
// hollow bar and "N/A".
func TestRenderDimension_NA(t *testing.T) {
	dim := domain.UsageDimension{Name: "x", PercentUsed: -1}
	got := renderDimension(dim)
	if !strings.Contains(got, "N/A") {
		t.Errorf("missing N/A: %q", got)
	}
	if !strings.Contains(got, "["+colorGray+"]"+strings.Repeat("░", barWidth)+"[-]") {
		t.Errorf("missing all-gray hollow bar: %q", got)
	}
}

// TestFormatMoneyShort 验证余额短格式：CNY→¥、USD→$、>1000 缩写 k、1 位小数；
// 负值负号置于符号前（spec §3，-¥50.0 而非 ¥-50.0），含 k 分支。
func TestFormatMoneyShort(t *testing.T) {
	cases := []struct {
		balance  float64
		currency string
		want     string
	}{
		{49.58894, "CNY", "¥49.6"},
		{3.0, "USD", "$3.0"},
		{1200.0, "CNY", "¥1.2k"},
		{0, "CNY", "¥0.0"},
		// M2: 负值 — 负号在符号前，普通与 k 分支各一。
		{-50.0, "CNY", "-¥50.0"},
		{-1500.0, "USD", "-$1.5k"},
	}
	for _, tc := range cases {
		if got := formatMoneyShort(tc.balance, tc.currency); got != tc.want {
			t.Errorf("formatMoneyShort(%v,%q) = %q, want %q", tc.balance, tc.currency, got, tc.want)
		}
	}
}

// TestFormatAccountLineBalance 验证余额型行渲染：含余额短格式、绿点（余额>0）、
// 灰色 miniBar（renderBar(-1,4) 自然得灰条）。
func TestFormatAccountLineBalance(t *testing.T) {
	balDim := domain.UsageDimension{Name: "可用余额", Balance: 49.58894, Currency: "CNY", PercentUsed: -1}
	u := domain.VendorUsage{AccountID: "k", Vendor: "kimi", Label: "Kimi-主力", Primary: &balDim, Dimensions: []domain.UsageDimension{balDim}}
	got := formatAccountLine(u)

	if !strings.Contains(got, "¥49.6") {
		t.Errorf("balance line should contain ¥49.6, got: %q", got)
	}
	if !strings.Contains(got, "["+colorGreen+"]") {
		t.Errorf("balance>0 should render green dot, got: %q", got)
	}
	if !strings.Contains(got, "["+colorGray+"]") {
		t.Errorf("balance line should have gray miniBar, got: %q", got)
	}
}

// TestFormatAccountLineBalanceDepleted 验证余额<=0 渲染红点。
func TestFormatAccountLineBalanceDepleted(t *testing.T) {
	balDim := domain.UsageDimension{Name: "可用余额", Balance: 0, Currency: "CNY", PercentUsed: -1}
	u := domain.VendorUsage{AccountID: "d", Vendor: "deepseek", Label: "DS", Primary: &balDim}
	got := formatAccountLine(u)
	if !strings.Contains(got, "["+colorRed+"]") {
		t.Errorf("balance<=0 should render red dot, got: %q", got)
	}
}

// errSentinel is a stable non-nil error for the err-marker test.
var errSentinel = errStr("boom")

type errStr string

func (e errStr) Error() string { return string(e) }
