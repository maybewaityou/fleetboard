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

import "time"

type ResetPolicy string

const (
	ResetRolling5h ResetPolicy = "rolling5h"
	ResetDaily     ResetPolicy = "daily"
	ResetMonthly   ResetPolicy = "monthly"
	ResetCustom    ResetPolicy = "custom"
)

// VendorUsage 是一次拉取的结果。一个账号可有多个额度维度。
type VendorUsage struct {
	AccountID  string
	Vendor     string
	Label      string
	Dimensions []UsageDimension
	Primary    *UsageDimension
	FetchedAt  time.Time
	Err        error
}

// UsageDimension 是单个额度维度（一个窗口/一档配额）。
type UsageDimension struct {
	Name        string
	Used        int64
	Limit       int64
	PercentUsed float64 // -1 = N/A
	Remaining   int64
	ResetsAt    time.Time
	Unit        string
	Source      string
}

// SelectPrimary 把 PercentUsed 最大的有效维度设为 Primary（最值得警惕的一档）。
// PercentUsed < 0 的维度视为无效（N/A）会被跳过；若全部无效则 Primary 为 nil。
func (u *VendorUsage) SelectPrimary() {
	var best *UsageDimension
	for i := range u.Dimensions {
		d := &u.Dimensions[i]
		if d.PercentUsed < 0 {
			continue
		}
		if best == nil || d.PercentUsed > best.PercentUsed {
			best = d
		}
	}
	u.Primary = best
}
