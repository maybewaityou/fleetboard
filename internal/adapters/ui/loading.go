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
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// spinnerInterval is the frame rate of the boot splash spinner. Fast enough to
// read as motion (~8 fps), slow enough not to peg the CPU / burn battery on the
// few seconds the initial fetch takes. The ticker is owned by bootAsync and
// stopped the instant the dataset lands, so it never outlives loading.
const spinnerInterval = 120 * time.Millisecond

// spinnerFrames is the canonical braille spinner. These glyphs render fine in
// the Termux/Android terminals fleetboard targets; if a terminal's font lacks
// them we'd fall back to ASCII here (not needed yet).
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// LoadingView is the centered boot splash shown while the initial usage fetch
// is in flight. It is a plain TextView (dynamic colors, centered) whose text is
// rewritten each spinner tick by SetFrame. tview has no cmd/msg stream, so the
// animation is driven the only way it can be: a goroutine ticking every
// spinnerInterval and pushing the next frame through queueDraw.
type LoadingView struct {
	*tview.TextView
}

// NewLoadingView builds the splash text view. The first frame is painted
// immediately so the very first drawn frame is not empty.
func NewLoadingView() *LoadingView {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetBackgroundColor(tcell.ColorDefault)
	lv := &LoadingView{TextView: tv}
	lv.SetFrame(0, "Loading accounts…")
	return lv
}

// SetFrame advances the spinner to frame (mod len(spinnerFrames)) and rewrites
// the whole splash: app name on top, blank line, then spinner + label. SetText
// (not io.Writer append) keeps the buffer from growing one frame per tick.
func (l *LoadingView) SetFrame(frame int, label string) {
	sp := spinnerFrames[frame%len(spinnerFrames)]
	l.SetText(fmt.Sprintf("[%s::b]fleetboard[-]\n\n[%s]%s[-]  %s",
		colorAccent, colorCyan, sp, label))
}

// newLoadingRoot wraps the splash in a row-Flex with flexible top/bottom nil
// spacers so the 3-line block sits at the vertical middle of the screen
// (mirrors openHelp's centering idiom). Layout only — input handling and focus
// are wired by bootAsync, which has the *tview.Application reference.
func newLoadingRoot(lv *LoadingView) *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false). // top spacer — flexible
		AddItem(lv, 3, 0, true).   // 3-line splash — fixed
		AddItem(nil, 0, 1, false)  // bottom spacer — flexible
}
