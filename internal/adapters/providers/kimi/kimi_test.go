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

package kimi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 Moonshot /v1/users/me/balance 成功响应金样本。
const goldenPayload = `{"code":0,"data":{"available_balance":49.58894,"voucher_balance":46.58893,"cash_balance":3.00001},"scode":"0x0","status":true}`

func TestVendorReturnsKimi(t *testing.T) {
	if got := New().Vendor(); got != "kimi" {
		t.Fatalf("Vendor() = %q, want kimi", got)
	}
}

// TestCurrencyFor 验证货币推断：moonshot.ai → USD，其余（含 .cn / 本地测试 server）→ CNY。
func TestCurrencyFor(t *testing.T) {
	cases := map[string]string{
		"https://api.moonshot.cn": "CNY",
		"https://api.moonshot.ai": "USD",
		"http://127.0.0.1:1234":   "CNY",
	}
	for base, want := range cases {
		if got := currencyFor(base); got != want {
			t.Errorf("currencyFor(%q) = %q, want %q", base, got, want)
		}
	}
}

// TestFetchUsageGolden 核心金测试：
//
//	(a) Authorization = "Bearer KEY123"（必须 Bearer 前缀）
//	(b) 路径 = /v1/users/me/balance
//	(c) 维度 Balance = available_balance，Currency = CNY（base 是本地 server → CNY），PercentUsed = -1
//	(d) Primary 指向该维度
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotCT, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenPayload)
	}))
	defer srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "KEY123")
	acc := domain.Account{ID: "k", Vendor: "kimi", Label: "Kimi", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a)(b)(c) 鉴权 + Content-Type + 路径
	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want %q (MUST have Bearer prefix)", gotAuth, "Bearer KEY123")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotPath != "/v1/users/me/balance" {
		t.Errorf("path = %q, want /v1/users/me/balance", gotPath)
	}

	// (c) 余额型维度
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Name != "可用余额" {
		t.Errorf("dim.Name = %q, want 可用余额", d.Name)
	}
	if d.Balance != 49.58894 {
		t.Errorf("dim.Balance = %v, want 49.58894 (available_balance)", d.Balance)
	}
	if d.Currency != "CNY" {
		t.Errorf("dim.Currency = %q, want CNY (base is local server)", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("dim.PercentUsed = %v, want -1 (balance type has no percent)", d.PercentUsed)
	}
	if d.Source != "api-balanced" {
		t.Errorf("dim.Source = %q, want api-balanced", d.Source)
	}

	// (d) Primary 指向余额维度
	if u.Primary == nil || u.Primary.Name != "可用余额" {
		t.Errorf("Primary = %+v, want 可用余额 dim", u.Primary)
	}

	// 账号字段 + Basic Info
	if u.AccountID != "k" || u.Vendor != "kimi" || u.Label != "Kimi" {
		t.Errorf("top fields wrong: %+v", u)
	}
	if u.Endpoint != "/v1/users/me/balance" {
		t.Errorf("Endpoint = %q, want /v1/users/me/balance", u.Endpoint)
	}
	if u.BaseURL != srv.URL {
		t.Errorf("BaseURL = %q, want %s", u.BaseURL, srv.URL)
	}
	if u.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set")
	}
	if u.FetchedAt.After(time.Now()) {
		t.Error("FetchedAt must not be in the future")
	}
}

// TestFetchUsageNonZeroCode 验证 code!=0 被拦截（不只看 HTTP 200）。
func TestFetchUsageNonZeroCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":401,"data":{},"status":false}`)
	}))
	defer srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "K")
	acc := domain.Account{ID: "k", Vendor: "kimi", Label: "l", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for code!=0, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set")
	}
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty on code!=0, got %+v", u.Dimensions)
	}
}

// TestFetchUsageServerDown 验证传输错误透传 + 账号字段仍填充。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "K")
	acc := domain.Account{ID: "k", Vendor: "kimi", Label: "l", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server down")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on transport error")
	}
	if u.AccountID != "k" || u.Vendor != "kimi" || u.Label != "l" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}

// TestFetchUsageNon200 验证非 2xx HTTP 状态（如 401）被状态守卫拦截：
// 即使 body 是合法 JSON 错误体（缺 error 字段），也不会被静默解码。
func TestFetchUsageNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "BADKEY")
	acc := domain.Account{ID: "k", Vendor: "kimi", Label: "l", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on non-2xx status")
	}
	// 关键：不能静默把缺字段的错误体当成「错误响应」。
	// Dimensions 应为空（解码从未发生），Primary 也应为 nil。
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty on HTTP error, got %+v", u.Dimensions)
	}
	if u.Primary != nil {
		t.Errorf("Primary should be nil on HTTP error, got %+v", u.Primary)
	}
	// 错误路径下仍填充账号字段
	if u.AccountID != "k" || u.Vendor != "kimi" || u.Label != "l" {
		t.Errorf("error-path VendorUsage fields wrong: %+v", u)
	}
}

// TestFetchUsageBadJSON 验证解码失败返回错误且填充 u.Err。
func TestFetchUsageBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	defer srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "K")
	acc := domain.Account{ID: "k", Vendor: "kimi", Label: "l", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on decode error")
	}
}
