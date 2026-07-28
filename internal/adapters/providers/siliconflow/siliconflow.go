// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package siliconflow 实现 ports.UsageProvider，对接硅基流动账户信息接口。
//
// 真实接口契约（docs.siliconflow.com/cn/api-reference/userinfo/get-user-info）：
//   - GET {BaseURL}/v1/user/info，默认 BaseURL = https://api.siliconflow.cn。
//   - 鉴权头：Authorization: Bearer <API_KEY>。
//
// 响应 {code,message,status,data:{balance,chargeBalance,totalBalance,status,...}}。
// code==20000 为成功（业务码，区别于 HTTP 状态码）。三个金额字段全是 string（需
// strconv.ParseFloat，单位即元，无换算）。balance 为当前可用余额；chargeBalance/
// totalBalance 为充值/总额（API 原值，不做相加约定，区别于 deepseek 的 granted/topped_up）。
package siliconflow

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
	defaultBaseURL = "https://api.siliconflow.cn"
	usagePath      = "/v1/user/info"
	httpTimeout    = 10 * time.Second

	sourceTag     = "api-balanced"
	nameAvailable = "Available balance"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 SiliconFlow 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 SiliconFlow Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Provider 返回厂商标识。
func (p *Provider) Provider() string { return "siliconflow" }

// apiResp 是 SiliconFlow /v1/user/info 响应信封。金额字段为 string。
type apiResp struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Status  bool     `json:"status"`
	Data    userInfo `json:"data"`
}

// userInfo 是 data 对象。金额字段为十进制字符串。
type userInfo struct {
	ID            string `json:"id"`
	Balance       string `json:"balance"`
	Status        string `json:"status"`
	ChargeBalance string `json:"chargeBalance"`
	TotalBalance  string `json:"totalBalance"`
}

// FetchUsage 拉取该账号余额，返回单维度余额型 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "siliconflow",
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
		u.Err = fmt.Errorf("siliconflow: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("siliconflow: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("siliconflow: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("siliconflow: decode response: %w", err)
		return u, u.Err
	}
	// 业务信封校验：code==20000 为成功（区别于 HTTP 状态码）。
	if r.Code != 20000 {
		u.Err = fmt.Errorf("siliconflow: code %d: %s", r.Code, r.Message)
		return u, u.Err
	}

	// 账号状态：先于金额解析填充，确保错误路径也携带状态。
	// normal→active；其余非空取值保留原值（frozen/banned 等）；空串不填。
	if r.Data.Status == "normal" {
		u.Status = "active"
	} else if r.Data.Status != "" {
		u.Status = r.Data.Status
	}

	// 主余额严格解析：失败即整体失败。
	balance, err := strconv.ParseFloat(r.Data.Balance, 64)
	if err != nil {
		u.Err = fmt.Errorf("siliconflow: parse balance %q: %w", r.Data.Balance, err)
		return u, u.Err
	}
	// 细分容错解析：ParseFloat 失败返回 0 值，用 _ 忽略 err——
	// 主余额已成功，细分缺失（=0）不致命，UI 自动跳过零值行。
	charge, _ := strconv.ParseFloat(r.Data.ChargeBalance, 64)
	total, _ := strconv.ParseFloat(r.Data.TotalBalance, 64)

	u.Dimensions = []domain.UsageDimension{{
		Name:          nameAvailable,
		Balance:       balance,
		Currency:      "CNY",
		PercentUsed:   -1,
		Source:        sourceTag,
		ChargeBalance: charge,
		TotalBalance:  total,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}
