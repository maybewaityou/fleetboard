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

// Package sub2api 实现 ports.UsageProvider，对接 sub2api 中转平台的余额接口。
//
// 真实接口契约（Wei-Shaw/sub2api gateway_handler.go）：GET {BaseURL}/v1/usage，
// 鉴权 Authorization: Bearer <sk-api-key>。响应为双模式：
//   - quota_limited：quota（总配额）+ rate_limits（5h/1d/7d 窗口）+ usage + status/expires_at。
//   - unrestricted：钱包余额（balance）或订阅（subscription 日/周/月限额）。
//
// sub2api 为自部署平台，BaseURL 必填（无官方默认）。余额单位 USD，可为负。
package sub2api

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
	usagePath   = "/v1/usage"
	httpTimeout = 10 * time.Second

	nameAvailable = "Available balance"
	sourceTag     = "sub2api"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 sub2api 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 sub2api Provider，HTTP 客户端超时 10s。
func New() *Provider { return &Provider{hc: &http.Client{Timeout: httpTimeout}} }

// Provider 返回厂商标识。
func (p *Provider) Provider() string { return "sub2api" }

// apiResp 是 /v1/usage 响应结构（Wei-Shaw/sub2api gateway_handler.go 双模式契约）。
type apiResp struct {
	Mode            string            `json:"mode"` // quota_limited | unrestricted
	IsValid         bool              `json:"isValid"`
	Status          string            `json:"status"`
	PlanName        string            `json:"planName"`
	Remaining       float64           `json:"remaining"`
	Unit            string            `json:"unit"`
	Balance         float64           `json:"balance"` // 钱包模式
	Quota           *quotaResp        `json:"quota"`   // 配额模式
	RateLimits      []rateLimitResp   `json:"rate_limits"`
	Subscription    *subscriptionResp `json:"subscription"` // 订阅模式
	Usage           *usageResp        `json:"usage"`
	ExpiresAt       *time.Time        `json:"expires_at"`
	DaysUntilExpiry *int              `json:"days_until_expiry"`
}

type quotaResp struct {
	Limit     float64 `json:"limit"`
	Used      float64 `json:"used"`
	Remaining float64 `json:"remaining"`
	Unit      string  `json:"unit"`
}

type rateLimitResp struct {
	Window    string     `json:"window"`
	Limit     float64    `json:"limit"`
	Used      float64    `json:"used"`
	Remaining float64    `json:"remaining"`
	ResetAt   *time.Time `json:"reset_at"`
}

type subscriptionResp struct {
	DailyUsageUSD     float64    `json:"daily_usage_usd"`
	WeeklyUsageUSD    float64    `json:"weekly_usage_usd"`
	MonthlyUsageUSD   float64    `json:"monthly_usage_usd"`
	DailyLimitUSD     *float64   `json:"daily_limit_usd"`
	WeeklyLimitUSD    *float64   `json:"weekly_limit_usd"`
	MonthlyLimitUSD   *float64   `json:"monthly_limit_usd"`
	WeeklyWindowStart *time.Time `json:"weekly_window_start"`
	ExpiresAt         *time.Time `json:"expires_at"`
}

type usageResp struct {
	Today             usageBucket `json:"today"`
	Total             usageBucket `json:"total"`
	AverageDurationMs int64       `json:"average_duration_ms"`
	Rpm               int         `json:"rpm"`
	Tpm               int         `json:"tpm"`
}

