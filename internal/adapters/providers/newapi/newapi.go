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

// Package newapi 实现 ports.UsageProvider，对接 new-api（one-api fork）中转平台余额接口。
//
// 真实接口契约（OpenAI 兼容 billing，QuantumNous/new-api-docs）：
//   - GET {BaseURL}/v1/dashboard/billing/subscription → system_hard_limit_usd（总额，回退 hard_limit_usd）
//   - GET {BaseURL}/v1/dashboard/billing/usage        → total_usage（已用，美元）
//   - 鉴权 Authorization: Bearer <sk-api-key>（单凭证）
//
// 余额 = 总额 - 已用。usage 端点失败时降级 used=0（某些版本已弃用该端点）。
// new-api 自部署，BaseURL 必填。
package newapi

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
	subscriptionPath = "/v1/dashboard/billing/subscription"
	usagePath        = "/v1/dashboard/billing/usage"
	httpTimeout      = 10 * time.Second

	nameAvailable = "Available balance"
	sourceTag     = "newapi"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 new-api 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 new-api Provider，HTTP 客户端超时 10s。
func New() *Provider { return &Provider{hc: &http.Client{Timeout: httpTimeout}} }

// Provider 返回厂商标识。
func (p *Provider) Provider() string { return "newapi" }

type subscriptionResp struct {
	SystemHardLimitUSD float64 `json:"system_hard_limit_usd"`
	HardLimitUSD       float64 `json:"hard_limit_usd"`
}

type usageResp struct {
	TotalUsage float64 `json:"total_usage"`
}

// FetchUsage 拉取该账号余额：subscription 取总额（必成），usage 取已用（失败降级 0）。
// 余额 = 总额 - 已用。subscription 失败则整体报错（无法取总额）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "newapi",
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}
	if acc.BaseURL == "" {
		u.Err = fmt.Errorf("newapi: base_url is required (self-hosted, no default)")
		return u, u.Err
	}
	key := os.Getenv(acc.TokenEnv)
	u.BaseURL = acc.BaseURL
	u.Endpoint = subscriptionPath

	sub := &subscriptionResp{}
	if err := p.getJSON(ctx, acc.BaseURL+subscriptionPath, key, sub); err != nil {
		u.Err = fmt.Errorf("newapi: subscription: %w", err)
		return u, u.Err
	}
	limit := sub.SystemHardLimitUSD
	if limit == 0 {
		limit = sub.HardLimitUSD // 字段回退
	}

	var used float64
	ur := &usageResp{}
	if err := p.getJSON(ctx, acc.BaseURL+usagePath, key, ur); err == nil {
		used = ur.TotalUsage
	}
	// usage 失败：降级 used=0（端点可能被弃用），余额=总额，不报错。

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     limit - used,
		Currency:    "USD",
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}

// getJSON 发 GET 并解码进 out；非 2xx 或解码失败返回错误。
func (p *Provider) getJSON(ctx context.Context, url, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
