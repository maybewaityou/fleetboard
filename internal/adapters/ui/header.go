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
	"time"

	"github.com/rivo/tview"
)

// RepoURL is the project link shown in the header. Centralized so a rename only
// touches one place.
const RepoURL = "github.com/maybewaityou/fleetboard"

// AppHeader is the top bar: brand on the left, version/commit in the center,
// repo link + clock on the right. Ported from lazytmux/header.go with the brand
// text swapped; the layout (3-column flex + underline separator) is identical.
type AppHeader struct {
	*tview.Flex
}

func NewAppHeader(version, commit, repoURL string) *AppHeader {
	h := &AppHeader{Flex: tview.NewFlex()}
	h.build(version, commit, repoURL)
	return h
}

func (h *AppHeader) build(version, commit, repoURL string) {
	left := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignLeft)
	left.SetText("📊 [" + colorPrimary + "::b]fleet[-][" + colorAccent + "::b]board[-]")

	center := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	center.SetText(makeTag(version, colorGreen) + "  " + makeTag(shortCommit(commit), colorPurple))

	right := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	currentTime := time.Now().Format("Mon, 02 Jan 2006 15:04")
	right.SetText("[" + colorAccent + "]🔗 " + repoURL + "[-]  [" + colorSecondary + "]• " + currentTime + "[-]")

	bar := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 0, 1, false).
		AddItem(center, 0, 1, false).
		AddItem(right, 0, 1, false)

	sep := tview.NewTextView().SetDynamicColors(true)
	sep.SetText("[" + colorBorder + "]" + strings.Repeat("─", 200) + "[-]")

	h.Flex.SetDirection(tview.FlexRow).AddItem(bar, 1, 0, false).AddItem(sep, 1, 0, false)
}

// shortCommit trims a git SHA to 7 chars and hides "unknown"/empty values so the
// header never shows a noisy placeholder chip.
func shortCommit(c string) string {
	c = strings.TrimSpace(c)
	if c == "" || c == "unknown" {
		return ""
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// makeTag renders text as a black-on-bg bold chip. Ported verbatim from
// lazytmux/header.go so version/commit chips look identical across tools.
func makeTag(text, bg string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "[black:" + bg + ":b]  " + text + "  [-]"
}
