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
// 真实接口契约（社区实现 KonataAPI 等确认）：GET {BaseURL}/v1/usage，
// 鉴权 Authorization: Bearer <sk-api-key>。响应含余额（USD，可为负）。
// sub2api 为自部署平台，BaseURL 必填（无官方默认）。
//
// 注：/v1/usage 的精确字段社区文档未完全公开，此处按 {balance, used} 假设；
// 若真实实例字段名不同，仅需调整 apiResp 的 json tag。
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

// apiResp 是 /v1/usage 响应结构（字段名按社区实现假设）。
type apiResp struct {
	Balance float64 `json:"balance"`
	Used    float64 `json:"used"`
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

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     r.Balance,
		Currency:    "USD",
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}
