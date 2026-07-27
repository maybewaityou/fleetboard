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

package ui

import (
	"strings"
	"sync/atomic"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// presetColors 把 YAML 里的预设色名映射到 const.go 的 Tokyo Night 调色板。
// 大小写不敏感；未列出的名字按非法处理（回退该档默认）。
var presetColors = map[string]string{
	"green":     colorGreen,
	"yellow":    colorYellow,
	"red":       colorRed,
	"gray":      colorGray,
	"grey":      colorGray,
	"purple":    colorPurple,
	"cyan":      colorCyan,
	"blue":      colorAccent,
	"accent":    colorAccent,
	"primary":   colorPrimary,
	"secondary": colorSecondary,
}

// colorScheme 是解析后的活动配色（颜色项已展开为 #RRGGBB 或调色板常量），
// 供 StatusColor / BalanceColor 读取。
type colorScheme struct {
	quota, balance domain.ThresholdColors
}

func defaultQuota() domain.ThresholdColors {
	return domain.ThresholdColors{
		// 91（不含）作 yellow 上界，精确复现旧 StatusColor 契约：
		// 「90 是最后 yellow，91 是首个 red」。左闭右开分桶（pickByQuota 用 pct<t）。
		Thresholds: []float64{70, 91},
		Colors:     []string{colorGreen, colorYellow, colorRed},
	}
}

func defaultBalance() domain.ThresholdColors {
	return domain.ThresholdColors{
		Thresholds: []float64{10, 1},
		Colors:     []string{colorGreen, colorYellow, colorRed},
	}
}

func defaultScheme() *colorScheme {
	return &colorScheme{quota: defaultQuota(), balance: defaultBalance()}
}

// activeColors 是进程级活动配色。用 atomic.Pointer 保证并发读（t.Parallel 测试 +
// 主循环渲染）与启动期单次写之间无 data race。
var activeColors atomic.Pointer[colorScheme]

func init() {
	activeColors.Store(defaultScheme())
}

// applyColorScheme 解析并安装用户配色；任一档非法则该档回退默认。main 启动期调用一次。
func applyColorScheme(cfg domain.ColorsConfig) {
	activeColors.Store(resolveColors(cfg))
}

// resolveColors 把用户配置解析为 colorScheme；非法档回退默认。
func resolveColors(cfg domain.ColorsConfig) *colorScheme {
	s := defaultScheme()
	if q, ok := resolveThresholds(cfg.Quota); ok {
		s.quota = q
	}
	if b, ok := resolveThresholds(cfg.Balance); ok {
		s.balance = b
	}
	return s
}

// resolveThresholds 校验一档：thresholds 非空、colors 数 == thresholds+1、每色合法。
// 通过则返回颜色已解析的副本；否则 ok=false（调用方回退默认）。
func resolveThresholds(tc domain.ThresholdColors) (domain.ThresholdColors, bool) {
	if len(tc.Thresholds) == 0 || len(tc.Colors) != len(tc.Thresholds)+1 {
		return domain.ThresholdColors{}, false
	}
	cols := make([]string, len(tc.Colors))
	for i, c := range tc.Colors {
		resolved, ok := resolveColor(c)
		if !ok {
			return domain.ThresholdColors{}, false
		}
		cols[i] = resolved
	}
	return domain.ThresholdColors{Thresholds: tc.Thresholds, Colors: cols}, true
}

// resolveColor 解析单个颜色项：预设名（大小写不敏感）或 #RRGGBB。
func resolveColor(name string) (string, bool) {
	if c, ok := presetColors[strings.ToLower(name)]; ok {
		return c, true
	}
	if isHexColor(name) {
		return name, true
	}
	return "", false
}

// isHexColor 校验 #RRGGBB（7 字符，#开头，后 6 位十六进制）。
func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// pickByQuota 配额型选色（thresholds 升序）：首个 threshold > pct 命中即返回对应色，
// 都未超过返回末尾兜底色。调用方需先处理 pct<0（N/A 灰）。
func pickByQuota(tc domain.ThresholdColors, pct float64) string {
	for i, t := range tc.Thresholds {
		if pct < t {
			return tc.Colors[i]
		}
	}
	return tc.Colors[len(tc.Colors)-1]
}

// pickByBalance 余额型选色（thresholds 降序）：首个 threshold <= balance 命中即返回对应色，
// 都低于返回末尾兜底色（含负值场景）。
func pickByBalance(tc domain.ThresholdColors, balance float64) string {
	for i, t := range tc.Thresholds {
		if balance >= t {
			return tc.Colors[i]
		}
	}
	return tc.Colors[len(tc.Colors)-1]
}
