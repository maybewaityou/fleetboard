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

// Package mock 提供一个 ports.UsageProvider 的可编程假实现，用于 service/UI 测试。
// 通过预设 Dimensions 与 Err，可在不触网的情况下驱动上层逻辑。
package mock

import (
	"context"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// Provider 是 UsageProvider 的 mock 实现。
// 字段直接暴露以便测试断言/调整。
type Provider struct {
	ProviderName string
	Dims         []domain.UsageDimension
	Err          error
	// FetchCount 记录 FetchUsage 被调用的次数，便于上层断言（可选）。
	FetchCount int
}

// New 构造一个 mock provider。
//   - provider：写入 ProviderName，对应 domain.Account.Provider
//   - dims：每次 FetchUsage 回放的维度切片（原样返回，会被 SelectPrimary 处理）
//   - err：FetchUsage 的返回错误；非 nil 时 ProviderUsage 仍被填充并计算 Primary
func New(provider string, dims []domain.UsageDimension, err error) *Provider {
	return &Provider{
		ProviderName: provider,
		Dims:         dims,
		Err:          err,
	}
}

// Provider 返回厂商标识，实现 ports.UsageProvider。
func (p *Provider) Provider() string { return p.ProviderName }

// FetchUsage 回放预设的 Dims 与 Err，并调用 SelectPrimary。
// 即使 Err != nil，ProviderUsage 也会被填充，便于上层在错误场景下展示局部信息。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	p.FetchCount++
	u := domain.ProviderUsage{
		AccountID:  acc.ID,
		Provider:   acc.Provider,
		Label:      acc.Label,
		Dimensions: p.Dims,
		FetchedAt:  time.Now(),
		Err:        p.Err,
	}
	u.SelectPrimary()
	return u, p.Err
}
