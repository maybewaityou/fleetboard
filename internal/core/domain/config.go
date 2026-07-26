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

// Config 是 fleetboard 的顶层配置（accounts.yaml 反序列化目标）。
type Config struct {
	Accounts []Account     `yaml:"accounts"`
	Refresh  RefreshConfig `yaml:"refresh"`
	UI       UIConfig      `yaml:"ui"`
}

// RefreshConfig 控制定时刷新行为。
type RefreshConfig struct {
	OnStart  bool   `yaml:"on_start"`
	Interval string `yaml:"interval"` // "5m"
}

// UIConfig 控制 TUI 表现层。
type UIConfig struct {
	Theme string `yaml:"theme"` // tokyo-night
}
