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

// ProviderUsage 是一次拉取的结果。一个账号可有多个额度维度。
// Basic Info 字段（BaseURL/Endpoint/PlanLevel/Model）由 adapter 从响应/请求中填充，
// 供 details 页面展示账号基本信息；不同 provider 填不同子集（零值=无）。
type ProviderUsage struct {
	AccountID  string
	Provider   string
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

	// 近窗口消耗摘要（adapter 填充，UI 读取）。nil = 该 provider 无此数据。
	Recent *RecentUsage

	// sub2api API Key 状态与过期（其他 provider 零值/nil=无）。
	APIKeyStatus    string
	ExpiresAt       *time.Time
	DaysUntilExpiry int

	// 账号可用状态（adapter 填充，UI 读取）。DeepSeek 由 is_available 映射：
	// true→"active"，false→"insufficient"。其他 provider 零值=无，UI 不渲染。
	// 与 APIKeyStatus（sub2api 的 key active/expired）语义不同，故独立成字段。
	Status string
}

// RecentUsage 是近窗口消耗摘要（余额型 provider 的补充信息）。
// nil 表示该 provider 无此数据（UI 不渲染 Recent 区块）；零值结构体表示"拉到了但全是 0"。
type RecentUsage struct {
	Window7d  float64 // 近7天消耗（美元）
	Window30d float64 // 近30天消耗（美元）
	RPM       int     // 实时每分钟请求数
	TPM       int     // 实时每分钟 token 数

	// 今日/累计统计（sub2api usage.today/total 填充；其他 provider 零值=无）。
	TodayCost     float64
	TotalCost     float64
	TodayTokens   int64
	TotalTokens   int64
	TodayRequests int64
	TotalRequests int64
	AvgDurationMs int64

	Currency string // "USD"
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

	// 余额型 provider 专用（Kimi/DeepSeek）：Balance 是当前余额（元/美元），
	// Currency 为 "CNY"/"USD"。配额型两者均零值。判断余额型用 Currency != ""。
	Balance  float64
	Currency string

	// 余额细分（余额型 provider 可选）：Granted=赠送/赠券部分，ToppedUp=充值/现金部分。
	// DeepSeek 填 granted_balance/topped_up_balance；Kimi 填 voucher_balance/cash_balance。
	// 配额型与其他余额型 provider 零值=无，UI 不渲染。语义约定 Granted+ToppedUp==Balance。
	Granted  float64
	ToppedUp float64

		// SiliconFlow 余额信息（adapter 填充，UI 读取）。零值=无，UI 不渲染。
		// 与 Granted/ToppedUp（剩余拆分，相加=Balance）语义不同：这里是 API 原值，
		// 不做相加约定——官方未保证 chargeBalance/totalBalance 与 balance 的恒等关系。
		// 仅 siliconflow provider 填充；配额型与其他余额型 provider 零值=无。
		ChargeBalance float64
		TotalBalance  float64

	// 金额型配额窗口（USD）：sub2api 的 rate_limits 与订阅日/周/月限额。非零时 renderDimension
	// 走金额配额分支（显示 $used/$limit + 进度条）；token 型 provider 不填，零值跳过。
	// 金额剩余复用 Balance 字段（Balance = MoneyLimit - MoneyUsed）。
	MoneyLimit float64
	MoneyUsed  float64

	// Order 是该维度在 provider 原生多档配额中的展示优先级（1=最优先置顶，越大越靠后）。
	// 仅多档配额型 adapter 填充：GLM 的 5h=1、weekly=2、MCP每月=3。零值=未设置，UI 据此
	// 在维度缺失 ResetsAt 时仍能稳定选出/置顶最短期窗口（GLM 偶发不返回 5h 的 nextResetTime，
	// 若靠重置时间排序会把 5h 误排到 weekly/MCP 之后）。零值维度保持既有「最近重置」逻辑。
	Order int
}

// SelectPrimary 把 PercentUsed 最大的有效维度设为 Primary（最值得警惕的一档）。
// PercentUsed < 0 的维度视为无效（N/A）会被跳过；若全部无效则 Primary 为 nil。
func (u *ProviderUsage) SelectPrimary() {
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
