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

package minimax

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 MiniMax token_plan/remains 接口的固定金样本：
//   - usagePercent = 12（已用 12%，直接用不反转）
//   - model_remains[0].end_time = 1711929600（2024-04-01 00:00:00 UTC）→ ResetsAt
//   - start_time = 1711843200（2024-03-31 00:00:00 UTC）
const goldenPayload = `{"usagePercent":12,"model_remains":[{"model":"abab6","start_time":1711843200,"end_time":1711929600}]}`

func TestVendorReturnsMiniMax(t *testing.T) {
	if got := New().Vendor(); got != "minimax" {
		t.Fatalf("Vendor() = %q, want minimax", got)
	}
}

// TestFetchUsageGolden 是核心 httptest 金测试，覆盖三个断言：
//
//	(a) Authorization 头是 "Bearer KEY123"（必须有 Bearer 前缀——区别于 GLM 裸 key）
//	(b) PercentUsed == 12（usagePercent=12 是「已用」，直接用不反转）
//	(c) ResetsAt 取自 end_time（Unix 秒 1711929600 → 2024-04-01 00:00:00 UTC）
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotCT, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenPayload)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "KEY123")
	acc := domain.Account{ID: "m", Vendor: "minimax", Label: "MiniMax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a) 鉴权头必须有 "Bearer " 前缀（MiniMax 该接口最易错点）
	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want %q (MUST have Bearer prefix)", gotAuth, "Bearer KEY123")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotPath != "/v1/token_plan/remains" {
		t.Errorf("request path = %q, want /v1/token_plan/remains", gotPath)
	}

	// (b) 单一维度
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1; dims=%+v", len(u.Dimensions), u.Dimensions)
	}
	d := u.Dimensions[0]
	if d.Name != "Token Plan" {
		t.Errorf("dim.Name = %q, want Token Plan", d.Name)
	}
	// 不反转：usagePercent=12 即已用 12%
	if d.PercentUsed != 12 {
		t.Errorf("dim.PercentUsed = %v, want 12 (usagePercent is USED, not inverted)", d.PercentUsed)
	}
	if d.Unit != "%" {
		t.Errorf("dim.Unit = %q, want %%", d.Unit)
	}
	if d.Source != "api-balanced" {
		t.Errorf("dim.Source = %q, want api-balanced", d.Source)
	}

	// (c) ResetsAt 来自 end_time（Unix 秒 1711929600 → 2024-04-01 00:00:00 UTC）
	wantReset := time.Unix(1711929600, 0).UTC()
	if !d.ResetsAt.Equal(wantReset) {
		t.Errorf("dim.ResetsAt = %v, want %v", d.ResetsAt, wantReset)
	}

	// Primary 指向唯一维度
	if u.Primary == nil || u.Primary.Name != "Token Plan" {
		t.Errorf("Primary = %+v, want Token Plan dim", u.Primary)
	}
	if u.Primary.PercentUsed != 12 {
		t.Errorf("Primary.PercentUsed = %v, want 12", u.Primary.PercentUsed)
	}

	// FetchedAt 与顶层账号字段
	if u.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set to time.Now()")
	}
	if u.FetchedAt.After(time.Now()) {
		t.Error("FetchedAt must not be in the future")
	}
	if u.AccountID != "m" || u.Vendor != "minimax" || u.Label != "MiniMax" {
		t.Errorf("VendorUsage top fields wrong: %+v", u)
	}
}

// TestFetchUsageSnakeCaseField 验证 usage_percent snake_case 变体同样被解析
// （真实 API 存在两种字段名，实现必须兼容）。
func TestFetchUsageSnakeCaseField(t *testing.T) {
	payload := `{"usage_percent":25,"model_remains":[{"start_time":1711843200,"end_time":1711929600}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Vendor: "minimax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// 不反转：usagePercent=25 即已用 25%
	if u.Dimensions[0].PercentUsed != 25 {
		t.Errorf("PercentUsed = %v, want 25 (usagePercent=25, not inverted)", u.Dimensions[0].PercentUsed)
	}
}

// TestFetchUsageEmptyRemains 验证 model_remains 为空时仍返回维度（ResetsAt 为零值，
// UI 层会跳过零 ResetsAt）。
func TestFetchUsageEmptyRemains(t *testing.T) {
	payload := `{"usagePercent":50,"model_remains":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Vendor: "minimax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(u.Dimensions) != 1 {
		t.Fatalf("dims = %d, want 1", len(u.Dimensions))
	}
	if u.Dimensions[0].PercentUsed != 50 {
		t.Errorf("PercentUsed = %v, want 50", u.Dimensions[0].PercentUsed)
	}
	if !u.Dimensions[0].ResetsAt.IsZero() {
		t.Errorf("ResetsAt should be zero for empty model_remains, got %v", u.Dimensions[0].ResetsAt)
	}
}

// TestFetchUsageServerDown 验证 HTTP 层错误被透传，且 VendorUsage 仍填充账号字段
// （与 GLM / mock provider 行为一致，便于上层展示局部信息）。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close() // 立即关闭，下次请求必然失败

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Vendor: "minimax", Label: "l", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server is down, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on transport error")
	}
	// 错误路径下仍填充账号字段
	if u.AccountID != "m" || u.Vendor != "minimax" || u.Label != "l" {
		t.Errorf("error-path VendorUsage fields wrong: %+v", u)
	}
}

// TestFetchUsageNon200 验证非 2xx HTTP 状态（如 401）被状态守卫拦截：
// 即使 body 是合法 JSON 错误体（缺 usage_percent），也不会被静默解码成 PercentUsed==100。
// MiniMax 鉴权失败典型响应：{"base_resp":{"status_code":1004,...}}。
func TestFetchUsageNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"base_resp":{"status_code":1004,"status_msg":"invalid api key"}}`)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "BADKEY")
	acc := domain.Account{ID: "m", Vendor: "minimax", Label: "l", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on non-2xx status")
	}
	// 关键：不能静默把缺字段的错误体当成「100% 已耗尽」。
	// Dimensions 应为空（解码从未发生），Primary 也应为 nil。
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty on HTTP error, got %+v (would risk PercentUsed==100)", u.Dimensions)
	}
	if u.Primary != nil {
		t.Errorf("Primary should be nil on HTTP error, got %+v", u.Primary)
	}
	// 错误路径下仍填充账号字段
	if u.AccountID != "m" || u.Vendor != "minimax" || u.Label != "l" {
		t.Errorf("error-path VendorUsage fields wrong: %+v", u)
	}
}

// TestFetchUsageBadJSON 验证解码失败返回错误且填充 u.Err。
func TestFetchUsageBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Vendor: "minimax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on decode error")
	}
}
