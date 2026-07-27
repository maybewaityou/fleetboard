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

package newapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

const (
	subscriptionPayload = `{"object":"billing_subscription","hard_limit_usd":100,"system_hard_limit_usd":100,"has_payment_method":true}`
	usagePayload        = `{"object":"list","total_usage":12.5}`
)

func TestProviderReturnsSlug(t *testing.T) {
	if got := New().Provider(); got != "newapi" {
		t.Fatalf("Provider() = %q, want newapi", got)
	}
}

// TestFetchUsageGolden：余额 = system_hard_limit_usd(100) - total_usage(12.5) = 87.5；
// 鉴权 Bearer；命中两个 billing 端点。
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth string
	var hitSub, hitUsage bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case strings.HasSuffix(r.URL.Path, "/dashboard/billing/subscription"):
			hitSub = true
			fmt.Fprint(w, subscriptionPayload)
		case strings.HasSuffix(r.URL.Path, "/dashboard/billing/usage"):
			hitUsage = true
			fmt.Fprint(w, usagePayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("NEWAPI_KEY", "KEY")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "NewAPI", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotAuth != "Bearer KEY" {
		t.Errorf("Authorization = %q, want Bearer KEY", gotAuth)
	}
	if !hitSub || !hitUsage {
		t.Errorf("both endpoints must be hit: sub=%v usage=%v", hitSub, hitUsage)
	}
	if len(u.Dimensions) != 1 || u.Dimensions[0].Balance != 87.5 {
		t.Errorf("Balance = %v, want 87.5 (100-12.5)", u.Dimensions[0].Balance)
	}
	if u.Dimensions[0].Currency != "USD" || u.Dimensions[0].PercentUsed != -1 {
		t.Errorf("dim wrong: %+v", u.Dimensions[0])
	}
	if u.Primary == nil || u.Primary.Balance != 87.5 {
		t.Errorf("Primary wrong: %+v", u.Primary)
	}
}

// TestFetchUsage_LimitFallback system_hard_limit_usd 为 0 时回退 hard_limit_usd。
func TestFetchUsage_LimitFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscription") {
			fmt.Fprint(w, `{"hard_limit_usd":50,"system_hard_limit_usd":0}`)
		} else {
			fmt.Fprint(w, `{"total_usage":10}`)
		}
	}))
	defer srv.Close()
	t.Setenv("NEWAPI_KEY", "K")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != 40 { // 50-10
		t.Errorf("Balance = %v, want 40 (fallback hard_limit_usd)", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_UsageDegraded usage 端点失败时降级 used=0（余额=总额），不报错。
func TestFetchUsage_UsageDegraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscription") {
			fmt.Fprint(w, subscriptionPayload)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("NEWAPI_KEY", "K")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("usage 404 should degrade, not error: %v", err)
	}
	if u.Dimensions[0].Balance != 100 { // 总额 - 0
		t.Errorf("Balance = %v, want 100 (degraded used=0)", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_SubscriptionFails subscription 端点失败 → 报错（无法取总额）。
func TestFetchUsage_SubscriptionFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscription") {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			fmt.Fprint(w, usagePayload)
		}
	}))
	defer srv.Close()
	t.Setenv("NEWAPI_KEY", "BAD")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: srv.URL}
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("subscription failure should error")
	}
}

// TestFetchUsage_BaseURLRequired 自部署无默认 base。
func TestFetchUsage_BaseURLRequired(t *testing.T) {
	t.Setenv("NEWAPI_KEY", "K")
	acc := domain.Account{ID: "n", Provider: "newapi", Label: "x", TokenEnv: "NEWAPI_KEY", BaseURL: ""}
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("expected error for missing base_url")
	}
}
