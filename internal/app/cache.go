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

// Package app 存放 fleetboard 的运行时状态，与 cmd 装配分离，便于独立测试。
package app

import (
	"sync"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// Cache 是最近一次拉取的 per-account 用量快照，并发安全。
// r/R 回调与 CRUD 回调都写穿它，TUI 从 Snapshot 取只读副本渲染。
type Cache struct {
	mu      sync.Mutex
	current []domain.ProviderUsage
}

// NewCache 构造一个空 Cache。
func NewCache() *Cache { return &Cache{} }

// ReplaceAll 替换整个数据集（R / boot / CRUD 后用）。拷贝入参 slice，
// 使 cache 持有独立副本——caller 传入后可安全保留或修改原 slice。
// 注意仅做一层浅拷贝：嵌套的 Dimensions 等切片/指针与 caller 共享，需按只读对待。
func (c *Cache) ReplaceAll(usages []domain.ProviderUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = make([]domain.ProviderUsage, len(usages))
	copy(c.current, usages)
}

// Snapshot 返回当前数据集的浅拷贝；调用方独占返回切片，
// 后台 tick 不会改动 TUI 正在绘制的快照。
// 注意仅做一层浅拷贝：嵌套的 Dimensions 等切片/指针与 cache 共享，需按只读对待。
func (c *Cache) Snapshot() []domain.ProviderUsage {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]domain.ProviderUsage, len(c.current))
	copy(out, c.current)
	return out
}

// UpdateOne 按 AccountID 替换单条（r 刷新选中用）；不存在则防御性追加。
func (c *Cache) UpdateOne(u domain.ProviderUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.current {
		if c.current[i].AccountID == u.AccountID {
			c.current[i] = u
			return
		}
	}
	c.current = append(c.current, u)
}

// SetPinned 设置 id 的 Pinned，不重新拉取（仅元数据变更）。
func (c *Cache) SetPinned(id string, pinned bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.current {
		if c.current[i].AccountID == id {
			c.current[i].Pinned = pinned
			return
		}
	}
}

// FindAccount 按 id 在 accs 中查账号配置（FetchOne 需要 provider/token_env/base_url）。
func FindAccount(accs []domain.Account, id string) (domain.Account, bool) {
	for _, a := range accs {
		if a.ID == id {
			return a, true
		}
	}
	return domain.Account{}, false
}

// RemoveAccounts 返回不含 id 的新切片（不改原切片），供删除账号使用。
func RemoveAccounts(accs []domain.Account, id string) []domain.Account {
	out := make([]domain.Account, 0, len(accs))
	for _, a := range accs {
		if a.ID != id {
			out = append(out, a)
		}
	}
	return out
}
