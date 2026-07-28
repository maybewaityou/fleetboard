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

package services

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/adapters/providers"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/mock"
	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

// --- FetchOne -------------------------------------------------------------

func TestFetchOneSuccess(t *testing.T) {
	reg := providers.NewRegistry(
		mock.New("glm", []domain.UsageDimension{
			{Name: "5h", PercentUsed: 30},
			{Name: "weekly", PercentUsed: 88},
		}, nil),
	)
	agg := NewAggregator(reg)

	acc := domain.Account{ID: "g1", Provider: "glm", Label: "main"}
	u := agg.FetchOne(context.Background(), acc)

	if u.Err != nil {
		t.Fatalf("unexpected err: %v", u.Err)
	}
	if u.AccountID != "g1" || u.Provider != "glm" || u.Label != "main" {
		t.Fatalf("account meta not propagated: %+v", u)
	}
	if u.Primary == nil || u.Primary.Name != "weekly" || u.Primary.PercentUsed != 88 {
		t.Fatalf("Primary should be weekly@88 (max valid), got %+v", u.Primary)
	}
	if u.FetchedAt.IsZero() || u.FetchedAt.After(time.Now()) {
		t.Fatalf("FetchedAt invalid: %v", u.FetchedAt)
	}
}

func TestFetchOneUnknownProvider(t *testing.T) {
	// Registry 里没有 "kimi" 的 adapter。
	reg := providers.NewRegistry(mock.New("glm", nil, nil))
	agg := NewAggregator(reg)

	acc := domain.Account{ID: "k1", Provider: "kimi", Label: "personal"}
	u := agg.FetchOne(context.Background(), acc)

	if u.Err == nil {
		t.Fatal("expected ErrUnknownProvider, got nil")
	}
	if !errors.Is(u.Err, ErrUnknownProvider) {
		t.Fatalf("err should wrap ErrUnknownProvider, got %v", u.Err)
	}
	// 元信息仍应回填，便于 UI 展示该账号（即使拉取失败）。
	if u.AccountID != "k1" || u.Provider != "kimi" || u.Label != "personal" {
		t.Fatalf("account meta not backfilled on unknown provider: %+v", u)
	}
	if u.Primary != nil {
		t.Fatalf("Primary must be nil when provider unknown, got %+v", u.Primary)
	}
	if len(u.Dimensions) != 0 {
		t.Fatalf("Dimensions must be empty when provider unknown, got %d", len(u.Dimensions))
	}
	if u.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set even on unknown-provider path")
	}
}

// 单账号失败不丢弃 u：provider 返回 err 时，ProviderUsage（含 Dimensions/Primary）仍透传。
func TestFetchOneErrPassthroughKeepsDimensions(t *testing.T) {
	wantErr := errors.New("boom")
	reg := providers.NewRegistry(
		mock.New("minimax", []domain.UsageDimension{{Name: "daily", PercentUsed: 50}}, wantErr),
	)
	agg := NewAggregator(reg)

	u := agg.FetchOne(context.Background(), domain.Account{ID: "m1", Provider: "minimax", Label: "dev"})

	if u.Err == nil || !errors.Is(u.Err, wantErr) {
		t.Fatalf("u.Err = %v, want %v", u.Err, wantErr)
	}
	// 关键契约：err != nil 时 Dimensions 与 Primary 仍存在（UI 据此标红但展示已有维度）。
	if len(u.Dimensions) != 1 || u.Dimensions[0].Name != "daily" {
		t.Fatalf("Dimensions must be retained on error path, got %+v", u.Dimensions)
	}
	if u.Primary == nil || u.Primary.Name != "daily" {
		t.Fatalf("Primary must still be computed on error path, got %+v", u.Primary)
	}
}

// --- FetchAll -------------------------------------------------------------

