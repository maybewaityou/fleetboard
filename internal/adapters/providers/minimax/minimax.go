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
// 真实接口契约（调研所得）：
//   - GET {BaseURL}/v1/token_plan/remains
//     默认 BaseURL = https://api.minimaxi.com；国际版可覆盖为 https://api.minimax.io；
//     acc.BaseURL 可覆盖。
//   - 鉴权头：Authorization: Bearer <token_plan_key> —— 【必须带 "Bearer " 前缀】，
//     这是该接口最易错点（与 GLM 裸 key 不同）。key 从 os.Getenv(acc.TokenEnv) 读取。
//
// 响应字段 usage_percent（或驼峰 usagePercent 变体）表示【已用】比例（字段名 usage=已用，
// 实测 unused=0），故 PercentUsed = usagePercent 直接用，不反转。model_remains[] 含
// start_time/end_time（Unix 毫秒）描述当前计费窗口，取首项 end_time 作 ResetsAt。
//
// 最终映射为单维度 UsageDimension{Name:"Token Plan",
// PercentUsed: usagePercent, Unit:"%", ResetsAt: end_time, Source:"api-balanced"}，
// 并调 ProviderUsage.SelectPrimary()。
package minimax

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	defaultBaseURL = "https://api.minimaxi.com"
	usagePath      = "/v1/token_plan/remains"
	httpTimeout    = 10 * time.Second

	// sourceTag 标记维度数据来源为「接口实时拉取后平衡出的值」。
	sourceTag = "api-balanced"

	nameTokenPlan = "Token Plan"
	unitPercent   = "%"
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

// apiResp 是 MiniMax token_plan/remains 接口的响应结构。
//
// usage_percent 在真实 API 里存在 snake_case 与 camelCase 两种变体，
// 故同时声明两个字段，解码后用 usagePercent() 取非零那个（二者不会同时非零）。
type apiResp struct {
	UsagePercent      int           `json:"usage_percent"`
	UsagePercentCamel int           `json:"usagePercent"`
	ModelRemains      []modelRemain `json:"model_remains"`
}

// usagePercent 返回已用比例（0-100，字段名 usage=已用）。优先取 camelCase 变体；
// 两者都为 0 时视为「0% 已用 = 完全未用」。
//
// 假设（经观察固化）：真实 API 不会同一次响应里同时输出 snake_case 与 camelCase 两个键。
// 故「camelCase==0 且 snake_case>0」只会出现在「API 只回了 snake_case」这一种情形，
// 此时 fallthrough 取 snake_case 是正确的；不会与「camelCase 真为 0」混淆。
func (r apiResp) usagePercent() int {
	if r.UsagePercentCamel != 0 {
		return r.UsagePercentCamel
	}
	return r.UsagePercent
}

// modelRemain 描述单个模型/计费窗口的剩余信息。
// start_time / end_time 均为 Unix 秒级时间戳。
type modelRemain struct {
	Model     string `json:"model"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
}

// FetchUsage 拉取该账号当前 Token Plan 用量，返回单维度 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err），便于上层展示局部信息。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:    "minimax",
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+usagePath, nil)
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

	// 状态守卫：MiniMax 错误响应（401/403/500…）常带 JSON 错误体但缺 usage_percent，
	// 若不拦截会解码出 usagePercent()==0 → 误报 "100% 已耗尽"。必须在解码前拦截。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("minimax: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("minimax: decode response: %w", err)
		return u, u.Err
	}

	if len(r.ModelRemains) > 0 {
		u.Model = r.ModelRemains[0].Model
	}
	u.Dimensions = []domain.UsageDimension{buildDimension(r)}
	u.SelectPrimary()
	return u, nil
}

// unixTime 解析 MiniMax 时间戳为 time.Time。真实接口的 start_time/end_time 为
// Unix 毫秒（13 位）；保留对偶发秒级（10 位）的兼容，与 glm.parseResetTime 同策略。
// 零值返回零时间，UI 层据此跳过渲染。
func unixTime(ts int64) time.Time {
	if ts == 0 {
		return time.Time{}
	}
	if ts >= 1_000_000_000_000 { // 13 位 = 毫秒
		return time.Unix(ts/1000, 0).UTC()
	}
	return time.Unix(ts, 0).UTC()
}

// buildDimension 把 MiniMax 响应映射为单维度 UsageDimension。
//   - usage_percent 是【已用】比例（字段名 usage=已用）→ PercentUsed = usagePercent（直接用，不反转）
//   - ResetsAt 取 model_remains[0].end_time（Unix 毫秒）；空数组则零值（UI 层会跳过）
func buildDimension(r apiResp) domain.UsageDimension {
	d := domain.UsageDimension{
		Name:        nameTokenPlan,
		PercentUsed: float64(r.usagePercent()),
		Unit:        unitPercent,
		Source:      sourceTag,
	}
	if len(r.ModelRemains) > 0 {
		d.ResetsAt = unixTime(r.ModelRemains[0].EndTime)
	}
	return d
}
