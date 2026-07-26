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

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version   = "develop"
	gitCommit = "unknown"
)

func main() {
	root := &cobra.Command{
		Use:   "fleetboard",
		Short: "AI Coding plan usage dashboard TUI",
		RunE: func(*cobra.Command, []string) error {
			fmt.Printf("fleetboard %s (%s) — scaffold\n", version, gitCommit)
			return nil
		},
	}
	root.SilenceUsage = true
	if err := root.Execute(); err != nil {
		fmt.Println(err)
	}
}
