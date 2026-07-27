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

// TestUsageDimensionBalanceFields 验证余额型字段可读写，且余额型维度（PercentUsed=-1）
// 在 SelectPrimary 中被跳过（配额型行为不受影响）。
func TestUsageDimensionBalanceFields(t *testing.T) {
	dim := UsageDimension{
		Name:        "可用余额",
		Balance:     49.58,
		Currency:    "CNY",
		PercentUsed: -1,
	}
	if dim.Balance != 49.58 {
		t.Errorf("Balance = %v, want 49.58", dim.Balance)
	}
	if dim.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", dim.Currency)
	}

	// SelectPrimary 跳过 PercentUsed<0 的维度：纯余额型维度集合 → Primary 为 nil
	u := VendorUsage{Dimensions: []UsageDimension{dim}}
	u.SelectPrimary()
	if u.Primary != nil {
		t.Errorf("SelectPrimary should skip PercentUsed<0 balance dim, got Primary=%+v", u.Primary)
	}
}
