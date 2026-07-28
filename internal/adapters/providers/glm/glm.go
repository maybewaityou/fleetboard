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

// Package glm 实现 ports.UsageProvider，对接智谱（Zhipu / GLM）Coding Plan 用量接口。
//
// 真实接口契约（cc-switch #1588）：
//   - GET {BaseURL}/api/monitor/usage/quota/limit
//     默认 BaseURL = https://open.bigmodel.cn；国际版可用 BaseURL 覆盖为 https://api.z.ai
//   - 鉴权头：Authorization: <API_TOKEN> —— 直接传裸 key，【不要】加 "Bearer " 前缀。
//     这是该接口最易错点。
//   - 另加 Content-Type: application/json；无请求参数。
//
// 响应中的 data.limits 数组被映射为多维度 domain.UsageDimension：
//   - TOKENS_LIMIT：按 nextResetTime 升序排列，第 1 个="5小时额度"、第 2 个="每周额度"；
//     PercentUsed=percentage，Unit="%"。
//   - TIME_LIMIT：Name="MCP每月"，Used=currentValue，Limit=usage，Remaining=remaining，
//     Unit="次"，PercentUsed=percentage。
//
// 构造完所有维度后调用 ProviderUsage.SelectPrimary()，取 PercentUsed 最大那档作为 Primary。
package glm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	defaultBaseURL = "https://open.bigmodel.cn"
	usagePath      = "/api/monitor/usage/quota/limit"
	httpTimeout    = 10 * time.Second

	// sourceTag 标记维度数据来源为「接口实时拉取后平衡出的值」。
	sourceTag = "api-balanced"

	limitTypeTokens = "TOKENS_LIMIT"
	limitTypeTime   = "TIME_LIMIT"

	unitPercent = "%"
	unitCount   = "uses"

	nameTokens5h     = "5h Quota"
	nameTokensWeekly = "Weekly Quota"
	nameTimeMonthly  = "MCP Monthly"

	// Order：多档配额的展示优先级（domain.UsageDimension.Order）。5h 是最短期/最值得警惕的
	// 窗口，固定置顶；weekly 次之；MCP 每月最后。即便 GLM 偶发不返回 5h 的 nextResetTime，
	// UI 仍能凭 Order 把 5h 稳定排到列表展示位与详情顶部。
	order5h      = 1
	orderWeekly  = 2
	orderMonthly = 3

	respCodeOK = 200
)

// 编译期断言：*Provider 实现 ports.UsageProvider。
var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 GLM 用量适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 GLM Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Provider 返回厂商标识，对应 domain.Account.Provider。
func (p *Provider) Provider() string { return "glm" }

// apiResp 是 GLM 配额接口的响应结构（仅解码需要的字段）。
type apiResp struct {
	Code int `json:"code"`
	Data struct {
		Level  string     `json:"level"`
		Limits []apiLimit `json:"limits"`
	} `json:"data"`
}

// apiLimit 是 limits 数组中的单条额度。
type apiLimit struct {
	Type          string      `json:"type"`
	Percentage    int         `json:"percentage"`
	Usage         int64       `json:"usage"`
	CurrentValue  int64       `json:"currentValue"`
	Remaining     int64       `json:"remaining"`
	NextResetTime json.Number `json:"nextResetTime"`
}

// FetchUsage 拉取该账号当前用量，返回多维度 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err），便于上层展示局部信息。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "glm",
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
		u.Err = fmt.Errorf("glm: build request: %w", err)
		return u, u.Err
	}
	// 鉴权：裸 key，不加 "Bearer " 前缀（GLM 该接口的易错点）。
	req.Header.Set("Authorization", key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("glm: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("glm: decode response: %w", err)
		return u, u.Err
	}
	if r.Code != respCodeOK {
		u.Err = fmt.Errorf("glm: non-200 response code %d", r.Code)
		return u, u.Err
	}

	u.PlanLevel = r.Data.Level
	u.Dimensions = buildDimensions(r.Data.Limits)
	u.SelectPrimary()
	return u, nil
}

// buildDimensions 把 GLM limits 数组映射为多维度 UsageDimension。
//   - TIME_LIMIT → "MCP每月"（含 Used/Limit/Remaining，Unit="次"）
//   - TOKENS_LIMIT → 按 nextResetTime 升序后按位置命名（"5小时额度"/"每周额度"，Unit="%"）
func buildDimensions(limits []apiLimit) []domain.UsageDimension {
	var tokens []apiLimit
	dims := make([]domain.UsageDimension, 0, len(limits))

	for _, l := range limits {
		switch l.Type {
		case limitTypeTime:
			dims = append(dims, domain.UsageDimension{
				Name:        nameTimeMonthly,
				Used:        l.CurrentValue,
				Limit:       l.Usage,
				Remaining:   l.Remaining,
				PercentUsed: float64(l.Percentage),
				ResetsAt:    parseResetTime(l.NextResetTime),
				Unit:        unitCount,
				Source:      sourceTag,
				Order:       orderMonthly,
			})
		case limitTypeTokens:
			tokens = append(tokens, l)
		}
	}

	// TOKENS_LIMIT 按 nextResetTime 升序（数字时间戳，用解析后的 time 比较，兼容秒/毫秒）。
	// 升序后位置 0=5h、位置 1=weekly；零重置时间（time.Time{}）早于一切，故即便 5h 缺时间
	// 仍稳定落在位置 0，命名与 Order 赋值都不受影响。
	sort.Slice(tokens, func(i, j int) bool {
		return parseResetTime(tokens[i].NextResetTime).Before(parseResetTime(tokens[j].NextResetTime))
	})
	tokenNames := []string{nameTokens5h, nameTokensWeekly}
	tokenOrders := []int{order5h, orderWeekly}
	for i, l := range tokens {
		name := fmt.Sprintf("Quota #%d", i+1)
		order := i + orderWeekly // 超出预定义档位时按位置递增，保证仍有确定序
		if i < len(tokenNames) {
			name = tokenNames[i]
		}
		if i < len(tokenOrders) {
			order = tokenOrders[i]
		}
		dims = append(dims, domain.UsageDimension{
			Name:        name,
			PercentUsed: float64(l.Percentage),
			ResetsAt:    parseResetTime(l.NextResetTime),
			Unit:        unitPercent,
			Source:      sourceTag,
			Order:       order,
		})
	}
	return dims
}

// parseResetTime 解析 nextResetTime（真实 API 返回的 Unix 秒或毫秒 number）为 time.Time。
// ≥1e12（13 位）视为毫秒，否则视为秒。空值或解析失败返回零值（UI 层会跳过零 ResetsAt）。
func parseResetTime(n json.Number) time.Time {
	if n == "" {
		return time.Time{}
	}
	v, err := n.Int64()
	if err != nil {
		return time.Time{}
	}
	if v >= 1_000_000_000_000 { // 13 位 = 毫秒
		return time.Unix(v/1000, 0)
	}
	return time.Unix(v, 0)
}
