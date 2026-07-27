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
)

// TestRenderHelpBodyContainsAllGroups 验证帮助正文含每个分组标题与关键键。
func TestRenderHelpBodyContainsAllGroups(t *testing.T) {
	body := renderHelpBody()
	for _, g := range collectHelpGroups() {
		if !strings.Contains(body, g.name) {
			t.Errorf("help body missing group header %q", g.name)
		}
	}
	for _, key := range []string{"a", "e", "d", "r", "R", "?", "q", "/"} {
		if !strings.Contains(body, key) {
			t.Errorf("help body missing key %q", key)
		}
	}
}

// TestPairHelpGroupsFirstAlone 验证第 0 组独占首行（右空），突出 Navigate。
func TestPairHelpGroupsFirstAlone(t *testing.T) {
	rows := pairHelpGroups(5)
	if len(rows) == 0 || rows[0] != [2]int{0, -1} {
		t.Fatalf("first row should be {0,-1}, got %v", rows)
	}
	// 5 组 → 首行 + 2 配对行(1,2)(3,4)
	if len(rows) != 3 {
		t.Fatalf("want 3 rows for 5 groups, got %d", len(rows))
	}
}
