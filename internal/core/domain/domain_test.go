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

func TestSelectPrimaryPicksMaxPercent(t *testing.T) {
	u := &ProviderUsage{Dimensions: []UsageDimension{
		{Name: "5h", PercentUsed: 30},
		{Name: "weekly", PercentUsed: 88},
		{Name: "mcp", PercentUsed: -1},
	}}
	u.SelectPrimary()
	if u.Primary == nil || u.Primary.Name != "weekly" {
		t.Fatalf("want weekly, got %+v", u.Primary)
	}
}

func TestSelectPrimaryAllInvalidReturnsNil(t *testing.T) {
	u := &ProviderUsage{Dimensions: []UsageDimension{
		{Name: "5h", PercentUsed: -1},
		{Name: "monthly", PercentUsed: -1},
	}}
	u.SelectPrimary()
	if u.Primary != nil {
		t.Fatalf("want nil Primary when all dimensions are N/A, got %+v", u.Primary)
	}
}

func TestSelectPrimaryEmptyDimensionsReturnsNil(t *testing.T) {
	u := &ProviderUsage{}
	u.SelectPrimary()
	if u.Primary != nil {
		t.Fatalf("want nil Primary when no dimensions, got %+v", u.Primary)
	}
}
