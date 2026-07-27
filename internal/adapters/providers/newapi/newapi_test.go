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
	"strconv"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func TestProviderReturnsSlug(t *testing.T) {
	if got := New().Provider(); got != "newapi" {
		t.Fatalf("Provider() = %q, want newapi", got)
	}
}

// nativeServer 构造一个 mock，按路径分发 4 个原生端点。
// stat 的 7d/30d 用 start_timestamp 区分：7d 窗口 start 更接近 now（> now-15d）。
func nativeServer(t *testing.T, userSelfStatus, statusStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("New-Api-User"); got != "16002" {
			t.Errorf("New-Api-User header = %q, want 16002", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer AT" {
			t.Errorf("Authorization = %q, want Bearer AT", got)
		}
		switch r.URL.Path {
		case "/api/user/self":
			w.WriteHeader(userSelfStatus)
			if userSelfStatus == 200 {
				fmt.Fprint(w, `{"data":{"quota":121992688,"used_quota":69281250,"request_count":29}}`)
			}
		case "/api/status":
			w.WriteHeader(statusStatus)
			if statusStatus == 200 {
				fmt.Fprint(w, `{"data":{"quota_per_unit":500000}}`)
			}
		case "/api/log/self/stat":
			start, _ := strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
			now := time.Now().Unix()
			if start > now-15*86400 { // 7d 窗口
				fmt.Fprint(w, `{"data":{"quota":25600000,"rpm":3,"tpm":1200}}`) // 51.20 USD
			} else { // 30d 窗口
				fmt.Fprint(w, `{"data":{"quota":69281250,"rpm":3,"tpm":1200}}`) // 138.56 USD
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func validAcc(base string) domain.Account {
	return domain.Account{
		ID: "n", Provider: "newapi", Label: "x", BaseURL: base,
		AccessTokenEnv: "NEWAPI_AT", UserID: "16002",
	}
}

// TestFetchUsage_NativeGolden 全流程：余额 = 121992688/500000 = 243.99；
// Recent.Window7d = 25600000/500000 = 51.2，Window30d = 69281250/500000 = 138.56。
func TestFetchUsage_NativeGolden(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 200, 200)
	defer srv.Close()

	u, err := New().FetchUsage(context.Background(), validAcc(srv.URL))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(u.Dimensions) != 1 || u.Dimensions[0].Balance != 243.985376 {
		t.Errorf("Balance = %v, want 243.99 (121992688/500000)", u.Dimensions[0].Balance)
	}
	if u.Dimensions[0].Currency != "USD" || u.Dimensions[0].PercentUsed != -1 {
		t.Errorf("dim wrong: %+v", u.Dimensions[0])
	}
	if u.Recent == nil {
		t.Fatal("Recent must not be nil when stat succeeds")
	}
	if u.Recent.Window7d != 51.2 || u.Recent.Window30d != 138.5625 {
		t.Errorf("Recent windows wrong: %+v", u.Recent)
	}
	if u.Recent.RPM != 3 || u.Recent.TPM != 1200 || u.Recent.Currency != "USD" {
		t.Errorf("Recent rpm/tpm/currency wrong: %+v", u.Recent)
	}
}

// TestFetchUsage_QPUFallback status 失败 → 用 defaultQPU(500000) 换算。
func TestFetchUsage_QPUFallback(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 200, 500) // status 500
	defer srv.Close()

	u, err := New().FetchUsage(context.Background(), validAcc(srv.URL))
	if err != nil {
		t.Fatalf("status 500 should fall back to defaultQPU, not error: %v", err)
	}
	if u.Dimensions[0].Balance != 243.985376 { // 仍用 500000
		t.Errorf("Balance = %v, want 243.99 with defaultQPU", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_StatDegraded stat 端点 404 → Recent=nil，余额仍正确。
func TestFetchUsage_StatDegraded(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/self":
			fmt.Fprint(w, `{"data":{"quota":121992688,"used_quota":69281250}}`)
		case "/api/status":
			fmt.Fprint(w, `{"data":{"quota_per_unit":500000}}`)
		case "/api/log/self/stat":
			w.WriteHeader(http.StatusNotFound) // stat 失败
		}
	}))
	defer srv.Close()

	u, err := New().FetchUsage(context.Background(), validAcc(srv.URL))
	if err != nil {
		t.Fatalf("stat 404 must not error: %v", err)
	}
	if u.Recent != nil {
		t.Errorf("Recent must be nil when stat fails, got %+v", u.Recent)
	}
	if u.Dimensions[0].Balance != 243.985376 {
		t.Errorf("Balance still correct despite stat fail: %v", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_UserSelfFails user/self 401 → 报错。
func TestFetchUsage_UserSelfFails(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 401, 200)
	defer srv.Close()

	if _, err := New().FetchUsage(context.Background(), validAcc(srv.URL)); err == nil {
		t.Fatal("user/self failure should error")
	}
}

// TestFetchUsage_MissingCreds 缺 access_token_env 或 user_id → 报错。
func TestFetchUsage_MissingCreds(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 200, 200)
	defer srv.Close()

	for _, acc := range []domain.Account{
		{ID: "n", Provider: "newapi", BaseURL: srv.URL, UserID: "16002"},             // 缺 AccessTokenEnv
		{ID: "n", Provider: "newapi", BaseURL: srv.URL, AccessTokenEnv: "NEWAPI_AT"}, // 缺 UserID
	} {
		if _, err := New().FetchUsage(context.Background(), acc); err == nil {
			t.Fatalf("missing creds should error: %+v", acc)
		}
	}
}

// TestFetchUsage_TokenNotSet access_token_env 配了但环境变量为空 → 报错。
func TestFetchUsage_TokenNotSet(t *testing.T) {
	t.Setenv("NEWAPI_AT", "") // 显式置空
	srv := nativeServer(t, 200, 200)
	defer srv.Close()

	if _, err := New().FetchUsage(context.Background(), validAcc(srv.URL)); err == nil {
		t.Fatal("empty access token should error")
	}
}

// TestFetchUsage_BaseURLRequired 自部署无默认 base。
func TestFetchUsage_BaseURLRequired(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	acc := validAcc("")
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("expected error for missing base_url")
	}
}