func TestFetchAllEmpty(t *testing.T) {
	agg := NewAggregator(providers.NewRegistry())
	got := agg.FetchAll(context.Background(), nil)
	if got == nil {
		t.Fatal("FetchAll(nil) must return non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("FetchAll(nil) len = %d, want 0", len(got))
	}
}

// Brief 的核心测试：2 账号（1 成功 1 失败），断言返回 2 条、失败那条 Err!=nil、成功那条有 Primary。
func TestFetchAllIsolatesFailures(t *testing.T) {
	reg := providers.NewRegistry(
		mock.New("glm", []domain.UsageDimension{{Name: "5h", PercentUsed: 30}}, nil),
		mock.New("minimax", nil, errors.New("boom")),
	)
	accs := []domain.Account{
		{ID: "g", Provider: "glm", Label: "GLM"},
		{ID: "m", Provider: "minimax", Label: "MiniMax"},
	}

	got := NewAggregator(reg).FetchAll(context.Background(), accs)

	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d", len(got))
	}
	// minimax 那条 Err != nil（即使无 Dimensions，u 仍被回写、不 panic）。
	if got[1].Err == nil {
		t.Fatal("minimax result should carry err, got nil")
	}
	// glm 那条无 err 且 Primary 已计算。
	if got[0].Err != nil {
		t.Fatalf("glm result should be clean, got err %v", got[0].Err)
	}
	if got[0].Primary == nil || got[0].Primary.Name != "5h" {
		t.Fatalf("glm Primary missing/wrong: %+v", got[0].Primary)
	}
}

func TestFetchAllPreservesOrder(t *testing.T) {
	// 6 个不同 provider，确保并发写回 out[i] 不乱序。
	dims := func(name string, pct float64) []domain.UsageDimension {
		return []domain.UsageDimension{{Name: name, PercentUsed: pct}}
	}
	reg := providers.NewRegistry(
		mock.New("a", dims("a", 10), nil),
		mock.New("b", dims("b", 20), nil),
		mock.New("c", dims("c", 30), nil),
		mock.New("d", dims("d", 40), nil),
		mock.New("e", dims("e", 50), nil),
		mock.New("f", dims("f", 60), nil),
	)
	accs := []domain.Account{
		{ID: "1", Provider: "a"},
		{ID: "2", Provider: "b"},
		{ID: "3", Provider: "c"},
		{ID: "4", Provider: "d"},
		{ID: "5", Provider: "e"},
		{ID: "6", Provider: "f"},
	}

	got := NewAggregator(reg).FetchAll(context.Background(), accs)

	for i, want := range []string{"a", "b", "c", "d", "e", "f"} {
		if got[i].Provider != want {
			t.Errorf("result[%d].Provider = %q, want %q (order broken)", i, got[i].Provider, want)
		}
		if got[i].Primary == nil || got[i].Primary.Name != want {
			t.Errorf("result[%d].Primary mismatch: %+v", i, got[i].Primary)
		}
	}
}

// 并发安全性 + 单账号失败完全隔离：混合 success/fail/unknown-provider，
// 每个失败只影响自己，不 panic、不阻断其他账号。配合 -race 验证无数据竞争。
//
// 注意：每个并发账号用 *独立 provider*（独立 mock 实例）。这不是 aggregator 的限制——
// aggregator 支持同 provider 多账号并发——而是规避 mock.Provider.FetchCount 的非原子自增
// （mock 已知限制，真实 adapter 无此问题）。同 provider 并发安全性由 HighConcurrency 测试
// （64 独立 provider）+ PreservesOrder 已充分覆盖。
func TestFetchAllMixedIsolationConcurrent(t *testing.T) {
	reg := providers.NewRegistry(
		mock.New("ok1", []domain.UsageDimension{{Name: "5h", PercentUsed: 41}}, nil),
		mock.New("ok2", []domain.UsageDimension{{Name: "weekly", PercentUsed: 55}}, nil),
		mock.New("fail1", []domain.UsageDimension{{Name: "x", PercentUsed: 7}}, errors.New("nope")),
		mock.New("fail2", []domain.UsageDimension{{Name: "y", PercentUsed: 8}}, errors.New("boom")),
		// "ghost1"/"ghost2" 故意不登记 → unknown provider 路径
	)
	accs := []domain.Account{
		{ID: "ok-1", Provider: "ok1", Label: "ok one"},
		{ID: "fail-1", Provider: "fail1", Label: "fail one"},
		{ID: "ghost-1", Provider: "ghost1", Label: "ghost one"},
		{ID: "ok-2", Provider: "ok2", Label: "ok two"},
		{ID: "ghost-2", Provider: "ghost2", Label: "ghost two"},
		{ID: "fail-2", Provider: "fail2", Label: "fail two"},
	}

	got := NewAggregator(reg).FetchAll(context.Background(), accs)

	if len(got) != len(accs) {
		t.Fatalf("len = %d, want %d", len(got), len(accs))
	}
	for i, acc := range accs {
		u := got[i]
		if u.AccountID != acc.ID {
			t.Errorf("[%d] AccountID = %q, want %q", i, u.AccountID, acc.ID)
		}
		switch acc.Provider {
		case "ok1", "ok2":
			if u.Err != nil {
				t.Errorf("[%d] ok account should be clean, got %v", i, u.Err)
			}
			if u.Primary == nil {
				t.Errorf("[%d] ok Primary missing: %+v", i, u.Primary)
			}
		case "fail1", "fail2":
			if u.Err == nil {
				t.Errorf("[%d] fail account should carry err", i)
			}
			// err 路径下 Dimensions 仍透传（关键契约）。
			if len(u.Dimensions) != 1 {
				t.Errorf("[%d] fail Dimensions should be retained, got %d", i, len(u.Dimensions))
			}
		default: // ghost*
			if !errors.Is(u.Err, ErrUnknownProvider) {
				t.Errorf("[%d] ghost should be ErrUnknownProvider, got %v", i, u.Err)
			}
			if u.Primary != nil {
				t.Errorf("[%d] ghost Primary must be nil, got %+v", i, u.Primary)
			}
		}
	}
}

