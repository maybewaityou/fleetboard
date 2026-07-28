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

// Package minimax 实现 ports.UsageProvider，对接 MiniMax Token Plan 用量接口。
//
// 真实接口契约（官网管理平台实测）：
//   - GET {BaseURL}/backend/account/token_plan/remains_percent
//     默认 BaseURL = https://www.minimaxi.com；acc.BaseURL 可覆盖。
//   - 鉴权头：Authorization: Bearer <api_key> —— 【必须带 "Bearer " 前缀】。
//     key 从 os.Getenv(acc.TokenEnv) 读取。
//
// 响应 base_resp.status_code 为业务码（0=成功）；非 0 视为失败（即便 HTTP 200）。
// model_remains[] 每项是一个模型/档位（如 general、video），首项为主档。每个档位含两个计费窗口：
//   - current_interval_*：5 小时滚动窗口（start_time/end_time，Unix 毫秒）
//   - current_weekly_*：周窗口（weekly_start_time/weekly_end_time，Unix 毫秒）
//
// 每个窗口的 *_used_percent 是字符串（如 "9%"），*_status 为窗口状态：3 = ∞ 无限制
// （此时 used_percent 无意义，多为 "0%"，不能当字面百分比展示）。
//
// 主档的两个窗口映射为两个 UsageDimension（仿 GLM 的 5h/Weekly）：
//   - status=3（无限制）→ Unlimited=true、PercentUsed=-1（SelectPrimary 跳过，UI 渲染 ∞）
//   - 否则 → PercentUsed=parsePercent("9%")=9
//
// ResetsAt 取对应窗口的 end_time。最终调 ProviderUsage.SelectPrimary()。
package minimax

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	defaultBaseURL = "https://www.minimaxi.com"
	usagePath      = "/backend/account/token_plan/remains_percent"
	httpTimeout    = 10 * time.Second

	// sourceTag 标记维度数据来源为「接口实时拉取后平衡出的值」。
	sourceTag = "api-balanced"

	unitPercent = "%"

	name5h     = "5h Quota"
	nameWeekly = "Weekly Quota"

	// Order：多档窗口展示优先级（domain.UsageDimension.Order）。5h 是最短期/最值得警惕的窗口，
	// 固定置顶；weekly 次之。与 GLM 一致，UI 据此稳定排序与选出列表展示维度。
	order5h     = 1
	orderWeekly = 2

	// statusUnlimited：MiniMax 窗口状态码 3 表示该窗口 ∞ 无限制（无配额上限）。
	statusUnlimited = 3
)

// 编译期断言：*Provider 实现 ports.UsageProvider。
var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 MiniMax 用量适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 MiniMax Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Provider 返回厂商标识，对应 domain.Account.Provider。
func (p *Provider) Provider() string { return "minimax" }

// apiResp 是 MiniMax token_plan/remains_percent 接口的响应结构。
type apiResp struct {
	ModelRemains []modelRemain `json:"model_remains"`
	BaseResp     baseResp      `json:"base_resp"`
}

// baseResp 是 MiniMax 通用业务状态包装。status_code=0 成功；非 0 为业务错误
// （鉴权失败等，即便 HTTP 200 也可能携带非 0 业务码）。
type baseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// modelRemain 描述单个模型/档位的剩余信息。start_time/end_time 及 weekly_*
// 均为 Unix 毫秒（13 位）。*_used_percent 为字符串百分比（"9%"）；*_status=3 表示无限制。
type modelRemain struct {
	ModelName string `json:"model_name"`

	// 5 小时滚动窗口。
	EndTime                int64  `json:"end_time"`                      // 窗口结束/重置（毫秒）→ ResetsAt
	CurrentIntervalUsedPct string `json:"current_interval_used_percent"` // "9%"
	CurrentIntervalStatus  int    `json:"current_interval_status"`       // 1=有限, 3=∞ 无限制

	// 周窗口。
	WeeklyEndTime        int64  `json:"weekly_end_time"` // 周窗口结束/重置（毫秒）
	CurrentWeeklyUsedPct string `json:"current_weekly_used_percent"`
	CurrentWeeklyStatus  int    `json:"current_weekly_status"`
}

