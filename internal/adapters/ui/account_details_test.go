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
