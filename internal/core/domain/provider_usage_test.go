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
		Name:        "Available balance",
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
	u := ProviderUsage{Dimensions: []UsageDimension{dim}}
	u.SelectPrimary()
	if u.Primary != nil {
		t.Errorf("SelectPrimary should skip PercentUsed<0 balance dim, got Primary=%+v", u.Primary)
	}
}

// TestUsageDimensionSiliconFlowFields 验证 SiliconFlow 余额信息字段可赋值/读取，零值默认。
// 注意：provider_usage_test.go 为 package domain（内部测试），直接用 UsageDimension，无 domain. 前缀。
func TestUsageDimensionSiliconFlowFields(t *testing.T) {
	d := UsageDimension{
		Name: "Available balance", Balance: 0.88, Currency: "CNY",
		ChargeBalance: 88.0, TotalBalance: 88.88,
	}
	if d.ChargeBalance != 88.0 {
		t.Errorf("ChargeBalance = %v, want 88.0", d.ChargeBalance)
	}
	if d.TotalBalance != 88.88 {
		t.Errorf("TotalBalance = %v, want 88.88", d.TotalBalance)
	}
	// 零值默认：未填时为 0（UI 据此跳过渲染）
	zero := UsageDimension{}
	if zero.ChargeBalance != 0 || zero.TotalBalance != 0 {
		t.Errorf("zero-value fields should be 0, got charge=%v total=%v", zero.ChargeBalance, zero.TotalBalance)
	}
}