// FetchUsage 拉取该账号当前 Token Plan 用量，返回多维度 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err），便于上层展示局部信息。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "minimax",
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}

	key := os.Getenv(acc.TokenEnv)
	base := acc.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	u.BaseURL = base
	u.Endpoint = usagePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+usagePath, http.NoBody)
	if err != nil {
		u.Err = fmt.Errorf("minimax: build request: %w", err)
		return u, u.Err
	}
	// 鉴权：必须带 "Bearer " 前缀（MiniMax 该接口的易错点，区别于 GLM 的裸 key）。
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("minimax: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	// 状态守卫：MiniMax 错误响应（401/403/500…）常带 JSON 错误体但缺有效百分比，
	// 若不拦截会解码出 0% → 误报「完全未用」。必须在解码前拦截。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("minimax: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("minimax: decode response: %w", err)
		return u, u.Err
	}

	// 业务码守卫：HTTP 200 但 base_resp.status_code != 0（如鉴权失败 1004）视为失败，
	// 避免把错误体里的空 model_remains 当成「无维度」静默吞掉。
	if r.BaseResp.StatusCode != 0 {
		u.Err = fmt.Errorf("minimax: business error %d: %s", r.BaseResp.StatusCode, r.BaseResp.StatusMsg)
		return u, u.Err
	}

	// 主档 model_name → Model（空数组时守卫，避免 [0] 越界 panic）。
	if len(r.ModelRemains) > 0 {
		u.Model = r.ModelRemains[0].ModelName
	}
	u.Dimensions = buildDimensions(r)
	u.SelectPrimary()
	return u, nil
}

// unixTime 解析 MiniMax 时间戳为 time.Time。真实接口的 end_time/weekly_end_time 为
// Unix 毫秒（13 位）；保留对偶发秒级（10 位）的兼容。零值返回零时间，UI 层据此跳过渲染。
func unixTime(ts int64) time.Time {
	if ts == 0 {
		return time.Time{}
	}
	if ts >= 1_000_000_000_000 { // 13 位 = 毫秒
		return time.Unix(ts/1000, 0).UTC()
	}
	return time.Unix(ts, 0).UTC()
}

// parsePercent 解析 MiniMax 字符串百分比（"9%" → 9）。空串或无法解析返回 -1（N/A），
// 让 SelectPrimary 跳过、UI 渲染为灰条，避免把异常值当 0% 误报。
func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	if s == "" {
		return -1
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return f
}

// buildDimensions 把主档（model_remains[0]）的两个计费窗口映射为 UsageDimension。
// 空数组返回 nil（UI 显示「no quota dimensions」）。每个窗口：status=3 → 无限制（∞），
// 否则取 used_percent。5h Order=1 置顶，weekly Order=2。
func buildDimensions(r apiResp) []domain.UsageDimension {
	if len(r.ModelRemains) == 0 {
		return nil
	}
	m := r.ModelRemains[0]
	return []domain.UsageDimension{
		buildWindow(name5h, order5h, m.CurrentIntervalUsedPct, m.CurrentIntervalStatus, m.EndTime),
		buildWindow(nameWeekly, orderWeekly, m.CurrentWeeklyUsedPct, m.CurrentWeeklyStatus, m.WeeklyEndTime),
	}
}

// buildWindow 构造单个计费窗口维度。status=3（无限制）→ Unlimited=true、PercentUsed=-1
// （SelectPrimary 跳过，UI 渲染 ∞）；否则 PercentUsed=parsePercent。ResetsAt 取窗口结束时间。
func buildWindow(name string, order int, usedPct string, status int, endMs int64) domain.UsageDimension {
	d := domain.UsageDimension{
		Name:     name,
		Order:    order,
		Unit:     unitPercent,
		Source:   sourceTag,
		ResetsAt: unixTime(endMs),
	}
	if status == statusUnlimited {
		d.Unlimited = true
		d.PercentUsed = -1
	} else {
		d.PercentUsed = parsePercent(usedPct)
	}
	return d
}
