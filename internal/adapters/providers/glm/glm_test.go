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

package glm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 GLM 配额接口的固定金样本（3 个维度：两个 TOKENS_LIMIT + 一个 TIME_LIMIT）。
// nextResetTime 是真实 API 返回的 Unix 毫秒数（13 位 number，非 RFC3339 字符串）。
const goldenPayload = `{"code":200,"data":{"level":"pro","limits":[
  {"type":"TOKENS_LIMIT","percentage":44,"nextResetTime":1775001600000},
  {"type":"TOKENS_LIMIT","percentage":53,"nextResetTime":1775606400000},
  {"type":"TIME_LIMIT","percentage":7,"usage":1000,"currentValue":72,"remaining":928,"nextResetTime":1777593600000}]}}`

func TestVendorReturnsGLM(t *testing.T) {
	if got := New().Vendor(); got != "glm" {
		t.Fatalf("Vendor() = %q, want glm", got)
	}
}

// TestFetchUsageGolden 是核心 httptest 金测试，覆盖四个断言：
//
//	(a) Authorization 头是裸 key（无 Bearer 前缀）+ Content-Type + 路径
//	(b) 解析出 3 个维度
//	(c) Primary 是 PercentUsed 最大 (53) 那档
//	(d) ResetsAt 正确解析
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotCT, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenPayload)
	}))
	defer srv.Close()

	t.Setenv("GLM_API_KEY", "KEY123")
	acc := domain.Account{ID: "g", Vendor: "glm", Label: "智谱", TokenEnv: "GLM_API_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a) 鉴权头是裸 key（GLM 该接口最易错点：不加 Bearer）；Content-Type 与路径正确
	if gotAuth != "KEY123" {
		t.Errorf("Authorization = %q, want %q (bare key, NO Bearer prefix)", gotAuth, "KEY123")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotPath != "/api/monitor/usage/quota/limit" {
		t.Errorf("request path = %q, want /api/monitor/usage/quota/limit", gotPath)
	}

	// (b) 解析出 3 个维度
	if len(u.Dimensions) != 3 {
		t.Fatalf("len(Dimensions) = %d, want 3; dims=%+v", len(u.Dimensions), u.Dimensions)
	}

	// (c) Primary 是 PercentUsed 最大 (53) 那档
	if u.Primary == nil {
		t.Fatalf("Primary is nil; dims=%+v", u.Dimensions)
	}
	if u.Primary.PercentUsed != 53 {
		t.Fatalf("Primary.PercentUsed = %v, want 53", u.Primary.PercentUsed)
	}
	if u.Primary.Name != "每周额度" {
		t.Errorf("Primary.Name = %q, want 每周额度", u.Primary.Name)
	}

	// (d) Primary.ResetsAt 正确解析为 2026-04-08T00:00:00Z
	wantReset := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	if !u.Primary.ResetsAt.Equal(wantReset) {
		t.Fatalf("Primary.ResetsAt = %v, want %v", u.Primary.ResetsAt, wantReset)
	}

	// 维度名映射：TOKENS_LIMIT 按 nextResetTime 升序 → 5小时额度 / 每周额度
	if d, ok := findDim(u.Dimensions, "5小时额度"); !ok || d.PercentUsed != 44 {
		t.Errorf("missing 5小时额度/44 dim; dims=%+v", u.Dimensions)
	}
	if d, ok := findDim(u.Dimensions, "每周额度"); !ok || d.PercentUsed != 53 {
		t.Errorf("missing 每周额度/53 dim; dims=%+v", u.Dimensions)
	}

	// TIME_LIMIT 字段映射：Used=currentValue, Limit=usage, Remaining=remaining, Unit="次"
	mcp, ok := findDim(u.Dimensions, "MCP每月")
	if !ok {
		t.Fatalf("missing MCP每月 dim; dims=%+v", u.Dimensions)
	}
	if mcp.Used != 72 || mcp.Limit != 1000 || mcp.Remaining != 928 {
		t.Errorf("MCP每月 fields: Used=%d Limit=%d Remaining=%d, want 72/1000/928", mcp.Used, mcp.Limit, mcp.Remaining)
	}
	if mcp.Unit != "次" {
		t.Errorf("MCP每月.Unit = %q, want 次", mcp.Unit)
	}
	wantMCPReset := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if !mcp.ResetsAt.Equal(wantMCPReset) {
		t.Errorf("MCP每月.ResetsAt = %v, want %v", mcp.ResetsAt, wantMCPReset)
	}

	// Source 与 FetchedAt
	for _, d := range u.Dimensions {
		if d.Source != "api-balanced" {
			t.Errorf("dim %q Source = %q, want api-balanced", d.Name, d.Source)
		}
	}
	if u.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set to time.Now()")
	}
	if u.FetchedAt.After(time.Now()) {
		t.Error("FetchedAt must not be in the future")
	}

	// VendorUsage 顶层账号字段
	if u.AccountID != "g" || u.Vendor != "glm" || u.Label != "智谱" {
		t.Errorf("VendorUsage top fields wrong: %+v", u)
	}
	// Basic Info 字段（adapter 填充）
	if u.PlanLevel != "pro" {
		t.Errorf("PlanLevel = %q, want pro", u.PlanLevel)
	}
	if u.Endpoint != "/api/monitor/usage/quota/limit" {
		t.Errorf("Endpoint = %q, want /api/monitor/usage/quota/limit", u.Endpoint)
	}
	if u.BaseURL != srv.URL {
		t.Errorf("BaseURL = %q, want %s", u.BaseURL, srv.URL)
	}
}

