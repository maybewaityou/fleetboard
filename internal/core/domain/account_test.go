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

package domain

import "testing"

// TestGenerateAccountID 验证 id 是 12 位 hex 且连续生成不碰撞。
func TestGenerateAccountID(t *testing.T) {
	id := GenerateAccountID()
	if len(id) != 12 {
		t.Fatalf("len(id) = %d, want 12; id=%q", len(id), id)
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("non-hex char %q in id %q", c, id)
		}
	}
	// 唯一性：6 字节空间 ~281 万亿，1000 次碰撞概率可忽略。
	seen := map[string]bool{id: true}
	for i := 0; i < 1000; i++ {
		id2 := GenerateAccountID()
		if seen[id2] {
			t.Fatalf("collision: %q generated twice", id2)
		}
		seen[id2] = true
	}
}
