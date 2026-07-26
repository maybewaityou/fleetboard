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

// Command fleetboard is the AI coding-plan usage dashboard TUI. This main.go is
// the task-8 UI-shell smoke harness: it wires a few mock accounts into the TUI
// and wires r/R to re-fetch the mock so both refresh keys give visible feedback.
// Task 9/12 replaces this with the real service + config store.
package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/maybewaityou/fleetboard/internal/adapters/ui"
	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/logger"
)

var (
	version   = "develop"
	gitCommit = "unknown"
)

func main() {
	sugar, err := logger.New("FLEETBOARD")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fleetboard: init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = sugar.Sync() }()

	root := &cobra.Command{
		Use:   "fleetboard",
		Short: "AI coding plan usage dashboard TUI",
		RunE: func(*cobra.Command, []string) error {
			seed := newMockData()
			tui := ui.NewTUI(ui.Config{
				Logger:      sugar,
				Version:     version,
				Commit:      gitCommit,
				InitialData: seed.snapshot(),
				// r/R re-fetch the mock (with mild drift) so both refresh keys
				// produce visible motion during manual smoke.
				RefreshSelected: func() []domain.VendorUsage { return seed.refreshSelected() },
				RefreshAll:      func() []domain.VendorUsage { return seed.refreshAll() },
			})
			return tui.Run()
		},
	}
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// mockData holds a small in-memory set of accounts and jitters percents on each
// refresh so r/R have something visible to do. It exists only for the shell
// smoke test; the real data path comes in Task 9/12.
type mockData struct {
	mu       sync.Mutex
	tick     int
	accounts []domain.VendorUsage
}

func newMockData() *mockData {
	now := time.Now()
	reset := now.Add(2 * time.Hour)
	m := &mockData{}
	m.accounts = []domain.VendorUsage{
		{
			AccountID: "glm-prod", Vendor: "glm", Label: "GLM 生产",
			FetchedAt: now,
			Dimensions: []domain.UsageDimension{
				{Name: "GLM-4.5", Used: 710_000, Limit: 1_000_000, PercentUsed: 71, Remaining: 290_000, ResetsAt: reset, Unit: "tok", Source: "api-balanced"},
				{Name: "GLM-4-Air", Used: 120_000, Limit: 500_000, PercentUsed: 24, Remaining: 380_000, ResetsAt: reset, Unit: "tok", Source: "api-balanced"},
			},
		},
		{
			AccountID: "minimax-dev", Vendor: "minimax", Label: "MiniMax Dev",
			FetchedAt: now,
			Dimensions: []domain.UsageDimension{
				{Name: "abab6.5", Used: 940_000, Limit: 1_000_000, PercentUsed: 94, Remaining: 60_000, ResetsAt: reset.Add(3 * time.Hour), Unit: "tok", Source: "api-balanced"},
			},
		},
		{
			AccountID: "kimi-personal", Vendor: "kimi", Label: "Kimi 个人",
			FetchedAt: now,
			Dimensions: []domain.UsageDimension{
				{Name: "moonshot", Used: 40_000, Limit: 200_000, PercentUsed: 20, Remaining: 160_000, ResetsAt: reset, Unit: "tok", Source: "api-balanced"},
			},
		},
		{
			// Partial-failure account: err is set but dimensions are still
			// populated, exercising the task-7 err-transparency contract in the
			// UI (⚠ marker in list, dimensions still shown in details).
			AccountID: "anthropic-stg", Vendor: "anthropic", Label: "Anthropic Staging",
			FetchedAt: now, Err: fmt.Errorf("partial: rate-limited"),
			Dimensions: []domain.UsageDimension{
				{Name: "claude-sonnet", Used: 5, Limit: 0, PercentUsed: -1, Remaining: 0, ResetsAt: time.Time{}, Unit: "req", Source: "api-balanced"},
			},
		},
	}
	for i := range m.accounts {
		m.accounts[i].SelectPrimary()
	}
	return m
}

func (m *mockData) snapshot() []domain.VendorUsage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.VendorUsage, len(m.accounts))
	copy(out, m.accounts)
	return out
}

// refreshSelected bumps the selected GLM account's primary usage slightly so r
// shows motion. In the shell we don't know which account is selected from here,
// so we drift every account's first dimension by a small, bounded amount — the
// list/details still visibly update on keypress.
func (m *mockData) refreshSelected() []domain.VendorUsage {
	return m.jitter()
}

func (m *mockData) refreshAll() []domain.VendorUsage {
	return m.jitter()
}

func (m *mockData) jitter() []domain.VendorUsage {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tick++
	for i := range m.accounts {
		u := &m.accounts[i]
		u.FetchedAt = time.Now()
		for j := range u.Dimensions {
			d := &u.Dimensions[j]
			if d.PercentUsed < 0 {
				continue
			}
			// oscillate ±2% per tick so motion is visible but bounded.
			delta := float64(2 * (m.tick % 5))
			d.PercentUsed = clampPct(d.PercentUsed - 1 + delta)
			d.Used = int64(float64(d.Limit) * d.PercentUsed / 100)
			if d.Limit > 0 {
				d.Remaining = d.Limit - d.Used
			}
		}
		u.SelectPrimary()
	}
	out := make([]domain.VendorUsage, len(m.accounts))
	copy(out, m.accounts)
	return out
}

func clampPct(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}
