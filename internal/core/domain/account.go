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

// Account 是一个被监控的厂商账号配置。
type Account struct {
	ID       string `yaml:"id"`
	Vendor   string `yaml:"vendor"` // glm | minimax | kimi | ...
	Label    string `yaml:"label"`
	BaseURL  string `yaml:"base_url,omitempty"` // 可选，覆盖默认
	TokenEnv string `yaml:"token_env"`          // 环境变量名，token 从此读
	Pinned   bool   `yaml:"pinned,omitempty"`   // 置顶标记；UI 置顶排序 + 📌 marker
}
