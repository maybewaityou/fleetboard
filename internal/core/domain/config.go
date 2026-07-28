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
	Timeout  string `yaml:"timeout"`  // "15s"；空/非法→默认 15s（aggregator per-account 兜底超时）
}

// UIConfig 控制 TUI 表现层。
type UIConfig struct {
	Theme  string       `yaml:"theme"`  // tokyo-night
	Colors ColorsConfig `yaml:"colors"` // 颜色阈值；零值→代码默认
}

// ColorsConfig 持有配额型与余额型两套颜色阈值。
type ColorsConfig struct {
	Quota   ThresholdColors `yaml:"quota"`   // 配额型（百分比，升序阈值）
	Balance ThresholdColors `yaml:"balance"` // 余额型（数值，降序阈值；支持负值）
}

// ThresholdColors：thresholds 为边界数组，colors 比 thresholds 多 1 个（末尾兜底）。
// 配额型 thresholds 升序、余额型降序；方向由选色函数（pickByQuota/pickByBalance）决定。
type ThresholdColors struct {
	Thresholds []float64 `yaml:"thresholds"`
	Colors     []string  `yaml:"colors"` // 预设名或 #RRGGBB
}