// TestFetchUsageUnsortedTokens 验证 TOKENS_LIMIT 按 nextResetTime 升序命名：
// 即便响应里 weekly(53%/04-08) 在前、5h(44%/04-01) 在后，
// 实现也必须按 reset 时间升序重排后赋名。
func TestFetchUsageUnsortedTokens(t *testing.T) {
	payload := `{"code":200,"data":{"limits":[
	  {"type":"TOKENS_LIMIT","percentage":53,"nextResetTime":1775606400},
	  {"type":"TOKENS_LIMIT","percentage":44,"nextResetTime":1775001600}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("GLM_API_KEY", "K")
	acc := domain.Account{ID: "g", Vendor: "glm", TokenEnv: "GLM_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(u.Dimensions) != 2 {
		t.Fatalf("dims = %d, want 2", len(u.Dimensions))
	}
	// 升序后 dims[0]=5小时额度(44%), dims[1]=每周额度(53%)
	if u.Dimensions[0].Name != "5小时额度" || u.Dimensions[0].PercentUsed != 44 {
		t.Errorf("dims[0] = %+v, want 5小时额度/44", u.Dimensions[0])
	}
	if u.Dimensions[1].Name != "每周额度" || u.Dimensions[1].PercentUsed != 53 {
		t.Errorf("dims[1] = %+v, want 每周额度/53", u.Dimensions[1])
	}
	// Primary 仍是 53 那档（按 PercentUsed 选，与排序无关）
	if u.Primary == nil || u.Primary.Name != "每周额度" {
		t.Errorf("Primary = %+v, want 每周额度", u.Primary)
	}
}

// TestFetchUsageNon200Code 验证非 200 响应码返回错误，且 VendorUsage 仍填充账号字段。
func TestFetchUsageNon200Code(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":401,"data":{}}`)
	}))
	defer srv.Close()

	t.Setenv("GLM_API_KEY", "K")
	acc := domain.Account{ID: "g", Vendor: "glm", Label: "l", TokenEnv: "GLM_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for non-200 code, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on error path")
	}
	// 错误路径下仍填充账号字段（与 mock provider 行为一致，便于上层展示局部信息）
	if u.AccountID != "g" || u.Vendor != "glm" || u.Label != "l" {
		t.Errorf("error-path VendorUsage fields wrong: %+v", u)
	}
}

// TestFetchUsageServerDown 验证 HTTP 层错误被透传。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close() // 立即关闭，下次请求必然失败

	t.Setenv("GLM_API_KEY", "K")
	acc := domain.Account{ID: "g", Vendor: "glm", TokenEnv: "GLM_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server is down, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on transport error")
	}
}

func findDim(dims []domain.UsageDimension, name string) (domain.UsageDimension, bool) {
	for _, d := range dims {
		if d.Name == name {
			return d, true
		}
	}
	return domain.UsageDimension{}, false
}
