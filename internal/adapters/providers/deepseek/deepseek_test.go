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

package deepseek

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 DeepSeek /user/balance 响应金样本。金额是 string。
const goldenPayload = `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"110.00","granted_balance":"10.00","topped_up_balance":"100.00"}]}`

func TestProviderReturnsDeepSeek(t *testing.T) {
	if got := New().Provider(); got != "deepseek" {
		t.Fatalf("Provider() = %q, want deepseek", got)
	}
}

// TestFetchUsageGolden：
//
//	(a) Authorization = "Bearer KEY123"
//	(b) 路径 = /user/balance
//	(c) Balance = 110.0（"110.00" ParseFloat，无 /100），Currency = CNY，PercentUsed = -1
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

	t.Setenv("DEEPSEEK_API_KEY", "KEY123")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a)(b)(c) 鉴权 + Content-Type + 路径
	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want Bearer KEY123", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotPath != "/user/balance" {
		t.Errorf("path = %q, want /user/balance", gotPath)
	}

	// (c) 余额型维度
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Name != "Available balance" {
		t.Errorf("dim.Name = %q, want Available balance", d.Name)
	}
	if d.Balance != 110.0 {
		t.Errorf("dim.Balance = %v, want 110.0 (total_balance \"110.00\" parsed, no /100)", d.Balance)
	}
	if d.Currency != "CNY" {
		t.Errorf("dim.Currency = %q, want CNY (from balance_infos[0].currency)", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("dim.PercentUsed = %v, want -1", d.PercentUsed)
	}
	if d.Source != "api-balanced" {
		t.Errorf("dim.Source = %q, want api-balanced", d.Source)
	}
	if d.Granted != 10.0 {
		t.Errorf("dim.Granted = %v, want 10.0 (granted_balance)", d.Granted)
	}
	if d.ToppedUp != 100.0 {
		t.Errorf("dim.ToppedUp = %v, want 100.0 (topped_up_balance)", d.ToppedUp)
	}

	// (d) Primary 指向余额维度
	if u.Primary == nil || u.Primary.Name != "Available balance" {
		t.Errorf("Primary = %+v, want Available balance dim", u.Primary)
	}

	// 账号字段 + Basic Info
	if u.AccountID != "d" || u.Provider != "deepseek" || u.Label != "DeepSeek" {
		t.Errorf("top fields wrong: %+v", u)
	}
	if u.Endpoint != "/user/balance" {
		t.Errorf("Endpoint = %q, want /user/balance", u.Endpoint)
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

// TestFetchUsageUSDCurrency 验证 currency 取自 balance_infos[0].currency（USD 场景）。
func TestFetchUsageUSDCurrency(t *testing.T) {
	payload := `{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"3.00","granted_balance":"0","topped_up_balance":"3.00"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Currency != "USD" {
		t.Errorf("Currency = %q, want USD", u.Dimensions[0].Currency)
	}
	if u.Dimensions[0].Balance != 3.0 {
		t.Errorf("Balance = %v, want 3.0", u.Dimensions[0].Balance)
	}
}

// TestFetchUsageNon200 验证非 2xx 被状态守卫拦截。
func TestFetchUsageNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "BAD")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
	if u.Err == nil {
		t.Error("u.Err should be set")
	}
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty on HTTP error, got %+v", u.Dimensions)
	}
	if u.Primary != nil {
		t.Errorf("Primary should be nil on HTTP error, got %+v", u.Primary)
	}
	if u.AccountID != "d" || u.Provider != "deepseek" || u.Label != "DeepSeek" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}

// TestFetchUsageServerDown 验证传输错误透传 + 账号字段仍填充（结构化错误处理）。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close() // 关闭 server，触发传输错误

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server down")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on transport error")
	}
	if u.AccountID != "d" || u.Provider != "deepseek" || u.Label != "DeepSeek" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}

// TestFetchUsageEmptyBalanceInfos 验证 is_available=true 但 balance_infos 为空数组时
// 触发契约守卫：返回错误且 u.Err 被填充（参见 deepseek.go 空数组分支）。
func TestFetchUsageEmptyBalanceInfos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"is_available":true,"balance_infos":[]}`)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for empty balance_infos, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on empty balance_infos")
	}
}

// TestFetchUsageBadJSON 验证解码失败返回错误且填充 u.Err。
func TestFetchUsageBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on decode error")
	}
}

// TestFetchUsageEmptyCurrency 验证 API 返回 currency:"" 时防御性回退为 CNY：
// 不让空串触发 UI 的 Currency!="" 判定（否则该余额维度会被误判为配额型并渲染 -1%），
// 同时保留余额数据而非整体失败。
func TestFetchUsageEmptyCurrency(t *testing.T) {
	payload := `{"is_available":true,"balance_infos":[{"currency":"","total_balance":"5.00","granted_balance":"0","topped_up_balance":"5.00"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err for empty currency (should default to CNY): %v", err)
	}
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	if u.Dimensions[0].Currency != "CNY" {
		t.Errorf("dim.Currency = %q, want CNY (defensive default for empty)", u.Dimensions[0].Currency)
	}
	if u.Dimensions[0].Balance != 5.0 {
		t.Errorf("dim.Balance = %v, want 5.0 (balance data preserved)", u.Dimensions[0].Balance)
	}
}

// TestFetchUsageBadGrantedBalance 验证 granted_balance 解析失败不致命：
// 主余额 total 照常成功，Granted 留零值，ToppedUp 正常解析。
func TestFetchUsageBadGrantedBalance(t *testing.T) {
	payload := `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"5.00","granted_balance":"oops","topped_up_balance":"5.00"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err (bad granted must NOT fail): %v", err)
	}
	d := u.Dimensions[0]
	if d.Balance != 5.0 {
		t.Errorf("dim.Balance = %v, want 5.0 (total preserved)", d.Balance)
	}
	if d.Granted != 0 {
		t.Errorf("dim.Granted = %v, want 0 (parse failure → zero)", d.Granted)
	}
	if d.ToppedUp != 5.0 {
		t.Errorf("dim.ToppedUp = %v, want 5.0 (valid parse)", d.ToppedUp)
	}
}
