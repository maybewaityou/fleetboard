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
