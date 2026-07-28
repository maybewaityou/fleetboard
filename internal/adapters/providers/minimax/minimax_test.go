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

// goldenPayload 是官网 remains_percent 接口的固定金样本（基于真实响应精简）：
//   - general 档：5h 窗口 current_interval_used_percent="9%" / status=1（有限），
//     周窗口 current_weekly_used_percent="0%" / status=3（∞ 无限制）。
//   - end_time=1711929600000（2024-04-01 00:00:00 UTC，毫秒）→ 5h ResetsAt；
//     weekly_end_time=1712448000000（2024-04-07 00:00:00 UTC，毫秒）→ weekly ResetsAt。
//   - base_resp.status_code=0（成功）。
const goldenPayload = `{"model_remains":[{"model_name":"general","start_time":1711843200000,"end_time":1711929600000,"current_interval_used_percent":"9%","current_interval_status":1,"weekly_start_time":1711843200000,"weekly_end_time":1712448000000,"current_weekly_used_percent":"0%","current_weekly_status":3}],"base_resp":{"status_code":0,"status_msg":"success"}}`

func TestProviderReturnsMiniMax(t *testing.T) {
	if got := New().Provider(); got != "minimax" {
		t.Fatalf("Provider() = %q, want minimax", got)
	}
}

// TestFetchUsageGolden 是核心 httptest 金测试，覆盖：
//
//	(a) Authorization 头是 "Bearer KEY123"（必须有 Bearer 前缀）
//	(b) 请求路径为 /backend/account/token_plan/remains_percent（真实接口）
//	(c) 两个维度：5h(9%, 有限) + weekly(∞ 无限制)
//	(d) Primary 指向 5h（weekly PercentUsed=-1 被 SelectPrimary 跳过）
//	(e) Model 取自 model_name="general"
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
	acc := domain.Account{ID: "m", Provider: "minimax", Label: "MiniMax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a)/(b) 鉴权头 + 真实路径。
	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want %q (MUST have Bearer prefix)", gotAuth, "Bearer KEY123")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotPath != "/backend/account/token_plan/remains_percent" {
		t.Errorf("request path = %q, want /backend/account/token_plan/remains_percent", gotPath)
	}

	// (c) 两个维度。
	if len(u.Dimensions) != 2 {
		t.Fatalf("len(Dimensions) = %d, want 2; dims=%+v", len(u.Dimensions), u.Dimensions)
	}
	d5h, dWeekly := u.Dimensions[0], u.Dimensions[1]

	// 5h 窗口：有限额，9%。
	if d5h.Name != "5h Quota" {
		t.Errorf("5h.Name = %q, want 5h Quota", d5h.Name)
	}
	if d5h.Order != 1 {
		t.Errorf("5h.Order = %v, want 1", d5h.Order)
	}
	if d5h.PercentUsed != 9 {
		t.Errorf("5h.PercentUsed = %v, want 9", d5h.PercentUsed)
	}
	if d5h.Unlimited {
		t.Errorf("5h.Unlimited = true, want false (status=1 = 有限)")
	}
	wantReset5h := time.UnixMilli(1711929600000).UTC()
	if !d5h.ResetsAt.Equal(wantReset5h) {
		t.Errorf("5h.ResetsAt = %v, want %v", d5h.ResetsAt, wantReset5h)
	}

	// 周窗口：无限制（status=3）→ Unlimited + PercentUsed=-1。
	if dWeekly.Name != "Weekly Quota" {
		t.Errorf("weekly.Name = %q, want Weekly Quota", dWeekly.Name)
	}
	if dWeekly.Order != 2 {
		t.Errorf("weekly.Order = %v, want 2", dWeekly.Order)
	}
	if !dWeekly.Unlimited {
		t.Errorf("weekly.Unlimited = false, want true (status=3 = ∞ 无限制)")
	}
	if dWeekly.PercentUsed != -1 {
		t.Errorf("weekly.PercentUsed = %v, want -1 (unlimited → N/A, NOT the literal \"0%%\")", dWeekly.PercentUsed)
	}
	wantResetWeekly := time.UnixMilli(1712448000000).UTC()
	if !dWeekly.ResetsAt.Equal(wantResetWeekly) {
		t.Errorf("weekly.ResetsAt = %v, want %v", dWeekly.ResetsAt, wantResetWeekly)
	}

	// (d) Primary 指向 5h（9%）—— weekly 被跳过。这是修复的核心：列表/详情不再显示 0%。
	if u.Primary == nil || u.Primary.Name != "5h Quota" || u.Primary.PercentUsed != 9 {
		t.Errorf("Primary = %+v, want 5h Quota @ 9%%", u.Primary)
	}

	// (e) Model 取自 model_name。
	if u.Model != "general" {
		t.Errorf("Model = %q, want general", u.Model)
	}
	if u.Endpoint != "/backend/account/token_plan/remains_percent" {
		t.Errorf("Endpoint = %q, want remains_percent path", u.Endpoint)
	}
	if u.BaseURL != srv.URL {
		t.Errorf("BaseURL = %q, want %s", u.BaseURL, srv.URL)
	}
	if u.FetchedAt.IsZero() || u.FetchedAt.After(time.Now()) {
		t.Errorf("FetchedAt wrong: %v", u.FetchedAt)
	}
}

