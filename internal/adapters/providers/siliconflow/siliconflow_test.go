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

package siliconflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 SiliconFlow /v1/user/info 响应金样本。金额是 string。
const goldenPayload = `{"code":20000,"message":"OK","status":true,"data":{"id":"user123","balance":"0.88","status":"normal","chargeBalance":"88.00","totalBalance":"88.88"}}`

func TestProviderReturnsSiliconFlow(t *testing.T) {
	if got := New().Provider(); got != "siliconflow" {
		t.Fatalf("Provider() = %q, want siliconflow", got)
	}
}

// TestFetchUsageNon200 验证非 2xx 被状态守卫拦截，错误路径仍填充账号字段。
func TestFetchUsageNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"code":40100,"message":"invalid api key"}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "BAD")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
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
	if u.AccountID != "sf" || u.Provider != "siliconflow" || u.Label != "SiliconFlow" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}

// TestFetchUsageBadJSON 验证解码失败返回错误且填充 u.Err。
func TestFetchUsageBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on decode error")
	}
}

// TestFetchUsageBadEnvelope 验证 HTTP 200 但业务 code≠20000 被信封守卫拦截。
func TestFetchUsageBadEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":40100,"message":"invalid api key","status":false,"data":{}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for code!=20000, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on bad envelope")
	}
}

// TestFetchUsageBadBalance 验证主余额非数字时整体失败（严格解析）。
func TestFetchUsageBadBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":20000,"message":"OK","status":true,"data":{"balance":"oops","status":"normal","chargeBalance":"88.00","totalBalance":"88.88"}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for non-numeric balance, got nil")
	}
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty when balance parse fails, got %+v", u.Dimensions)
	}
}

// TestFetchUsageBadChargeBalance 验证细分解析失败不致命：主余额照常成功，细分留零值。
func TestFetchUsageBadChargeBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":20000,"message":"OK","status":true,"data":{"balance":"5.00","status":"normal","chargeBalance":"oops","totalBalance":"oops"}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err (bad chargeBalance must NOT fail): %v", err)
	}
	d := u.Dimensions[0]
	if d.Balance != 5.0 {
		t.Errorf("dim.Balance = %v, want 5.0 (balance preserved)", d.Balance)
	}
	if d.ChargeBalance != 0 {
		t.Errorf("dim.ChargeBalance = %v, want 0 (parse failure → zero)", d.ChargeBalance)
	}
	if d.TotalBalance != 0 {
		t.Errorf("dim.TotalBalance = %v, want 0 (parse failure → zero)", d.TotalBalance)
	}
}

// TestFetchUsageStatusNonNormal 验证非 normal 状态保留原值字符串。
func TestFetchUsageStatusNonNormal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":20000,"message":"OK","status":true,"data":{"balance":"1.00","status":"frozen","chargeBalance":"0","totalBalance":"1.00"}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.Status != "frozen" {
		t.Errorf("Status = %q, want frozen (preserved original)", u.Status)
	}
}

// TestFetchUsageServerDown 验证传输错误透传 + 账号字段仍填充。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close() // 关闭 server，触发传输错误

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server down")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on transport error")
	}
	if u.AccountID != "sf" || u.Provider != "siliconflow" || u.Label != "SiliconFlow" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}

// TestFetchUsageGolden：
//
//	(a) Authorization = "Bearer KEY123"，Content-Type = application/json
//	(b) 路径 = /v1/user/info
//	(c) Balance = 0.88（"0.88" ParseFloat，无换算），Currency = CNY，PercentUsed = -1
//	(d) ChargeBalance = 88.0，TotalBalance = 88.88
//	(e) Primary 指向该维度
//	(f) Status = active（data.status="normal"）
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotCT, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenPayload)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "KEY123")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a)(b) 鉴权 + Content-Type + 路径
	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want Bearer KEY123", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotPath != "/v1/user/info" {
		t.Errorf("path = %q, want /v1/user/info", gotPath)
	}

	// (c)(d) 余额型维度 + 细分
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Name != "Available balance" {
		t.Errorf("dim.Name = %q, want Available balance", d.Name)
	}
	if d.Balance != 0.88 {
		t.Errorf("dim.Balance = %v, want 0.88 (balance parsed)", d.Balance)
	}
	if d.Currency != "CNY" {
		t.Errorf("dim.Currency = %q, want CNY", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("dim.PercentUsed = %v, want -1", d.PercentUsed)
	}
	if d.Source != "api-balanced" {
		t.Errorf("dim.Source = %q, want api-balanced", d.Source)
	}
	if d.ChargeBalance != 88.0 {
		t.Errorf("dim.ChargeBalance = %v, want 88.0 (chargeBalance)", d.ChargeBalance)
	}
	if d.TotalBalance != 88.88 {
		t.Errorf("dim.TotalBalance = %v, want 88.88 (totalBalance)", d.TotalBalance)
	}

	// (e) Primary 指向余额维度
	if u.Primary == nil || u.Primary.Name != "Available balance" {
		t.Errorf("Primary = %+v, want Available balance dim", u.Primary)
	}

	// (f) 账号状态：data.status="normal" → Status="active"
	if u.Status != "active" {
		t.Errorf("Status = %q, want active (data.status=normal)", u.Status)
	}

	// 账号字段 + Basic Info
	if u.AccountID != "sf" || u.Provider != "siliconflow" || u.Label != "SiliconFlow" {
		t.Errorf("top fields wrong: %+v", u)
	}
	if u.Endpoint != "/v1/user/info" {
		t.Errorf("Endpoint = %q, want /v1/user/info", u.Endpoint)
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
