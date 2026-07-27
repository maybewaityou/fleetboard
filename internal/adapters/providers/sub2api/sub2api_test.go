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

// goldenUsage 是 sub2api /v1/usage 响应金样本（字段名按社区实现假设；
// 若真实实例字段不同，仅改 apiResp 的 json tag 与本 golden 即可）。
const goldenUsage = `{"balance":42.5,"used":7.5}`

func TestProviderReturnsSlug(t *testing.T) {
	if got := New().Provider(); got != "sub2api" {
		t.Fatalf("Provider() = %q, want sub2api", got)
	}
}

// TestFetchUsageGolden：(a) Authorization=Bearer KEY；(b) 路径=/v1/usage；
// (c) Balance=42.5、Currency=USD、PercentUsed=-1；(d) Primary 指向余额维度。
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenUsage)
	}))
	defer srv.Close()

	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "MyRelay", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotAuth != "Bearer KEY" {
		t.Errorf("Authorization = %q, want Bearer KEY", gotAuth)
	}
	if gotPath != "/v1/usage" {
		t.Errorf("path = %q, want /v1/usage", gotPath)
	}
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Balance != 42.5 {
		t.Errorf("Balance = %v, want 42.5", d.Balance)
	}
	if d.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("PercentUsed = %v, want -1", d.PercentUsed)
	}
	if d.Source != "sub2api" {
		t.Errorf("Source = %q, want sub2api", d.Source)
	}
	if u.Primary == nil || u.Primary.Balance != 42.5 {
		t.Errorf("Primary wrong: %+v", u.Primary)
	}
	if u.Endpoint != "/v1/usage" || u.BaseURL != srv.URL {
		t.Errorf("basic info wrong: %+v", u)
	}
}

// TestFetchUsage_NegativeBalance 余额可为负（订阅+余额并存场景），仍正常返回。
func TestFetchUsage_NegativeBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"balance":-3.25,"used":0}`)
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
