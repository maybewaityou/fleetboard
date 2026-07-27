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
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// TestChipRendersBold 是渲染回归测试：直接读 tcell.Style 的 AttrBold 位，而不是
// 断言颜色标记字符串的字面量。带背景的彩色 chip 必须写成 [black:bg:b]（2 冒号、
// fg:bg:flags 三段）；若误写成 [black:bg::b]，会变成 3 冒号 4 段，tview 解析时把
// flags 段挤成空串，bold 被静默丢弃——只断言字符串的测试无法发现这一点，本测试可以。
//
// 历史上 provider chip 与 makeTag 都因写成 [black:bg::b] 而从未真正加粗。
func TestChipRendersBold(t *testing.T) {
	cases := []struct {
		name   string
		render func() tview.Primitive
		needle rune
	}{
		{"list provider chip (tview.List)", func() tview.Primitive {
			l := tview.NewList()
			l.AddItem(formatAccountLine(domain.ProviderUsage{
				AccountID: "p1", Provider: "glm", Label: "prod",
				Primary: &domain.UsageDimension{PercentUsed: 30},
			}), "", 0, nil)
			return l
		}, 'g'},
		{"details provider chip (tview.TextView)", func() tview.Primitive {
			tv := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
			tv.SetText(providerInfoLine("glm"))
			return tv
		}, 'g'},
		{"header makeTag (tview.TextView)", func() tview.Primitive {
			tv := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
			tv.SetText(makeTag("v1.0", "#10B981"))
			return tv
		}, 'v'},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !runeIsBold(c.render(), c.needle) {
				t.Errorf("chip did not render bold; background-bearing chips must use [black:bg:b], not [black:bg::b] (3-colon form drops the flag in tview)")
			}
		})
	}
}

// runeIsBold 把 primitive 画到 tcell 模拟屏，返回 needle 字符首次出现处是否带
// AttrBold。tcell SimulationScreen 的渲染路径与真实终端一致（tcell 再把 Style 翻译
// 成 SGR 转义），因此模拟屏里 bold 位置位 ≈ 真实终端会输出加粗。
func runeIsBold(p tview.Primitive, needle rune) bool {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		return false
	}
	defer s.Fini()
	const w, h = 120, 5
	s.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(s)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			str, st, _ := s.Get(x, y)
			if str == "" {
				continue
			}
			if []rune(str)[0] == needle {
				_, _, attr := st.Decompose()
				return attr&tcell.AttrBold != 0
			}
		}
	}
	return false
}
