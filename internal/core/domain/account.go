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

package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateAccountID 生成 12 字符 hex 随机 id（crypto/rand，6 字节）。
// 新增账号时由 cmd/main 自动生成稳定唯一标识，不依赖用户输入；用户在 UI
// 中既看不到也不编辑它。crypto/rand 失败几乎不可能（仅 OS 熵池枯竭），故
// panic 而非返回 error——调用方无法有意义地恢复。
func GenerateAccountID() string {
	b := make([]byte, 6) // 6 bytes → 12 hex chars
	if _, err := rand.Read(b); err != nil {
		panic("domain: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Account 是一个被监控的厂商账号配置。
type Account struct {
	ID       string `yaml:"id"`
	Provider   string `yaml:"provider"` // glm | minimax | kimi | ...
	Label    string `yaml:"label"`
	BaseURL  string `yaml:"base_url,omitempty"` // 可选，覆盖默认
	TokenEnv string `yaml:"token_env"`          // 环境变量名，token 从此读
	Pinned   bool   `yaml:"pinned,omitempty"`   // 置顶标记；UI 置顶排序 + 📌 marker
}
