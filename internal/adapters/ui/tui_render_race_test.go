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
	"sync"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// TestRender_AllCacheWriteIsMarshalledToMainLoop reproduces the race a reviewer
// flagged: Render used to assign t.allCache on the *caller's* goroutine (the
// background goroutine spawned by doRefreshSelected/doRefreshAll), while the
// tview main loop reads t.allCache through visibleUsages() (driven by
// handleSearchInput on every keystroke). With refresh in flight and the user
// typing, the slice header is read and written concurrently.
//
// This test stands up a single-goroutine "main loop" that drains queued funcs
// (the same serialization tview's QueueUpdateDraw provides) and then fires
// background Render callers against main-loop visibleUsages readers. Run under
// `go test -race`, the OLD implementation trips a data-race report on
// t.allCache; the fixed implementation (allCache write inside the queueDraw
// callback) is clean because every allCache access is on the main loop.
//
// We cannot spin a real *tview.Application here (it needs a TTY), but the race
// is purely about field access ordering, which the fake main loop models
// faithfully: queueDraw callbacks run on exactly one goroutine, exactly like
// tview's event loop.
func TestRender_AllCacheWriteIsMarshalledToMainLoop(t *testing.T) {
	tui := NewTUI(Config{})
	// Build the leaf components (list/details/search/status) without Run() so
	// applyCacheToViews has real primitives to mutate. tview model methods
	// (Clear/AddItem/SetText) don't require a Screen — drawing is separate.
	tui.buildComponents()

	work := make(chan func(), 8192)
	quit := make(chan struct{})
	// main loop: the single goroutine all queueDraw callbacks run on.
	go func() {
		for {
			select {
			case f := <-work:
				f()
			case <-quit:
				return
			}
		}
	}()
	defer close(quit)
	tui.queueDraw = func(f func()) { work <- f }

	const writers = 6
	const readers = 4
	const iters = 150

	var wg sync.WaitGroup

	// Background Render callers — stand-ins for the goroutines spawned in
	// doRefreshSelected / doRefreshAll (and, eventually, the Task 12 service).
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				tui.Render([]domain.ProviderUsage{
					{AccountID: fmt.Sprintf("a-%d-%d", n, j), Provider: "glm", Label: fmt.Sprintf("acct-%d-%d", n, j)},
				})
			}
		}(w)
	}

	// Main-loop readers — stand-ins for handleSearchInput, which runs on the
	// tview loop and calls visibleUsages (reading allCache). We submit them
	// through queueDraw so they execute on the same goroutine as the Render
	// callbacks, matching real scheduling.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				tui.queueDraw(func() {
					_ = tui.visibleUsages()
				})
			}
		}()
	}

	wg.Wait()

	// Drain any straggler callbacks so the final assertion sees settled state.
	// A sync.Once gate via queueDraw tells us the main loop has processed
	// everything queued above before we read allCache from the test goroutine.
	drained := make(chan struct{})
	tui.queueDraw(func() {
		last := tui.allCache[len(tui.allCache)-1]
		if last.AccountID == "" {
			t.Errorf("unexpected empty last usage after concurrent renders")
		}
		close(drained)
	})
	<-drained
}
