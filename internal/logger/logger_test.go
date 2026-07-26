// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesLogFile(t *testing.T) {
	// Redirect HOME to a temp dir so we don't touch the real ~/.fleetboard.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	lg, err := New("FLEETBOARD")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if lg == nil {
		t.Fatal("New returned nil logger")
	}

	// Writing should flush to the log file under tmp/.fleetboard/fleetboard.log.
	lg.Info("hello fleetboard")
	_ = lg.Sync()

	logPath := filepath.Join(tmp, ".fleetboard", "fleetboard.log")
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("log file is empty")
	}
}
