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

// TestVisibleSorted_PinnedFirst 验证置顶项排到列表顶部，未置顶项保持原序（稳定排序）。
// 这是 pin 功能的核心可见行为：toggle 后置顶项立即钉在顶部。
func TestVisibleSorted_PinnedFirst(t *testing.T) {
	tui := NewTUI(Config{})
	tui.buildComponents()
	tui.allCache = []domain.ProviderUsage{
		{AccountID: "a1", Provider: "glm", Label: "one"},
		{AccountID: "a2", Provider: "glm", Label: "two", Pinned: true},
		{AccountID: "a3", Provider: "glm", Label: "three"},
	}

	got := tui.visibleSorted()
	if len(got) != 3 {
		t.Fatalf("want 3 items, got %d", len(got))
	}
	if got[0].AccountID != "a2" {
		t.Errorf("pinned a2 should sort first, got %q", got[0].AccountID)
	}
	// 未置顶项保持原配置顺序（稳定排序）
	if got[1].AccountID != "a1" || got[2].AccountID != "a3" {
		t.Errorf("unpinned should keep original order a1,a3; got %q,%q", got[1].AccountID, got[2].AccountID)
	}
}

// TestDoTogglePin_NoQueueDraw 验证 doTogglePin 不经过 queueDraw（QueueUpdateDraw）。
// doTogglePin 在 tview 主循环（input-capture handler）里同步执行；若它走 Render 的
// QueueUpdateDraw 路径，会因主循环被当前 handler 占用而自死锁——QueueUpdate 阻塞等待
// 主循环执行排队回调，但主循环正忙于此 handler 永远回不到消费 updates 的 select，
// 于是按 p 后应用永久卡死。修复契约：主循环里的调用者必须同步刷新视图（tview 会在
// handler 返回后自动重绘），绝不经 QueueUpdateDraw。
func TestDoTogglePin_NoQueueDraw(t *testing.T) {
	tui := NewTUI(Config{
		OnTogglePin: func(string) []domain.ProviderUsage {
			return []domain.ProviderUsage{{AccountID: "a1", Pinned: true, Label: "pinned"}}
		},
	})
	tui.buildComponents()
	queued := false
	tui.queueDraw = func(f func()) { queued = true; f() } // 模拟运行时 queueDraw
	tui.selectedID = "a1"

	tui.doTogglePin()

	if queued {
		t.Fatal("doTogglePin must not route through queueDraw — that deadlocks the tview main loop (freeze on 'p'); update views synchronously instead")
	}
	if len(tui.allCache) != 1 || tui.allCache[0].AccountID != "a1" {
		t.Errorf("doTogglePin should apply dataset synchronously, got allCache=%+v", tui.allCache)
	}
}

// TestVisibleSorted_NoPinned 验证无置顶时顺序不变。
func TestVisibleSorted_NoPinned(t *testing.T) {
	tui := NewTUI(Config{})
	tui.buildComponents()
	tui.allCache = []domain.ProviderUsage{
		{AccountID: "a1", Provider: "glm", Label: "one"},
		{AccountID: "a2", Provider: "glm", Label: "two"},
	}
	got := tui.visibleSorted()
	if got[0].AccountID != "a1" || got[1].AccountID != "a2" {
		t.Errorf("order should be unchanged without pins, got %q,%q", got[0].AccountID, got[1].AccountID)
	}
}
