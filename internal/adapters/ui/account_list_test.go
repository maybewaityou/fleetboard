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
	if !strings.Contains(got, "█") || !strings.Contains(got, "░") {
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

// errSentinel is a stable non-nil error for the err-marker test.
var errSentinel = errStr("boom")

type errStr string

func (e errStr) Error() string { return string(e) }
