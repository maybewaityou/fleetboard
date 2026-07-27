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

// KeyBinding 是帮助面板的一行（也是 footer 提示与文档的单一真源）。
// 新增快捷键只在此 slice 加一条，help 面板与提示即同步，永不漂移。
type KeyBinding struct {
	Group  string
	Key    string
	Action string
}

// keyBindings 是对外广告的快捷键，按分组排列。Group 字符串同时是 help 面板的分组标题。
// 约束：同组条目必须连续——collectHelpGroups 只合并相邻同名组。
var keyBindings = []KeyBinding{
	{"Navigate", "↑↓", "Move"},
	{"Navigate", "←/→", "Focus list/details"},
	{"Navigate", "/", "Search"},
	{"Navigate", "q", "Quit"},
	{"Account", "a", "New"},
	{"Account", "e", "Edit"},
	{"Account", "d", "Delete"},
	{"Account", "p", "Pin / unpin"},
	{"Usage", "r", "Refresh selected"},
	{"Usage", "R", "Refresh all"},
	{"Usage", "s/S", "Cycle sort"},
	{"Other", "?", "Help"},
}
