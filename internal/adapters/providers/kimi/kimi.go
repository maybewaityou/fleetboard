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

// Package kimi 实现 ports.UsageProvider，对接 Moonshot（Kimi）账户余额接口。
//
// 真实接口契约（platform.kimi.com/docs/api/balance）：
//   - GET {BaseURL}/v1/users/me/balance
//     默认 BaseURL = https://api.moonshot.cn（CNY）；国际版可覆盖为 https://api.moonshot.ai（USD）。
//   - 鉴权头：Authorization: Bearer <MOONSHOT_API_KEY> —— 【必须带 "Bearer " 前缀】。
//
// 成功响应 {code, data:{available_balance, voucher_balance, cash_balance}, scode, status}；
// 错误响应为 OpenAI 风格 {error:{message,type,code}}，结构不同，故先以 HTTP 状态守卫拦截，
// 再判 code==0。余额型：仅展示 available_balance，PercentUsed=-1，Currency 按 base 推断。
package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	defaultBaseURL = "https://api.moonshot.cn"
	usagePath      = "/v1/users/me/balance"
	httpTimeout    = 10 * time.Second

	sourceTag     = "api-balanced"
	nameAvailable = "可用余额"

	codeOK      = 0
	currencyCNY = "CNY"
	currencyUSD = "USD"
)

// 编译期断言：*Provider 实现 ports.UsageProvider。
var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 Kimi 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 Kimi Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Provider 返回厂商标识。
func (p *Provider) Provider() string { return "kimi" }

// apiResp 是 Moonshot 余额接口的成功响应结构。
type apiResp struct {
	Code int `json:"code"`
	Data struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
	Status bool `json:"status"`
}

// currencyFor 按 base URL 推断货币：moonshot.ai → USD，其余 → CNY。
// 本地 httptest server（127.0.0.1）走 CNY 默认分支。
func currencyFor(base string) string {
	if strings.Contains(base, "moonshot.ai") {
		return currencyUSD
	}
	return currencyCNY
}

// FetchUsage 拉取该账号余额，返回单维度余额型 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:    "kimi",
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
		u.Err = fmt.Errorf("kimi: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("kimi: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	// 状态守卫：错误响应体结构与成功态不同，先按 HTTP 状态拦截，避免误解码。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("kimi: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("kimi: decode response: %w", err)
		return u, u.Err
	}
	// 业务码守卫：不只看 HTTP 200，code!=0 视为失败。
	if r.Code != codeOK {
		u.Err = fmt.Errorf("kimi: non-zero code %d", r.Code)
		return u, u.Err
	}

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     r.Data.AvailableBalance,
		Currency:    currencyFor(base),
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0] // 余额型：Primary 指向余额维度（不调 SelectPrimary）
	return u, nil
}
