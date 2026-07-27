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

// Package newapi 实现 ports.UsageProvider，对接 new-api（one-api fork）中转平台原生管理层。
//
// 真实接口契约（QuantumNous/new-api，实测 kuaipao.pro）：
//   - GET {BaseURL}/api/user/self                         → data.quota（剩余，内部单位）
//   - GET {BaseURL}/api/status                            → data.quota_per_unit（换算因子）
//   - GET {BaseURL}/api/log/self/stat?start&end           → data.quota（区间消耗）+ rpm/tpm
//   - 鉴权 Authorization: Bearer <access_token> + New-Api-User: <user_id>（双 header）
//
// 余额 = quota / quota_per_unit（美元）。quota_per_unit 失败回退 500000。
// stat 失败时 Recent=nil（消耗为次要信息，余额不受影响）。
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
	userSelfPath = "/api/user/self"
	statusPath   = "/api/status"
	logStatPath  = "/api/log/self/stat"

	defaultQPU  = 500000 // quota_per_unit 回退默认
	window7d    = 7 * 24 * time.Hour
	window30d   = 30 * 24 * time.Hour
	httpTimeout = 10 * time.Second

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

type userSelfResp struct {
	Data struct {
		Quota int64 `json:"quota"`
	} `json:"data"`
}

type statusResp struct {
	Data struct {
		QuotaPerUnit float64 `json:"quota_per_unit"`
	} `json:"data"`
}

type statResp struct {
	Data struct {
		Quota int64 `json:"quota"`
		RPM   int   `json:"rpm"`
		TPM   int   `json:"tpm"`
	} `json:"data"`
}

// FetchUsage 拉取该账号真实余额与近窗口消耗。
// user/self 为核心（失败报错）；status 决定换算因子（失败回退）；stat 为辅助（失败 Recent=nil）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "newapi",
		Label:     acc.Label,
		FetchedAt: time.Now(),
		BaseURL:   acc.BaseURL,
		Endpoint:  userSelfPath,
	}
	if acc.BaseURL == "" {
		u.Err = fmt.Errorf("newapi: base_url is required (self-hosted, no default)")
		return u, u.Err
	}
	if acc.AccessTokenEnv == "" || acc.UserID == "" {
		u.Err = fmt.Errorf("newapi: access_token_env and user_id are required")
		return u, u.Err
	}
	accessToken := os.Getenv(acc.AccessTokenEnv)
	if accessToken == "" {
		u.Err = fmt.Errorf("newapi: access token not set in env %q", acc.AccessTokenEnv)
		return u, u.Err
	}

	// 1) user/self — 余额（核心，失败整体报错）。
	us := &userSelfResp{}
	if err := p.getJSON(ctx, acc.BaseURL+userSelfPath, accessToken, acc.UserID, us); err != nil {
		u.Err = fmt.Errorf("newapi: user/self: %w", err)
		return u, u.Err
	}

	// 2) status — 换算因子（失败回退 defaultQPU）。
	qpu := float64(defaultQPU)
	if st, err := p.getStatus(ctx, acc.BaseURL, accessToken, acc.UserID); err == nil && st > 0 {
		qpu = st
	}
	usd := func(q int64) float64 { return float64(q) / qpu }

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     usd(us.Data.Quota),
		Currency:    "USD",
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]

	// 3) stat — 近窗口消耗（辅助，失败 Recent=nil）。
	now := time.Now()
	statURL := func(window time.Duration) string {
		return fmt.Sprintf("%s%s?start_timestamp=%d&end_timestamp=%d",
			acc.BaseURL, logStatPath, now.Add(-window).Unix(), now.Unix())
	}
	s7, err7 := p.getStat(ctx, statURL(window7d), accessToken, acc.UserID)
	s30, err30 := p.getStat(ctx, statURL(window30d), accessToken, acc.UserID)
	if err7 == nil && err30 == nil {
		u.Recent = &domain.RecentUsage{
			Window7d:  usd(s7.Data.Quota),
			Window30d: usd(s30.Data.Quota),
			RPM:       s7.Data.RPM,
			TPM:       s7.Data.TPM,
			Currency:  "USD",
		}
	}
	return u, nil
}

// getStatus 取 quota_per_unit；失败返回 0（调用方回退默认）。
func (p *Provider) getStatus(ctx context.Context, base, bearer, newUser string) (float64, error) {
	st := &statusResp{}
	if err := p.getJSON(ctx, base+statusPath, bearer, newUser, st); err != nil {
		return 0, err
	}
	return st.Data.QuotaPerUnit, nil
}

// getStat 取区间消耗统计；失败返回 error（调用方据此决定 Recent 取舍）。
func (p *Provider) getStat(ctx context.Context, url, bearer, newUser string) (*statResp, error) {
	s := &statResp{}
	if err := p.getJSON(ctx, url, bearer, newUser, s); err != nil {
		return nil, err
	}
	return s, nil
}

// getJSON 发 GET（带 Bearer + New-Api-User 双 header）并解码进 out；非 2xx 或解码失败返回错误。
func (p *Provider) getJSON(ctx context.Context, url, bearer, newUser string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("New-Api-User", newUser)

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