type usageBucket struct {
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`
}

// FetchUsage 拉取该账号余额，返回单维度余额型 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "sub2api",
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}

	if acc.BaseURL == "" {
		u.Err = fmt.Errorf("sub2api: base_url is required (self-hosted, no default)")
		return u, u.Err
	}
	key := os.Getenv(acc.TokenEnv)
	u.BaseURL = acc.BaseURL
	u.Endpoint = usagePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, acc.BaseURL+usagePath, http.NoBody)
	if err != nil {
		u.Err = fmt.Errorf("sub2api: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("sub2api: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("sub2api: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("sub2api: decode response: %w", err)
		return u, u.Err
	}

	const usd = "USD"
	// 1) Primary 余额维度（Dimensions[0]）：归一三种模式的"剩余"。
	primary := domain.UsageDimension{
		Name:        nameAvailable,
		Currency:    usd,
		PercentUsed: -1,
		Source:      sourceTag,
	}
	switch {
	case r.Mode == "quota_limited" && r.Quota != nil:
		primary.Balance = r.Quota.Remaining
	case r.Mode == "unrestricted" && r.Subscription != nil:
		primary.Balance = r.Remaining
	default: // 钱包余额模式（或未知模式兜底取 balance）
		primary.Balance = r.Balance
	}
	dims := []domain.UsageDimension{primary}

	// 2) 金额配额维度：配额模式追加 Total quota + 各 rate_limit 窗口。
	if r.Mode == "quota_limited" && r.Quota != nil && r.Quota.Limit > 0 {
		dims = append(dims, moneyQuotaDim("Total quota", r.Quota.Limit, r.Quota.Used, r.Quota.Remaining, usd, time.Time{}))
		for _, rl := range r.RateLimits {
			var reset time.Time
			if rl.ResetAt != nil {
				reset = *rl.ResetAt
			}
			dims = append(dims, moneyQuotaDim(rl.Window+" window", rl.Limit, rl.Used, rl.Remaining, usd, reset))
		}
	}
	// 订阅模式追加日/周/月限额维度。
	if r.Mode == "unrestricted" && r.Subscription != nil {
		s := r.Subscription
		if s.DailyLimitUSD != nil && *s.DailyLimitUSD > 0 {
			dims = append(dims, moneyQuotaDim("Daily limit", *s.DailyLimitUSD, s.DailyUsageUSD, *s.DailyLimitUSD-s.DailyUsageUSD, usd, time.Time{}))
		}
		if s.WeeklyLimitUSD != nil && *s.WeeklyLimitUSD > 0 {
			var reset time.Time
			if s.WeeklyWindowStart != nil {
				reset = *s.WeeklyWindowStart
			}
			dims = append(dims, moneyQuotaDim("Weekly limit", *s.WeeklyLimitUSD, s.WeeklyUsageUSD, *s.WeeklyLimitUSD-s.WeeklyUsageUSD, usd, reset))
		}
		if s.MonthlyLimitUSD != nil && *s.MonthlyLimitUSD > 0 {
			dims = append(dims, moneyQuotaDim("Monthly limit", *s.MonthlyLimitUSD, s.MonthlyUsageUSD, *s.MonthlyLimitUSD-s.MonthlyUsageUSD, usd, time.Time{}))
		}
	}

	u.Provider = "sub2api"
	u.Dimensions = dims
	u.Primary = &u.Dimensions[0]
	u.PlanLevel = r.PlanName
	u.APIKeyStatus = r.Status
	// expires_at：quota_limited 在顶层（API-key 过期）；订阅模式在 subscription.expires_at（顶层无此字段）。
	switch {
	case r.ExpiresAt != nil:
		u.ExpiresAt = r.ExpiresAt
	case r.Subscription != nil && r.Subscription.ExpiresAt != nil:
		u.ExpiresAt = r.Subscription.ExpiresAt
	}
	// days_until_expiry 仅 quota_limited 顶层存在，订阅模式不填。
	if r.DaysUntilExpiry != nil {
		u.DaysUntilExpiry = *r.DaysUntilExpiry
	}
	if r.Usage != nil {
		u.Recent = &domain.RecentUsage{
			TodayCost:     r.Usage.Today.Cost,
			TotalCost:     r.Usage.Total.Cost,
			TodayTokens:   r.Usage.Today.TotalTokens,
			TotalTokens:   r.Usage.Total.TotalTokens,
			TodayRequests: r.Usage.Today.Requests,
			TotalRequests: r.Usage.Total.Requests,
			RPM:           r.Usage.Rpm,
			TPM:           r.Usage.Tpm,
			AvgDurationMs: r.Usage.AverageDurationMs,
			Currency:      usd,
		}
	}
	return u, nil
}

// moneyQuotaDim 构造一个金额配额维度（sub2api rate_limits / 订阅周期）。reset 零值=无重置时间。
func moneyQuotaDim(name string, limit, used, remaining float64, currency string, reset time.Time) domain.UsageDimension {
	pct := -1.0
	if limit > 0 {
		pct = used / limit * 100
	}
	return domain.UsageDimension{
		Name:        name,
		MoneyLimit:  limit,
		MoneyUsed:   used,
		Balance:     remaining,
		Currency:    currency,
		PercentUsed: pct,
		ResetsAt:    reset,
		Source:      sourceTag,
	}
}
