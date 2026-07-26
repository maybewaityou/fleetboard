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

// Package yaml is a ConfigStore adapter that persists domain.Config to a YAML
// file. Writes are atomic (temp file + chmod 0600 + rename) and non-destructive:
// before each overwrite the previous version is rolled into a numbered backup
// (base-1.bak .. base-10.bak), keeping at most maxBackups copies.
package yaml

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

// maxBackups is the cap on retained backup copies. Once the limit is reached the
// oldest slot (base-10.bak) is dropped before each rotation.
const maxBackups = 10

// Store implements ports.ConfigStore against a single YAML file. It is stateless
// across calls: Load always re-reads disk and Save always rewrites, so there is
// no in-memory snapshot to go stale. A mutex serializes the read/rotate/write
// sequence so two concurrent Saves cannot corrupt the shared temp file.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore returns a Store bound to path. The file (and its parent directory)
// need not exist yet: Load on a missing path returns an empty Config, and Save
// creates the parent directory on demand.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads and unmarshals the config file. A missing file is treated as
// "first run" and yields an empty Config with a nil error rather than failing.
func (s *Store) Load() (domain.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Config{}, nil
	}
	if err != nil {
		return domain.Config{}, fmt.Errorf("read config %s: %w", s.path, err)
	}

	var cfg domain.Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return domain.Config{}, fmt.Errorf("parse config %s: %w", s.path, err)
	}
	return cfg, nil
}

// Save marshals cfg to YAML and persists it atomically: it writes path+".tmp",
// forces 0600 permissions, rolls the previous version into base-1.bak (shifting
// older backups down and dropping anything past base-10.bak), then renames the
// temp file over path. The previous file is backed up before the rename so the
// live file is never left half-written or missing.
func (s *Store) Save(cfg domain.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
	}

	b, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := s.path + ".tmp"
	// WriteFile truncates any pre-existing temp file but, when the file already
	// exists, does not reset its mode — so Chmod afterwards guarantees 0600
	// regardless of umask or a stale temp left by a crashed run.
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fmt.Errorf("chmod temp config: %w", err)
	}

	if err := s.rotateBackups(); err != nil {
		return fmt.Errorf("rotate backups: %w", err)
	}
	if _, err := os.Stat(s.path); err == nil {
		if err := copyFile(s.path, s.backupPath(1), 0o600); err != nil {
			return fmt.Errorf("snapshot previous config: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config: %w", err)
	}

	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}
	return nil
}

// rotateBackups shifts base-N.bak -> base-(N+1).bak for N = maxBackups-1 down
// to 1, having first removed base-maxBackups.bak. Walking high-to-low keeps each
// rename target free, so the chain never overwrites an existing backup.
func (s *Store) rotateBackups() error {
	oldest := s.backupPath(maxBackups)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for n := maxBackups - 1; n >= 1; n-- {
		from := s.backupPath(n)
		if _, err := os.Stat(from); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if err := os.Rename(from, s.backupPath(n+1)); err != nil {
			return err
		}
	}
	return nil
}

// backupPath returns the n-th backup location: same directory as path, base name
// with extension stripped, then "-<n>.bak". For "config.yaml" / 3 that is
// "config-3.bak" beside the original.
func (s *Store) backupPath(n int) string {
	dir := filepath.Dir(s.path)
	base := strings.TrimSuffix(filepath.Base(s.path), filepath.Ext(s.path))
	return filepath.Join(dir, fmt.Sprintf("%s-%d.bak", base, n))
}

// copyFile copies src to dst, creating dst with the given mode. It is used only
// for backups, so a partial copy on error merely yields a slightly truncated
// backup — the live config is untouched because the rename has not happened yet.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Compile-time check that Store satisfies the port.
var _ ports.ConfigStore = (*Store)(nil)
