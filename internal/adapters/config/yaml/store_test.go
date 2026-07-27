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

package yaml_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/adapters/config/yaml"
	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	s := yaml.NewStore(path)
	orig := domain.Config{Accounts: []domain.Account{{ID: "g", Provider: "glm", Label: "智谱", TokenEnv: "GLM_API_KEY"}}}
	if err := s.Save(orig); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].ID != "g" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	// 权限 0600
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600", fi.Mode().Perm())
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	s := yaml.NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	got, err := s.Load()
	if err != nil || len(got.Accounts) != 0 {
		t.Fatalf("want empty+nil, got %+v %v", got, err)
	}
}

// TestSaveRollsBackups saves more than maxBackups (10) times and verifies that
// (a) at most 10 .bak files survive and (b) base-1.bak holds the second-to-last
// version — i.e. the live file holds the latest save and the chain ages away
// from it.
func TestSaveRollsBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	s := yaml.NewStore(path)

	const saves = 12
	for i := 0; i < saves; i++ {
		cfg := domain.Config{
			Accounts: []domain.Account{{
				ID:       fmt.Sprintf("a%d", i),
				Provider:   "glm",
				TokenEnv: "GLM_API_KEY",
			}},
		}
		if err := s.Save(cfg); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	// Live file must hold the latest save (a11).
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].ID != "a11" {
		t.Fatalf("live config = %+v, want a11", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var baks []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			baks = append(baks, e.Name())
		}
	}
	if len(baks) > 10 {
		t.Fatalf("expected <= 10 backups, got %d: %v", len(baks), baks)
	}
	// The save that precedes the live one (a10) must be in slot 1.
	b, err := os.ReadFile(filepath.Join(dir, "config-1.bak"))
	if err != nil {
		t.Fatalf("read config-1.bak: %v", err)
	}
	if !strings.Contains(string(b), "a10") {
		t.Fatalf("config-1.bak should contain a10, got:\n%s", b)
	}
	// The very first save (a0) must have aged out past slot 10.
	if _, err := os.Stat(filepath.Join(dir, "config-10.bak")); err != nil {
		t.Fatalf("config-10.bak should exist: %v", err)
	}
	b10, err := os.ReadFile(filepath.Join(dir, "config-10.bak"))
	if err != nil {
		t.Fatalf("read config-10.bak: %v", err)
	}
	if strings.Contains(string(b10), "a0") {
		t.Fatalf("a0 should have been rotated out of slot 10, got:\n%s", b10)
	}
}

// TestOverwriteResetsPerm guards against the WriteFile-keeps-old-mode gotcha: if
// a stale temp file with loose perms lingers from a crashed run, the explicit
// Chmod must still bring the committed file back to 0600.
func TestOverwriteResetsPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	s := yaml.NewStore(path)

	if err := s.Save(domain.Config{}); err != nil {
		t.Fatal(err)
	}
	// Plant a loose-mode temp file, simulating a prior crashed save.
	if err := os.WriteFile(path+".tmp", []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(domain.Config{Accounts: []domain.Account{{ID: "x", Provider: "glm", TokenEnv: "K"}}}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600 after overwrite", fi.Mode().Perm())
	}
}