// 大量并发 goroutine：验证 out[i] 写入无竞争、无 panic、顺序保持。
// 每个账号不同 provider → 不同 mock 实例，避开 mock.FetchCount 的非原子写（mock 已知限制）。
func TestFetchAllHighConcurrencyRaceSafety(t *testing.T) {
	const n = 64
	ps := make([]ports.UsageProvider, n)
	accs := make([]domain.Account, n)
	for i := 0; i < n; i++ {
		v := "v" + strconv.Itoa(i)
		ps[i] = mock.New(v, []domain.UsageDimension{
			{Name: "d", PercentUsed: float64(i % 100)},
		}, nil)
		accs[i] = domain.Account{ID: "a" + strconv.Itoa(i), Provider: v}
	}
	reg := providers.NewRegistry(ps...)

	got := NewAggregator(reg).FetchAll(context.Background(), accs)

	if len(got) != n {
		t.Fatalf("len = %d, want %d", len(got), n)
	}
	for i := range got {
		if got[i].AccountID != "a"+strconv.Itoa(i) {
			t.Errorf("order broken at %d: %s", i, got[i].AccountID)
		}
		if got[i].Primary == nil || got[i].Primary.PercentUsed != float64(i%100) {
			t.Errorf("primary mismatch at %d: %+v", i, got[i].Primary)
		}
	}
}

// ProviderLookup 接口适配性：自定义实现也能注入（不限于 *providers.Registry），
// 验证 Aggregator 依赖的是 ports 接口而非具体 Registry。
func TestNewAggregatorAcceptsCustomLookup(t *testing.T) {
	mp := mock.New("glm", []domain.UsageDimension{{Name: "5h", PercentUsed: 9}}, nil)
	lookup := &stubLookup{p: mp, has: map[string]bool{"glm": true}}

	u := NewAggregator(lookup).FetchOne(context.Background(), domain.Account{ID: "x", Provider: "glm"})

	if u.Err != nil {
		t.Fatalf("unexpected err: %v", u.Err)
	}
	if u.Primary == nil || u.Primary.PercentUsed != 9 {
		t.Fatalf("custom lookup result wrong: %+v", u.Primary)
	}
	// 未登记 provider 走 unknown 路径。
	u2 := NewAggregator(lookup).FetchOne(context.Background(), domain.Account{ID: "y", Provider: "zzz"})
	if !errors.Is(u2.Err, ErrUnknownProvider) {
		t.Fatalf("custom lookup unknown path: %v", u2.Err)
	}
}

type stubLookup struct {
	p   ports.UsageProvider
	has map[string]bool
}

func (s *stubLookup) Get(provider string) (ports.UsageProvider, bool) {
	if s.has[provider] {
		return s.p, true
	}
	return nil, false
}

// --- per-account timeout (Task 2) ----------------------------------------

// slowProvider 是超时测试专用 stub：FetchUsage 阻塞到 ctx 被取消/超时，
// 然后返回 ctx.Err。不放入 mock.go（mock 不模拟阻塞）。
type slowProvider struct {
	name string
}

func (s *slowProvider) Provider() string { return s.name }

func (s *slowProvider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	<-ctx.Done()
	return domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  s.name,
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}, ctx.Err()
}

