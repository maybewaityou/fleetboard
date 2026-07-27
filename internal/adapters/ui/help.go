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
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// 双列帮助面板，移植自 lazytmux help.go。核心设计：两列拼进单个 TextView
// （SetWrap(false) + Scrollable），滚动时两列同步、表头永不对齐错位。

type helpBinding struct {
	key    string
	action string
}

type helpGroup struct {
	name     string
	bindings []helpBinding
}

// collectHelpGroups 遍历 keyBindings 单一真源，按相邻同 Group 聚合（不合并非相邻同名组）。
func collectHelpGroups() []helpGroup {
	var groups []helpGroup
	for _, kb := range keyBindings {
		if len(groups) == 0 || groups[len(groups)-1].name != kb.Group {
			groups = append(groups, helpGroup{name: kb.Group})
		}
		groups[len(groups)-1].bindings = append(groups[len(groups)-1].bindings, helpBinding{key: kb.Key, action: kb.Action})
	}
	return groups
}

// renderGroupLines 把一个分组渲染为行切片：彩色表头 + 每条绑定缩进。
func renderGroupLines(g helpGroup) []string {
	lines := []string{"[" + colorAccent + "::b]" + g.name + "[-]"}
	for _, bd := range g.bindings {
		lines = append(lines, fmt.Sprintf("  ["+colorCyan+"]%-6s[-]  %s", bd.key, bd.action))
	}
	return lines
}

const helpGutter = 2

// leftColumnWidth 是每个左列行右补到的宽度（让右列起始列对齐）。
func leftColumnWidth(groups []helpGroup) int {
	w := 0
	for _, g := range groups {
		for _, line := range renderGroupLines(g) {
			if tw := tview.TaggedStringWidth(line); tw > w {
				w = tw
			}
		}
	}
	return w
}

// padTagged 右补空格到屏幕宽度 w，保留 tview 颜色 tag（tag 不计宽度）。
func padTagged(s string, w int) string {
	if pad := w - tview.TaggedStringWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// renderHelpRow 把一对分组行对行合并：左行补到 leftColumnWidth + gutter + 右行。
// 右侧缺失（rightIndex -1，如 Navigate 行）只打印左行。
func renderHelpRow(groups []helpGroup, row [2]int, w int) []string {
	hasRight := row[1] >= 0 && row[1] < len(groups)
	var left, right []string
	if row[0] >= 0 && row[0] < len(groups) {
		left = renderGroupLines(groups[row[0]])
	}
	if hasRight {
		right = renderGroupLines(groups[row[1]])
	}
	h := len(left)
	if len(right) > h {
		h = len(right)
	}
	out := make([]string, 0, h)
	for k := 0; k < h; k++ {
		var l, r string
		if k < len(left) {
			l = left[k]
		}
		if k < len(right) {
			r = right[k]
		}
		line := padTagged(l, w)
		if hasRight {
			line += strings.Repeat(" ", helpGutter) + r
		}
		out = append(out, strings.TrimRight(line, " "))
	}
	return out
}

// buildHelpBody 把成对分组铺成一个可滚动文本块，行间空行。
func buildHelpBody(groups []helpGroup, rows [][2]int) string {
	w := leftColumnWidth(groups)
	var b strings.Builder
	for ri, row := range rows {
		for _, line := range renderHelpRow(groups, row, w) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if ri < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// pairHelpGroups 把分组排成双列行：第 0 组独占首行（右空，突出 Navigate），
// 其余两两配对，尾组奇数独占一行。返回 [leftIndex, rightIndex]，-1=空。
func pairHelpGroups(n int) [][2]int {
	var rows [][2]int
	if n == 0 {
		return rows
	}
	rows = append(rows, [2]int{0, -1})
	for i := 1; i < n; i += 2 {
		right := -1
		if i+1 < n {
			right = i + 1
		}
		rows = append(rows, [2]int{i, right})
	}
	return rows
}

// renderHelpBody 构建帮助正文（纯函数，可单测）。
func renderHelpBody() string {
	groups := collectHelpGroups()
	rows := pairHelpGroups(len(groups))
	return buildHelpBody(groups, rows)
}

// helpTextView 是不换行、动态颜色的文本窗格。
func helpTextView(text string) *tview.TextView {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetTextAlign(tview.AlignLeft)
	tv.SetWrap(false)
	tv.SetText(text)
	return tv
}

// HelpModal 是帮助面板：标题行 + 可滚动双列正文。内容全部来自 keyBindings。
type HelpModal struct {
	*tview.Flex
	focus tview.Primitive
}

// NewHelpModal 构建面板。嵌入布局 Flex，并把可滚动 body 暴露为焦点目标。
func NewHelpModal() *HelpModal {
	bodyTv := helpTextView(renderHelpBody())
	bodyTv.SetScrollable(true)
	title := helpTextView("[" + colorAccent + "::b]fleetboard — Key Bindings  (Esc / ? / q to close)[-]")
	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(bodyTv, 0, 1, true)
	return &HelpModal{Flex: root, focus: bodyTv}
}