// TestParsePercent 验证字符串百分比解析与降级。
func TestParsePercent(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"9%", 9},
		{"0%", 0},
		{"100%", 100},
		{" 17% ", 17}, // 含空白
		{"", -1},      // 空 → N/A
		{"abc", -1},   // 坏值 → N/A
		{"%", -1},     // 仅百分号 → N/A
	}
	for _, c := range cases {
		if got := parsePercent(c.in); got != c.want {
			t.Errorf("parsePercent(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestFetchUsageEmptyRemains 验证 model_remains 为空时不 panic（len 守卫）、
// 返回 0 维度且 Primary=nil。这是空数组越界的回归保护。
func TestFetchUsageEmptyRemains(t *testing.T) {
	payload := `{"model_remains":[],"base_resp":{"status_code":0,"status_msg":"success"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Provider: "minimax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err on empty model_remains: %v", err)
	}
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions = %+v, want empty (no model_remains)", u.Dimensions)
	}
	if u.Primary != nil {
		t.Errorf("Primary = %+v, want nil", u.Primary)
	}
	if u.Model != "" {
		t.Errorf("Model = %q, want empty", u.Model)
	}
}

// TestFetchUsageBusinessError 验证 HTTP 200 但 base_resp.status_code != 0 被业务码守卫拦截
// （鉴权失败等可能返回 200 + 非 0 业务码）。
func TestFetchUsageBusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model_remains":[],"base_resp":{"status_code":1004,"status_msg":"invalid api key"}}`)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "BADKEY")
	acc := domain.Account{ID: "m", Provider: "minimax", Label: "l", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for base_resp.status_code=1004, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on business error")
	}
	// 业务错误不能静默产出维度（否则会把空 model_remains 误当正常）。
	if len(u.Dimensions) != 0 || u.Primary != nil {
		t.Errorf("business error must not yield dimensions: %+v", u.Dimensions)
	}
}

// TestFetchUsageAllUnlimited 验证两个窗口都无限制时 Primary=nil（SelectPrimary 全跳过），
// 且两维度均 Unlimited。覆盖「全无限」退化场景，不崩溃。
func TestFetchUsageAllUnlimited(t *testing.T) {
	payload := `{"model_remains":[{"model_name":"general","end_time":1711929600000,"current_interval_used_percent":"0%","current_interval_status":3,"weekly_end_time":1712448000000,"current_weekly_used_percent":"0%","current_weekly_status":3}],"base_resp":{"status_code":0,"status_msg":"success"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Provider: "minimax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(u.Dimensions) != 2 {
		t.Fatalf("Dimensions = %d, want 2", len(u.Dimensions))
	}
	for i, d := range u.Dimensions {
		if !d.Unlimited || d.PercentUsed != -1 {
			t.Errorf("dim[%d] = %+v, want Unlimited + PercentUsed=-1", i, d)
		}
	}
	if u.Primary != nil {
		t.Errorf("Primary = %+v, want nil when all windows unlimited", u.Primary)
	}
}

// TestFetchUsageServerDown 验证 HTTP 传输错误被透传，且 ProviderUsage 仍填充账号字段。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close() // 立即关闭，下次请求必然失败

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Provider: "minimax", Label: "l", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server is down, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on transport error")
	}
	if u.AccountID != "m" || u.Provider != "minimax" || u.Label != "l" {
		t.Errorf("error-path ProviderUsage fields wrong: %+v", u)
	}
}

// TestFetchUsageNon200 验证非 2xx HTTP 状态（如 401）被状态守卫拦截，
// 即使 body 是合法 JSON 错误体也不会被静默解码。
func TestFetchUsageNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"model_remains":[],"base_resp":{"status_code":1004,"status_msg":"invalid api key"}}`)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "BADKEY")
	acc := domain.Account{ID: "m", Provider: "minimax", Label: "l", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on non-2xx status")
	}
	if len(u.Dimensions) != 0 || u.Primary != nil {
		t.Errorf("Dimensions/Primary should be empty on HTTP error: %+v", u.Dimensions)
	}
}

// TestFetchUsageBadJSON 验证解码失败返回错误且填充 u.Err。
func TestFetchUsageBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	defer srv.Close()

	t.Setenv("MINIMAX_TOKEN_PLAN_KEY", "K")
	acc := domain.Account{ID: "m", Provider: "minimax", TokenEnv: "MINIMAX_TOKEN_PLAN_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on decode error")
	}
}
