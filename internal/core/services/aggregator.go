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

// Package services 实现 fleetboard 的应用服务层：编排 domain/ports，不依赖任何 adapter。
// Aggregator 是用量聚合服务——并发拉取多账号用量，单点失败不连坐。
package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

// ErrUnknownProvider 表示账号的 Provider 在 ProviderLookup 中找不到对应 adapter。
// 用 sentinel + %w 包装，使调用方可 errors.Is 判定，同时消息里携带具体 provider 名。
var ErrUnknownProvider = errors.New("unknown provider: no usage provider registered")

// Aggregator 聚合多账号用量拉取：按账号的 Provider 字段分派到对应 adapter，
// 并发执行且单点失败不连坐（错误只写回对应 ProviderUsage.Err，不 panic、不阻断其他账号）。
//
// 依赖 ports.ProviderLookup（而非具体 *providers.Registry），保持六边形依赖方向：
// core/services → ports，不反向依赖 adapters/providers。
type Aggregator struct {
	lookup ports.ProviderLookup
}

// NewAggregator 构造一个依赖 ProviderLookup 的聚合器。
// 通常传入 *providers.Registry——其 Get 方法实现了 ProviderLookup。
func NewAggregator(lookup ports.ProviderLookup) *Aggregator {
	return &Aggregator{lookup: lookup}
}

// FetchOne 拉取单个账号的用量（Task 12 的 "r" 选中刷新使用）。
//   - 无 adapter：返回填充了账号元信息、Err=ErrUnknownProvider 的 ProviderUsage（无 Dimensions、Primary=nil）。
//   - 有 adapter：调用其 FetchUsage；即使返回 err != nil，返回的 ProviderUsage 仍被透传
//     （含 provider 已计算的 Dimensions/Primary），UI 据此对失败账号标红但仍展示已有维度。
func (a *Aggregator) FetchOne(ctx context.Context, acc domain.Account) domain.ProviderUsage {
	return a.fetchOne(ctx, acc)
}

// FetchAll 并发拉取多账号用量，结果按输入顺序写回 out[i]（Task 12 的 "R" 全量刷新使用）。
// 每个账号一个 goroutine；单账号失败只进对应 out[i].Err，不连坐其他账号。
// 输入为空时返回长度为 0 的非 nil 切片。
func (a *Aggregator) FetchAll(ctx context.Context, accs []domain.Account) []domain.ProviderUsage {
	out := make([]domain.ProviderUsage, len(accs))
	var wg sync.WaitGroup
	for i, acc := range accs {
		wg.Add(1)
		go func(i int, acc domain.Account) {
			defer wg.Done()
			out[i] = a.fetchOne(ctx, acc)
		}(i, acc)
	}
	wg.Wait()
	return out
}

// fetchOne 是 FetchOne/FetchAll 的共享实现。
func (a *Aggregator) fetchOne(ctx context.Context, acc domain.Account) domain.ProviderUsage {
	p, ok := a.lookup.Get(acc.Provider)
	if !ok {
		// 无 adapter：构造空 u + ErrUnknownProvider（携带 provider 名便于排障）。
		return domain.ProviderUsage{
			AccountID: acc.ID,
			Provider:    acc.Provider,
			Label:     acc.Label,
			FetchedAt: time.Now(),
			Pinned:    acc.Pinned,
			Err:       fmt.Errorf("%w: %q", ErrUnknownProvider, acc.Provider),
		}
	}

	u, err := p.FetchUsage(ctx, acc)
	// 契约（err 透传）：即使 err != nil，provider 返回的 u 仍可能含 Dimensions/Primary
	// （mock 在 err 路径仍 SelectPrimary）。aggregator 必须把 u 存入结果，不能丢弃或跳过。
	// 仅当 provider 未把 err 写入 u.Err 时补齐（防御真实 adapter 返回零值 u 的情况）。
	if err != nil && u.Err == nil {
		u.Err = err
	}
	// Pinned 是 UI 元数据（非接口数据），在此单点从 Account 注入，provider 无需关心。
	u.Pinned = acc.Pinned
	return u
}
