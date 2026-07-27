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

// Package deepseek 实现 ports.UsageProvider，对接 DeepSeek 账户余额接口。
//
// 真实接口契约（api-docs.deepseek.com/api/get-user-balance）：
//   - GET {BaseURL}/user/balance，默认 BaseURL = https://api.deepseek.com。
//   - 鉴权头：Authorization: Bearer <API_KEY>。
//
// 响应 {is_available, balance_infos:[{currency, total_balance, granted_balance,
// topped_up_balance}]}。金额字段全是 string（需 strconv.ParseFloat，单位即元/美元，
// 无 /100 换算）。total_balance = granted + topped_up，是「剩余」语义，无已用/百分比/重置。
package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	defaultBaseURL = "https://api.deepseek.com"
	usagePath      = "/user/balance"
	httpTimeout    = 10 * time.Second

	sourceTag     = "api-balanced"
	nameAvailable = "Available balance"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 DeepSeek 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 DeepSeek Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Provider 返回厂商标识。
func (p *Provider) Provider() string { return "deepseek" }

// apiResp 是 DeepSeek /user/balance 响应结构。金额字段为 string。
type apiResp struct {
	IsAvailable  bool          `json:"is_available"`
	BalanceInfos []balanceInfo `json:"balance_infos"`
}

// balanceInfo 是单币种余额明细。三个金额字段均为十进制字符串。
type balanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// FetchUsage 拉取该账号余额，返回单维度余额型 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "deepseek",
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
		u.Err = fmt.Errorf("deepseek: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("deepseek: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("deepseek: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("deepseek: decode response: %w", err)
		return u, u.Err
	}
	if len(r.BalanceInfos) == 0 {
		u.Err = fmt.Errorf("deepseek: empty balance_infos")
		return u, u.Err
	}

	info := r.BalanceInfos[0]
	// 防御性默认：API 偶发返回 currency:"" 时，UI 判定（Currency != "" 区分余额/配额型）
	// 会把该维度误判为配额型并渲染 -1%——正是本特性要修的 bug。保留余额数据，币别回退 CNY。
	if info.Currency == "" {
		info.Currency = "CNY"
	}
	total, err := strconv.ParseFloat(info.TotalBalance, 64)
	if err != nil {
		u.Err = fmt.Errorf("deepseek: parse total_balance %q: %w", info.TotalBalance, err)
		return u, u.Err
	}

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     total,
		Currency:    info.Currency,
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}
