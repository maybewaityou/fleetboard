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

package domain

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSelectPrimaryPicksMaxPercent(t *testing.T) {
	u := &ProviderUsage{Dimensions: []UsageDimension{
		{Name: "5h", PercentUsed: 30},
		{Name: "weekly", PercentUsed: 88},
		{Name: "mcp", PercentUsed: -1},
	}}
	u.SelectPrimary()
	if u.Primary == nil || u.Primary.Name != "weekly" {
		t.Fatalf("want weekly, got %+v", u.Primary)
	}
}

func TestSelectPrimaryAllInvalidReturnsNil(t *testing.T) {
	u := &ProviderUsage{Dimensions: []UsageDimension{
		{Name: "5h", PercentUsed: -1},
		{Name: "monthly", PercentUsed: -1},
	}}
	u.SelectPrimary()
	if u.Primary != nil {
		t.Fatalf("want nil Primary when all dimensions are N/A, got %+v", u.Primary)
	}
}

func TestSelectPrimaryEmptyDimensionsReturnsNil(t *testing.T) {
	u := &ProviderUsage{}
	u.SelectPrimary()
	if u.Primary != nil {
		t.Fatalf("want nil Primary when no dimensions, got %+v", u.Primary)
	}
}

// TestRecentUsageNilDefault 验证 ProviderUsage.Recent 默认 nil（UI 据此跳过区块）。
func TestRecentUsageNilDefault(t *testing.T) {
	var u ProviderUsage
	if u.Recent != nil {
		t.Errorf("Recent should default to nil, got %+v", u.Recent)
	}
}

// TestAccountNewFields 验证 new-api 凭证字段可构造且默认零值（非 newapi 账号无感）。
func TestAccountNewFields(t *testing.T) {
	var zero Account
	if zero.AccessTokenEnv != "" || zero.UserID != "" {
		t.Fatalf("new fields should default to empty, got %+v", zero)
	}
	acc := Account{
		Provider:       "newapi",
		AccessTokenEnv: "NEWAPI_AT",
		UserID:         "16002",
	}
	if acc.AccessTokenEnv != "NEWAPI_AT" || acc.UserID != "16002" {
		t.Fatalf("new credential fields not set: %+v", acc)
	}
}

// TestRefreshConfigTimeoutYAML 验证 RefreshConfig.Timeout 的 yaml 解析与零值默认。
func TestRefreshConfigTimeoutYAML(t *testing.T) {
	var cfg struct {
		Refresh RefreshConfig `yaml:"refresh"`
	}
	if err := yaml.Unmarshal([]byte("refresh:\n  timeout: 15s\n"), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Refresh.Timeout != "15s" {
		t.Errorf("Refresh.Timeout = %q, want 15s", cfg.Refresh.Timeout)
	}
	// 零值：未配置时为空字符串（main 据此回退默认 15s）。
	var zero RefreshConfig
	if zero.Timeout != "" {
		t.Errorf("zero-value Timeout should be empty, got %q", zero.Timeout)
	}
}
