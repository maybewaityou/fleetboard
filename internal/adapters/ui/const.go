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

// Package ui hosts the tview-based TUI adapter for fleetboard. This file holds
// the Tokyo Night palette shared by the theme application, provider tags and the
// status indicator.
//
// The palette is ported from lazytmux/internal/adapters/ui/const.go and the
// shared color values are aligned verbatim so the two tools look identical.
// colorRed and colorGray are net-new for fleetboard: they back StatusColor,
// which lazytmux does not need.
package ui

// Tokyo Night palette (see docs/superpowers/specs/2026-07-27-fleetboard-design.md §9).
const (
	colorBorder    = "#292e42"
	colorTitle     = "#7dcfff"
	colorPrimary   = "#c0caf5"
	colorSecondary = "#565f89"
	colorAccent    = "#7aa2f7"
	colorGreen     = "#9ece6a"
	colorYellow    = "#e0af68"
	colorPurple    = "#bb9af7"
	colorCyan      = "#7dcfff"
	colorGray      = "#414868" // Tokyo Night "comment"/dim — StatusColor N/A.
	colorRed       = "#f7768e" // Tokyo Night red — StatusColor >=90%.
	colorSelected  = "#33467c"
)
