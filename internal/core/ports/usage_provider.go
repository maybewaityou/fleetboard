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

import (
	"context"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// UsageProvider 是单个厂商的用量查询适配器。
// 每个具体厂商（GLM / MiniMax / Kimi …）实现此接口，由 service 层注入。
type UsageProvider interface {
	// Vendor 返回厂商标识，对应 domain.Account.Vendor（如 "glm"）。
	Vendor() string
	// FetchUsage 拉取该账号当前用量；返回带多维度信息的 VendorUsage。
	FetchUsage(ctx context.Context, acc domain.Account) (domain.VendorUsage, error)
}
