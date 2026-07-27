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

package sub2api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// 三模式金样本（字段取自 Wei-Shaw/sub2api gateway_handler.go 的真实响应结构）。
const goldenWallet = `{"mode":"unrestricted","isValid":true,"planName":"钱包余额","remaining":42.5,"unit":"USD","balance":42.5}`

const goldenQuota = `{
  "mode":"quota_limited","isValid":true,"status":"active","unit":"USD",
  "quota":{"limit":100,"used":37.5,"remaining":62.5,"unit":"USD"},
  "remaining":62.5,
  "rate_limits":[
    {"window":"5h","limit":20,"used":7,"remaining":13,"reset_at":"2026-07-28T05:00:00Z"},
    {"window":"1d","limit":50,"used":12,"remaining":38,"reset_at":"2026-07-28T00:00:00Z"}
  ],
  "expires_at":"2026-12-31T00:00:00Z","days_until_expiry":156,
  "usage":{"today":{"requests":10,"total_tokens":3050,"cost":1.5},"total":{"requests":100,"total_tokens":30000,"cost":15.0},"average_duration_ms":2500,"rpm":5,"tpm":1500}
}`

const goldenSubscription = `{
  "mode":"unrestricted","isValid":true,"planName":"Weekly plan","remaining":12.5,"unit":"USD",
  "subscription":{"daily_usage_usd":2.5,"weekly_usage_usd":7.5,"monthly_usage_usd":50.0,"daily_limit_usd":5.0,"weekly_limit_usd":20.0,"monthly_limit_usd":100.0,"weekly_window_start":"2026-07-13T00:30:00+08:00","expires_at":"2026-12-31T00:00:00Z"},
  "usage":{"today":{"requests":3,"total_tokens":800,"cost":0.4},"total":{"requests":30,"total_tokens":8000,"cost":4.0},"rpm":1,"tpm":400}
}`

func TestProviderReturnsSlug(t *testing.T) {
	if got := New().Provider(); got != "sub2api" {
		t.Fatalf("Provider() = %q, want sub2api", got)
	}
}

// 钱包模式：balance 归一为 Primary.Balance，仅 1 个维度，PlanLevel=钱包余额。
func TestFetchUsage_WalletMode(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenWallet)
	}))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "MyRelay", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotAuth != "Bearer KEY" || gotPath != "/v1/usage" {
		t.Errorf("auth/path wrong: %q %q", gotAuth, gotPath)
	}
	if len(u.Dimensions) != 1 {
		t.Fatalf("wallet: len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	if u.Dimensions[0].Balance != 42.5 || u.Dimensions[0].Currency != "USD" || u.Dimensions[0].PercentUsed != -1 {
		t.Errorf("wallet primary wrong: %+v", u.Dimensions[0])
	}
	if u.PlanLevel != "钱包余额" {
		t.Errorf("PlanLevel = %q, want 钱包余额", u.PlanLevel)
	}
}

// 配额模式：Balance=quota.remaining；维度=1 primary + 1 Total quota + 2 rate_limits；Recent 非空；状态/过期填充。
func TestFetchUsage_QuotaMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, goldenQuota) }))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "q", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != 62.5 {
		t.Errorf("quota primary Balance = %v, want 62.5", u.Dimensions[0].Balance)
	}
	// 1 primary + 1 Total quota + 2 rate_limits = 4
	if len(u.Dimensions) != 4 {
		t.Fatalf("quota: len(Dimensions) = %d, want 4", len(u.Dimensions))
	}
	// Total quota 维度（index 1）
	tq := u.Dimensions[1]
	if tq.Name != "Total quota" || tq.MoneyLimit != 100 || tq.MoneyUsed != 37.5 || tq.Balance != 62.5 {
		t.Errorf("Total quota dim wrong: %+v", tq)
	}
	// 第一个 rate_limit 窗口（index 2）带 ResetsAt
	rl := u.Dimensions[2]
	if rl.Name != "5h window" || rl.MoneyLimit != 20 || rl.ResetsAt.IsZero() {
		t.Errorf("5h window dim wrong: %+v", rl)
	}
	if u.Recent == nil || u.Recent.TodayCost != 1.5 || u.Recent.TotalTokens != 30000 || u.Recent.RPM != 5 {
		t.Errorf("Recent wrong: %+v", u.Recent)
	}
	if u.APIKeyStatus != "active" || u.DaysUntilExpiry != 156 || u.ExpiresAt == nil {
		t.Errorf("status/expiry wrong: %+v", u)
	}
}

// 订阅模式：Balance=remaining；维度=1 primary + 3 日/周/月；PlanLevel=Weekly plan。
func TestFetchUsage_SubscriptionMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, goldenSubscription) }))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "sub", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != 12.5 {
		t.Errorf("sub primary Balance = %v, want 12.5", u.Dimensions[0].Balance)
	}
	// 1 primary + 日/周/月 = 4
	if len(u.Dimensions) != 4 {
		t.Fatalf("sub: len(Dimensions) = %d, want 4", len(u.Dimensions))
	}
	if u.PlanLevel != "Weekly plan" {
		t.Errorf("PlanLevel = %q, want Weekly plan", u.PlanLevel)
	}
}

func TestFetchUsage_NegativeBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"mode":"unrestricted","planName":"钱包余额","remaining":-3.25,"unit":"USD","balance":-3.25}`)
	}))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "K")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "neg", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != -3.25 {
		t.Errorf("Balance = %v, want -3.25", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_BaseURLRequired 自部署平台无默认 base；缺失即报错。
func TestFetchUsage_BaseURLRequired(t *testing.T) {
	t.Setenv("SUB2API_KEY", "K")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "x", TokenEnv: "SUB2API_KEY", BaseURL: ""}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for missing base_url")
	}
	if u.Err == nil {
		t.Error("u.Err should be set")
	}
}

// TestFetchUsage_Non200 非 2xx 被状态守卫拦截。
func TestFetchUsage_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "BAD")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "x", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}
