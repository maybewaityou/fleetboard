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

// sort.go 定义列表排序模式，移植自 lazytmux sort.go：一个小枚举，由 s/S 键
// 循环切换；置顶账号始终浮顶。比较器复用 account_list.displayPercent，保证
// 排序键与列表展示的是同一档（最近重置窗口）。
package ui

import (
	"sort"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// SortMode 是列表的排序模式。
type SortMode int

const (
	SortByNameAsc SortMode = iota // Name ↑
	SortByUsageDesc               // Usage % ↓
	SortByRefreshedDesc           // Last Refreshed ↓
)

// String 返回展示标签（列表标题与排序 toast 共用）。
func (s SortMode) String() string {
	switch s {
	case SortByUsageDesc:
		return "Usage ↓"
	case SortByRefreshedDesc:
		return "Refreshed ↓"
	default:
		return "Name ↑"
	}
}

// Next 前进一步，末尾回绕。
func (s SortMode) Next() SortMode { return (s + 1) % 3 }

// sortUsagesForUI 原地稳定排序：置顶优先，组内按 mode。稳定排序让等键行保持原序。
func sortUsagesForUI(usages []domain.ProviderUsage, mode SortMode) {
	sort.SliceStable(usages, func(i, j int) bool {
		if usages[i].Pinned != usages[j].Pinned {
			return usages[i].Pinned
		}
		switch mode {
		case SortByUsageDesc:
			return displayPercent(usages[i]) > displayPercent(usages[j])
		case SortByRefreshedDesc:
			return usages[i].FetchedAt.After(usages[j].FetchedAt)
		default:
			return usages[i].Label < usages[j].Label
		}
	})
}
