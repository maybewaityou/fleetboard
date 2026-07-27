package app

import (
	"sync"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func newUsage(id string, pct int, pinned bool) domain.ProviderUsage {
	return domain.ProviderUsage{
		AccountID: id,
		Provider:  "glm",
		Label:     id,
		Pinned:    pinned,
		Dimensions: []domain.UsageDimension{{
			Name:        "5h",
			PercentUsed: float64(pct),
		}},
	}
}

func TestCache_ReplaceAllAndSnapshot(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false), newUsage("b", 20, false)})
	got := c.Snapshot()
	if len(got) != 2 || got[0].AccountID != "a" || got[1].AccountID != "b" {
		t.Fatalf("Snapshot = %+v, want [a,b]", got)
	}
}

func TestCache_SnapshotIndependence(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false)})
	got := c.Snapshot()
	got[0].AccountID = "mutated"
	again := c.Snapshot()
	if again[0].AccountID != "a" || len(again) != 1 {
		t.Fatalf("Snapshot not independent: %+v", again)
	}
}

func TestCache_UpdateOne(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false), newUsage("b", 20, false)})
	c.UpdateOne(newUsage("a", 90, false)) // 替换已存在
	if got := c.Snapshot(); got[0].Dimensions[0].PercentUsed != 90 || len(got) != 2 {
		t.Fatalf("UpdateOne replace = %+v", got)
	}
	c.UpdateOne(newUsage("c", 30, false)) // 追加新条目
	if got := c.Snapshot(); len(got) != 3 || got[2].AccountID != "c" {
		t.Fatalf("UpdateOne append = %+v", got)
	}
}

func TestCache_SetPinned(t *testing.T) {
	c := NewCache()
	c.ReplaceAll([]domain.ProviderUsage{newUsage("a", 10, false)})
	c.SetPinned("a", true)
	if got := c.Snapshot(); !got[0].Pinned {
		t.Fatalf("SetPinned hit: want Pinned=true, got %+v", got[0])
	}
	c.SetPinned("missing", true) // 未命中 no-op
	if got := c.Snapshot(); len(got) != 1 {
		t.Fatalf("SetPinned miss mutated cache: %+v", got)
	}
}

func TestFindAccount(t *testing.T) {
	accs := []domain.Account{{ID: "a", Provider: "glm"}, {ID: "b", Provider: "kimi"}}
	if acc, ok := FindAccount(accs, "b"); !ok || acc.ID != "b" {
		t.Fatalf("FindAccount hit = %+v ok=%v", acc, ok)
	}
	if _, ok := FindAccount(accs, "missing"); ok {
		t.Fatal("FindAccount miss: want ok=false")
	}
}

func TestRemoveAccounts(t *testing.T) {
	accs := []domain.Account{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if got := RemoveAccounts(accs, "b"); len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("RemoveAccounts hit = %+v", got)
	}
	if got := RemoveAccounts(accs, "missing"); len(got) != 3 {
		t.Fatalf("RemoveAccounts miss = %+v", got)
	}
}

func TestCache_Concurrent(t *testing.T) {
	c := NewCache()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.ReplaceAll([]domain.ProviderUsage{newUsage("a", n, false)})
			_ = c.Snapshot()
			c.UpdateOne(newUsage("a", n, false))
			c.SetPinned("a", n%2 == 0)
		}(i)
	}
	wg.Wait()
	_ = c.Snapshot() // -race 下无竞争即通过
}
