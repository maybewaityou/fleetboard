// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS BASIS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func TestProviderReturnsName(t *testing.T) {
	p := New("glm", nil, nil)
	if got := p.Provider(); got != "glm" {
		t.Fatalf("Provider() = %q, want glm", got)
	}
}

func TestFetchUsageSelectsPrimary(t *testing.T) {
	dims := []domain.UsageDimension{
		{Name: "5h", PercentUsed: 30},
		{Name: "weekly", PercentUsed: 88},
		{Name: "mcp", PercentUsed: -1}, // N/A, skipped
	}
	p := New("glm", dims, nil)

	acc := domain.Account{ID: "acc-1", Provider: "glm", Label: "main"}
	u, err := p.FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.Primary == nil || u.Primary.Name != "weekly" {
		t.Fatalf("Primary should be weekly (max valid PercentUsed), got %+v", u.Primary)
	}
	if u.Primary.PercentUsed != 88 {
		t.Fatalf("Primary.PercentUsed = %v, want 88", u.Primary.PercentUsed)
	}
}

func TestFetchUsageErrPassthrough(t *testing.T) {
	wantErr := errors.New("boom")
	dims := []domain.UsageDimension{{Name: "5h", PercentUsed: 50}}
	p := New("glm", dims, wantErr)

	u, err := p.FetchUsage(context.Background(), domain.Account{ID: "a", Provider: "glm", Label: "l"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	// 错误路径下 ProviderUsage 仍应被填充并计算 Primary，便于上层展示局部信息
	if u.Err == nil || !errors.Is(u.Err, wantErr) {
		t.Fatalf("u.Err = %v, want %v", u.Err, wantErr)
	}
	if u.Primary == nil || u.Primary.Name != "5h" {
		t.Fatalf("Primary should still be computed on error path, got %+v", u.Primary)
	}
}

func TestFetchUsageFillsAccountFields(t *testing.T) {
	p := New("minimax", []domain.UsageDimension{{Name: "daily", PercentUsed: 10}}, nil)
	acc := domain.Account{ID: "acc-9", Provider: "minimax", Label: "prod"}
	u, _ := p.FetchUsage(context.Background(), acc)

	if u.AccountID != "acc-9" {
		t.Errorf("AccountID = %q, want acc-9", u.AccountID)
	}
	if u.Provider != "minimax" {
		t.Errorf("Provider = %q, want minimax", u.Provider)
	}
	if u.Label != "prod" {
		t.Errorf("Label = %q, want prod", u.Label)
	}
	if u.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set to time.Now()")
	}
	if u.FetchedAt.After(time.Now()) {
		t.Error("FetchedAt must not be in the future")
	}
}

func TestFetchUsageIncrementsCount(t *testing.T) {
	p := New("glm", nil, nil)
	for i := 0; i < 3; i++ {
		if _, _ = p.FetchUsage(context.Background(), domain.Account{}); p.FetchCount != i+1 {
			t.Fatalf("after call %d: FetchCount = %d, want %d", i+1, p.FetchCount, i+1)
		}
	}
}

func TestFetchUsageAllInvalidPrimaryNil(t *testing.T) {
	p := New("glm", []domain.UsageDimension{
		{Name: "5h", PercentUsed: -1},
		{Name: "monthly", PercentUsed: -1},
	}, nil)
	u, _ := p.FetchUsage(context.Background(), domain.Account{})
	if u.Primary != nil {
		t.Fatalf("Primary should be nil when all dims are N/A, got %+v", u.Primary)
	}
}
