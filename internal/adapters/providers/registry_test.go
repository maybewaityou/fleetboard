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

package providers

import (
	"context"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/adapters/providers/mock"
	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func TestNewRegistryEmpty(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("anything"); ok {
		t.Fatal("empty registry should miss every provider")
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := mock.New("glm", nil, nil)
	r.Register(p)

	got, ok := r.Get("glm")
	if !ok {
		t.Fatal("expected to find registered provider glm")
	}
	if got.Provider() != "glm" {
		t.Fatalf("got provider %q, want glm", got.Provider())
	}
	if got != p {
		t.Fatal("Get should return the exact pointer registered")
	}
}

func TestGetMissingProvider(t *testing.T) {
	r := NewRegistry(mock.New("glm", nil, nil))
	if _, ok := r.Get("minimax"); ok {
		t.Fatal("minimax was never registered, expected ok=false")
	}
}

func TestNewRegistryVariadic(t *testing.T) {
	r := NewRegistry(
		mock.New("glm", nil, nil),
		mock.New("minimax", nil, nil),
		mock.New("kimi", nil, nil),
	)
	for _, v := range []string{"glm", "minimax", "kimi"} {
		if _, ok := r.Get(v); !ok {
			t.Errorf("expected %q to be registered", v)
		}
	}
}

func TestRegisterOverwritesSameProvider(t *testing.T) {
	first := mock.New("glm", []domain.UsageDimension{{Name: "old"}}, nil)
	second := mock.New("glm", []domain.UsageDimension{{Name: "new"}}, nil)
	r := NewRegistry(first)
	r.Register(second)

	got, ok := r.Get("glm")
	if !ok {
		t.Fatal("glm must be present")
	}
	if got != second {
		t.Fatal("Register must overwrite; expected to get the second provider")
	}
	// 原 provider 没有被破坏
	if first.Dims[0].Name != "old" {
		t.Fatalf("first provider should be untouched, got %q", first.Dims[0].Name)
	}
}

// ensure Registry 与 mock 在真实接口契约下能联动：调用 FetchUsage 不出错。
func TestRegistryReturnsUsableProvider(t *testing.T) {
	r := NewRegistry(mock.New("glm", []domain.UsageDimension{
		{Name: "5h", PercentUsed: 30},
		{Name: "weekly", PercentUsed: 88},
	}, nil))

	p, ok := r.Get("glm")
	if !ok {
		t.Fatal("glm missing")
	}
	u, err := p.FetchUsage(context.Background(), domain.Account{ID: "a1", Provider: "glm", Label: "main"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.Primary == nil || u.Primary.Name != "weekly" {
		t.Fatalf("SelectPrimary should have run via interface, got %+v", u.Primary)
	}
}
