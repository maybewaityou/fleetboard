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

package ports

import "github.com/maybewaityou/fleetboard/internal/core/domain"

// View 是 TUI 抽象（便于测试 service 时不拉起真实 tview）。
type View interface {
	// Run 阻塞运行 TUI 主循环，直到用户退出。
	Run() error
	// Render 将一次刷新的全部厂商用量渲染到界面上。
	Render(usages []domain.ProviderUsage)
}
