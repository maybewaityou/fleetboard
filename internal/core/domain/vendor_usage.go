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
// Basic Info 字段（BaseURL/Endpoint/PlanLevel/Model）由 adapter 从响应/请求中填充，
// 供 details 页面展示账号基本信息；不同 vendor 填不同子集（零值=无）。
type VendorUsage struct {
	AccountID  string
	Vendor     string
	Label      string
	Dimensions []UsageDimension
	Primary    *UsageDimension
	FetchedAt  time.Time
	Err        error

	// Pinned 来自 Account.Pinned（aggregator 注入）；UI 据此置顶排序并显示 📌。
	Pinned bool

	// Basic Info（adapter 填充，UI 读取）。
	BaseURL   string // 实际请求用的 base（acc.BaseURL 或默认）
	Endpoint  string // 刷新接口路径
	PlanLevel string // 套餐等级：GLM data.level；MiniMax 无
	Model     string // 模型：MiniMax model；GLM 无
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

	// 余额型 vendor 专用（Kimi/DeepSeek）：Balance 是当前余额（元/美元），
	// Currency 为 "CNY"/"USD"。配额型两者均零值。判断余额型用 Currency != ""。
	Balance  float64
	Currency string
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
