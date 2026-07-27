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

// Package providers 提供厂商适配器的注册表（Registry）与公共工具。
// Registry 是 provider -> ports.UsageProvider 的查表结构，由 main 装配，
// 供 service 层按账号的 Provider 字段取出对应实现。
package providers

import "github.com/maybewaityou/fleetboard/internal/core/ports"

// Registry 按 provider 名聚合 UsageProvider。
// 零值不可用，请用 NewRegistry 构造。
type Registry struct {
	byProvider map[string]ports.UsageProvider
}

// NewRegistry 建立注册表并可选地立即登记一批 provider。
// 重复 provider 的后者覆盖前者（与 Register 一致）。
func NewRegistry(ps ...ports.UsageProvider) *Registry {
	r := &Registry{byProvider: map[string]ports.UsageProvider{}}
	for _, p := range ps {
		r.Register(p)
	}
	return r
}

// Register 登记一个 provider，键为其 Provider()。覆盖同名的旧实现。
func (r *Registry) Register(p ports.UsageProvider) {
	r.byProvider[p.Provider()] = p
}

// Get 按 provider 取出 provider；未登记时 ok=false。
func (r *Registry) Get(provider string) (ports.UsageProvider, bool) {
	p, ok := r.byProvider[provider]
	return p, ok
}
