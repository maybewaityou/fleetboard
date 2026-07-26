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

// ConfigStore 负责配置的持久化读写（accounts.yaml 的加载/落盘）。
type ConfigStore interface {
	// Load 从底层存储（默认 ~/.fleetboard/accounts.yaml）读取并反序列化配置。
	Load() (domain.Config, error)
	// Save 将配置序列化写回存储。
	Save(domain.Config) error
}
