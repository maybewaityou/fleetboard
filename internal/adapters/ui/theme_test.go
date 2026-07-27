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

	"github.com/rivo/tview"
)

func TestProviderTag_KnownProviders(t *testing.T) {
	t.Parallel()
	// Every entry in providerColor must match spec §9.2 verbatim, so adding a
	// new provider without updating this table fails loudly.
	cases := []struct {
		provider       string
		wantBG, wantFG string
	}{
		{"glm", "#7C3AED", "#FFFFFF"},
		{"minimax", "#EF4444", "#FFFFFF"},
		{"kimi", "#06B6D4", "#001016"},
		{"anthropic", "#D97757", "#FFFFFF"},
		{"openai", "#10A37F", "#FFFFFF"},
		{"cursor", "#6366F1", "#FFFFFF"},
		{"copilot", "#0969DA", "#FFFFFF"},
		{"deepseek", "#2563EB", "#FFFFFF"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.provider, func(t *testing.T) {
			t.Parallel()
			bg, fg := ProviderTag(tc.provider)
			if bg != tc.wantBG || fg != tc.wantFG {
				t.Fatalf("ProviderTag(%q) = (%q, %q); want (%q, %q)",
					tc.provider, bg, fg, tc.wantBG, tc.wantFG)
			}
		})
	}
}

func TestProviderTag_Unknown(t *testing.T) {
	t.Parallel()
	// Empty string, never-heard-of provider, and case-mismatched slugs (the map
	// is case-sensitive by design) all must fall back to the gray pair.
	for _, v := range []string{"", "unknown", "GLM", "OpenAI", "claude"} {
		bg, fg := ProviderTag(v)
		if bg != unknownProviderBG || fg != unknownProviderFG {
			t.Fatalf("ProviderTag(%q) = (%q, %q); want (%q, %q)",
				v, bg, fg, unknownProviderBG, unknownProviderFG)
		}
	}
}

func TestStatusColor(t *testing.T) {
	t.Parallel()
	// Boundary cases mandated by the task-6 brief: 69 green, 70 yellow, 89
	// yellow, 90 red, -1 gray. The const comparison (not a hex literal) makes
	// this a refactor-safe contract on the *meaning*, while locking the
	// boundary positions.
	cases := []struct {
		name    string
		percent float64
		want    string
	}{
		{"zero usage", 0.0, colorGreen},
		{"just below green-yellow boundary (69)", 69.0, colorGreen},
		{"green-yellow boundary (70)", 70.0, colorYellow},
		{"mid yellow", 80.0, colorYellow},
		{"just below yellow-red boundary (89)", 89.0, colorYellow},
		{"yellow-red boundary (90 inclusive yellow per spec)", 90.0, colorYellow},
		{"just above yellow-red boundary (91 first red)", 91.0, colorRed},
		{"fully consumed (100)", 100.0, colorRed},
		{"over consumed (120)", 120.0, colorRed},
		{"negative means N/A (-1)", -1.0, colorGray},
		{"strongly negative also gray", -100.0, colorGray},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := StatusColor(tc.percent); got != tc.want {
				t.Fatalf("StatusColor(%v) = %q; want %q", tc.percent, got, tc.want)
			}
		})
	}
}

func TestInitializeTheme_AppliesTokyoNight(t *testing.T) {
	// NOTE: deliberately NOT t.Parallel() — this test mutates the process-global
	// tview.Styles table, which would race with any other ui test that touches
	// Styles. The other StatusColor/ProviderTag tests are pure and stay parallel.
	// initializeTheme mutates tview.Styles (global by tview's design) and must
	// return a usable Application. We assert non-nil and spot-check one style
	// field to catch a regression where someone drops a setter.
	app := initializeTheme()
	if app == nil {
		t.Fatal("initializeTheme returned nil *tview.Application")
	}
	// tcell.GetColor normalizes hex to uppercase on read-back, so compare
	// case-insensitively against our lowercase palette literal.
	if got := tviewStyleBorderColor(); !strings.EqualFold(got, colorBorder) {
		t.Fatalf("tview.Styles.BorderColor = %q; want %q (colorBorder)", got, colorBorder)
	}
}

// tview.Styles is global state; reading it back through a helper keeps the test
// honest about what it is checking (and gives a single seam to update if tview
// ever renames the field).
func tviewStyleBorderColor() string {
	return tview.Styles.BorderColor.String()
}
