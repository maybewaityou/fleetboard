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
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func TestSortMode_Next(t *testing.T) {
	if got := SortByNameAsc.Next(); got != SortByUsageDesc {
		t.Errorf("Name.Next = %v, want Usage", got)
	}
	if got := SortByRefreshedDesc.Next(); got != SortByNameAsc {
		t.Errorf("Refreshed.Next = %v, want Name (wrap)", got)
	}
	if s := SortByNameAsc.Next().Next(); s != SortByRefreshedDesc {
		t.Errorf("Name.Next.Next = %v, want Refreshed", s)
	}
}

func TestSortUsagesByName(t *testing.T) {
	accts := []domain.ProviderUsage{{Label: "b"}, {Label: "a"}}
	sortUsagesForUI(accts, SortByNameAsc)
	if accts[0].Label != "a" || accts[1].Label != "b" {
		t.Errorf("Name asc order = %s,%s, want a,b", accts[0].Label, accts[1].Label)
	}
}

func TestSortUsagesByUsage(t *testing.T) {
	soon := time.Now().Add(time.Hour)
	accts := []domain.ProviderUsage{
		{Label: "lo", Dimensions: []domain.UsageDimension{{PercentUsed: 10, ResetsAt: soon}}},
		{Label: "hi", Dimensions: []domain.UsageDimension{{PercentUsed: 90, ResetsAt: soon}}},
	}
	sortUsagesForUI(accts, SortByUsageDesc)
	if accts[0].Label != "hi" {
		t.Errorf("Usage desc top = %s, want hi", accts[0].Label)
	}
}

func TestSortUsagesByRefreshed(t *testing.T) {
	now := time.Now()
	accts := []domain.ProviderUsage{
		{Label: "old", FetchedAt: now.Add(-2 * time.Hour)},
		{Label: "new", FetchedAt: now},
	}
	sortUsagesForUI(accts, SortByRefreshedDesc)
	if accts[0].Label != "new" {
		t.Errorf("Refreshed desc top = %s, want new", accts[0].Label)
	}
}

func TestSortUsagesPinnedFloat(t *testing.T) {
	accts := []domain.ProviderUsage{
		{Label: "z-pinned", Pinned: true},
		{Label: "a", Pinned: false},
	}
	sortUsagesForUI(accts, SortByNameAsc) // Name asc would put "a" first, but pinned wins
	if accts[0].Label != "z-pinned" {
		t.Errorf("pinned must float to top, got %s", accts[0].Label)
	}
}