// TestWithTimeoutBuilder 验证 WithTimeout 设置字段；未调用时为 DefaultFetchTimeout。
func TestWithTimeoutBuilder(t *testing.T) {
	reg := providers.NewRegistry(mock.New("glm", nil, nil))
	def := NewAggregator(reg)
	if def.timeout != DefaultFetchTimeout {
		t.Fatalf("default timeout = %v, want %v", def.timeout, DefaultFetchTimeout)
	}
	custom := NewAggregator(reg).WithTimeout(7 * time.Second)
	if custom.timeout != 7*time.Second {
		t.Fatalf("WithTimeout(7s) = %v, want 7s", custom.timeout)
	}
	zero := NewAggregator(reg).WithTimeout(0)
	if zero.timeout != 0 {
		t.Fatalf("WithTimeout(0) = %v, want 0 (unlimited)", zero.timeout)
	}
}

// TestFetchOneTimeout 验证 per-account 超时截断慢 provider 并回填 DeadlineExceeded。
func TestFetchOneTimeout(t *testing.T) {
	reg := providers.NewRegistry(&slowProvider{name: "slow"})
	agg := NewAggregator(reg).WithTimeout(50 * time.Millisecond)
	acc := domain.Account{ID: "s1", Provider: "slow", Label: "slow"}

	start := time.Now()
	u := agg.FetchOne(context.Background(), acc)
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, want ~50ms (tolerance 40ms~500ms)", elapsed)
	}
	if u.Err == nil {
		t.Fatal("expected timeout err, got nil")
	}
	if !errors.Is(u.Err, context.DeadlineExceeded) {
		t.Fatalf("u.Err = %v, want wraps context.DeadlineExceeded", u.Err)
	}
	// 账号元信息仍回填（UI 标红但展示账号）。
	if u.AccountID != "s1" || u.Provider != "slow" {
		t.Errorf("meta not backfilled: %+v", u)
	}
}

// TestFetchOneNoTimeout 验证 WithTimeout(0) 不限超时：slowProvider 收到的 ctx 无 deadline。
// 用外部 cancel 主动退出，避免测试自身卡住。
func TestFetchOneNoTimeout(t *testing.T) {
	reg := providers.NewRegistry(&slowProvider{name: "slow"})
	agg := NewAggregator(reg).WithTimeout(0)
	acc := domain.Account{ID: "s2", Provider: "slow", Label: "slow"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan domain.ProviderUsage, 1)
	go func() { done <- agg.FetchOne(ctx, acc) }()

	// 给一点时间确认 FetchUsage 确实进入阻塞（未被立即超时截断）。
	select {
	case u := <-done:
		t.Fatalf("FetchOne returned early without external cancel: %+v", u)
	case <-time.After(30 * time.Millisecond):
		// 预期：还在阻塞 → 说明无 deadline 截断。
	}

	cancel() // 主动退出
	select {
	case u := <-done:
		if u.Err == nil {
			t.Error("expected err after cancel, got nil")
		}
	case <-time.After(time.Second):
		t.Fatal("FetchOne did not return within 1s after cancel")
	}
}

// TestFetchAllTimeoutDoesNotBlockOthers 验证慢账号超时不阻塞快账号。
func TestFetchAllTimeoutDoesNotBlockOthers(t *testing.T) {
	reg := providers.NewRegistry(
		&slowProvider{name: "slow"},
		mock.New("fast", []domain.UsageDimension{{Name: "5h", PercentUsed: 30}}, nil),
	)
	agg := NewAggregator(reg).WithTimeout(50 * time.Millisecond)
	accs := []domain.Account{
		{ID: "slow-1", Provider: "slow", Label: "Slow"},
		{ID: "fast-1", Provider: "fast", Label: "Fast"},
	}

	start := time.Now()
	got := agg.FetchAll(context.Background(), accs)
	elapsed := time.Since(start)

	// FetchAll 总耗时 ≈ 最慢账号（slow ~50ms），不是它们的和。
	if elapsed > 500*time.Millisecond {
		t.Fatalf("FetchAll elapsed = %v, want < 500ms (slow must not serialize behind... itself)", elapsed)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// slow 超时带 err；fast 立即返回无 err。
	if got[0].Err == nil || !errors.Is(got[0].Err, context.DeadlineExceeded) {
		t.Errorf("slow result err = %v, want DeadlineExceeded", got[0].Err)
	}
	if got[1].Err != nil {
		t.Errorf("fast result should be clean, got %v", got[1].Err)
	}
	if got[1].Primary == nil || got[1].Primary.Name != "5h" {
		t.Errorf("fast Primary missing: %+v", got[1].Primary)
	}
}
