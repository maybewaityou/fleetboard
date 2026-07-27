# 国内其他平台接入（Kimi + DeepSeek）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 接入 Kimi（Moonshot）与 DeepSeek 两家国内 AI 平台的余额查询，并引入「余额型」vendor 展示模式，使无百分比的余额账户在列表与详情页正确展示。

**Architecture:** 沿用六边形（ports & adapters）。每个新平台 = 一个 `providers/<name>/<name>.go` 实现 `ports.UsageProvider`。在 `UsageDimension` 加 `Balance`+`Currency` 两字段表达余额（配额型零值不受影响），UI 层据 `Currency != ""` 走余额型渲染分支。

**Tech Stack:** Go 1.24 · tview/tcell · 标准库 `net/http` + `encoding/json` + `strconv`

**Reference spec:** `docs/superpowers/specs/2026-07-27-domestic-platforms-design.md`

## Global Constraints

- **module path**: `github.com/maybewaityou/fleetboard`
- **Go 版本**: `go 1.24.6`
- **代码风格**: `gofumpt` + `go vet`；每个新 `.go` 文件顶部加项目既有 Apache 2.0 license header（全文见下方模板）
- **测试**: `go test -race -cover ./...`；TDD（先红后绿）
- **提交**: 语义化 `type(scope): 简短描述`；每个 Task 末尾 commit
- **余额型约定**: 判断依据 `Currency != ""`（非 `Balance > 0`，因余额可为 0 或负）；余额型 adapter 显式设 `u.Primary = &u.Dimensions[0]`，**不调 `SelectPrimary()`**；余额型 `PercentUsed = -1`，列表 miniBar 复用 `renderBar(-1, 4)` 自然得灰条
- **token 不明文落盘**: config 只存 `token_env`，adapter 运行时 `os.Getenv` 取值
- **日志脱敏**: token / Authorization 永不打屏

**License header 模板**（每个新建 `.go` 文件顶部，原样粘贴）：

```
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
```

---

## File Structure

**新建：**
- `internal/adapters/providers/kimi/kimi.go` + `kimi_test.go` — Kimi 余额 adapter
- `internal/adapters/providers/deepseek/deepseek.go` + `deepseek_test.go` — DeepSeek 余额 adapter

**改动：**
- `internal/core/domain/vendor_usage.go` — `UsageDimension` 加 `Balance` + `Currency`
- `internal/adapters/ui/account_list.go` — `formatAccountLine` 余额型分支 + `formatMoneyShort`
- `internal/adapters/ui/account_details.go` — `renderDimension` 余额型分支 + `formatMoney`/`currencySymbol`
- `internal/adapters/ui/account_form.go` — `vendorOptions` 加 kimi/deepseek
- `internal/adapters/ui/theme.go` — `vendorColor` 加 deepseek
- `internal/adapters/ui/theme_test.go` — `TestVendorTag_KnownVendors` 表加 deepseek
- `cmd/main.go` — Registry 注册 kimi/deepseek + import
- `docs/superpowers/specs/2026-07-27-fleetboard-design.md` — §9.2 配色清单加 deepseek

---

## Task 1: 扩展 UsageDimension 数据模型（Balance + Currency）

**Files:**
- Modify: `internal/core/domain/vendor_usage.go`（`UsageDimension` 结构体）
- Test: `internal/core/domain/vendor_usage.go` 内新增 `TestUsageDimensionBalanceFields`

**Interfaces:**
- Produces: `domain.UsageDimension` 新增 `Balance float64` 与 `Currency string` 字段；后续所有 adapter/UI task 据此判断余额型（`Currency != ""`）。

- [ ] **Step 1: 写失败测试**

在 `internal/core/domain/vendor_usage.go` 末尾追加（package `domain` 已 import `testing`？否——若未 import，测试放 `vendor_usage_test.go`）。新建 `internal/core/domain/vendor_usage_test.go`：

```go
// <LICENSE_HEADER>
package domain

import "testing"

// TestUsageDimensionBalanceFields 验证余额型字段可读写，且余额型维度（PercentUsed=-1）
// 在 SelectPrimary 中被跳过（配额型行为不受影响）。
func TestUsageDimensionBalanceFields(t *testing.T) {
	dim := UsageDimension{
		Name:        "可用余额",
		Balance:     49.58,
		Currency:    "CNY",
		PercentUsed: -1,
	}
	if dim.Balance != 49.58 {
		t.Errorf("Balance = %v, want 49.58", dim.Balance)
	}
	if dim.Currency != "CNY" {
		t.Errorf("Currency = %q, want CNY", dim.Currency)
	}

	// SelectPrimary 跳过 PercentUsed<0 的维度：纯余额型维度集合 → Primary 为 nil
	u := VendorUsage{Dimensions: []UsageDimension{dim}}
	u.SelectPrimary()
	if u.Primary != nil {
		t.Errorf("SelectPrimary should skip PercentUsed<0 balance dim, got Primary=%+v", u.Primary)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/core/domain/ -run TestUsageDimensionBalanceFields -v`
Expected: FAIL / 编译失败（`dim.Balance undefined` / `dim.Currency undefined`）

- [ ] **Step 3: 加字段实现**

在 `vendor_usage.go` 的 `UsageDimension` 结构体末尾（`Source string` 之后）追加两字段：

```go
type UsageDimension struct {
	Name        string
	Used        int64
	Limit       int64
	PercentUsed float64 // -1 = N/A
	Remaining   int64
	ResetsAt    time.Time
	Unit        string
	Source      string

	// 余额型 vendor 专用（Kimi/DeepSeek）：Balance 是当前余额（元/美元），
	// Currency 为 "CNY"/"USD"。配额型两者均零值。判断余额型用 Currency != ""。
	Balance  float64
	Currency string
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `go test ./internal/core/domain/ -run TestUsageDimensionBalanceFields -v`
Expected: PASS

- [ ] **Step 5: 全量回归 + 提交**

Run: `go test -race ./...`
Expected: 全绿（纯加字段，不破坏现有配额型逻辑）

```bash
git add internal/core/domain/vendor_usage.go internal/core/domain/vendor_usage_test.go
git commit -m "feat(domain): add Balance/Currency fields to UsageDimension"
```

---

## Task 2: Kimi（Moonshot）余额 adapter

**Files:**
- Create: `internal/adapters/providers/kimi/kimi.go`
- Test: `internal/adapters/providers/kimi/kimi_test.go`

**Interfaces:**
- Consumes: `domain.Account`（`TokenEnv`/`BaseURL`）、Task 1 的 `UsageDimension.Balance`/`Currency`
- Produces: `kimi.New() *Provider`，实现 `ports.UsageProvider`（`Vendor()="kimi"`、`FetchUsage`）

**接口契约（写进文件头注释）：**
- `GET {BaseURL}/v1/users/me/balance`，默认 `https://api.moonshot.cn`
- 鉴权 `Authorization: Bearer <KEY>`（**必须带 Bearer 前缀**）
- 成功态 `{code, data:{available_balance, voucher_balance, cash_balance}, scode, status}`；错误态 `{error:{message,type,code}}`（结构不同，需状态守卫先拦截）
- 货币随 base：`.cn`→CNY / `.ai`→USD

- [ ] **Step 1: 写失败测试（kimi_test.go）**

```go
// <LICENSE_HEADER>
package kimi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 Moonshot /v1/users/me/balance 成功响应金样本。
const goldenPayload = `{"code":0,"data":{"available_balance":49.58894,"voucher_balance":46.58893,"cash_balance":3.00001},"scode":"0x0","status":true}`

func TestVendorReturnsKimi(t *testing.T) {
	if got := New().Vendor(); got != "kimi" {
		t.Fatalf("Vendor() = %q, want kimi", got)
	}
}

// TestCurrencyFor 验证货币推断：moonshot.ai → USD，其余（含 .cn / 本地测试 server）→ CNY。
func TestCurrencyFor(t *testing.T) {
	cases := map[string]string{
		"https://api.moonshot.cn":  "CNY",
		"https://api.moonshot.ai":  "USD",
		"http://127.0.0.1:1234":    "CNY",
	}
	for base, want := range cases {
		if got := currencyFor(base); got != want {
			t.Errorf("currencyFor(%q) = %q, want %q", base, got, want)
		}
	}
}

// TestFetchUsageGolden 核心金测试：
//   (a) Authorization = "Bearer KEY123"（必须 Bearer 前缀）
//   (b) 路径 = /v1/users/me/balance
//   (c) 维度 Balance = available_balance，Currency = CNY（base 是本地 server → CNY），PercentUsed = -1
//   (d) Primary 指向该维度
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenPayload)
	}))
	defer srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "KEY123")
	acc := domain.Account{ID: "k", Vendor: "kimi", Label: "Kimi", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a)(b) 鉴权 + 路径
	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want %q (MUST have Bearer prefix)", gotAuth, "Bearer KEY123")
	}
	if gotPath != "/v1/users/me/balance" {
		t.Errorf("path = %q, want /v1/users/me/balance", gotPath)
	}

	// (c) 余额型维度
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Name != "可用余额" {
		t.Errorf("dim.Name = %q, want 可用余额", d.Name)
	}
	if d.Balance != 49.58894 {
		t.Errorf("dim.Balance = %v, want 49.58894 (available_balance)", d.Balance)
	}
	if d.Currency != "CNY" {
		t.Errorf("dim.Currency = %q, want CNY (base is local server)", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("dim.PercentUsed = %v, want -1 (balance type has no percent)", d.PercentUsed)
	}
	if d.Source != "api-balanced" {
		t.Errorf("dim.Source = %q, want api-balanced", d.Source)
	}

	// (d) Primary 指向余额维度
	if u.Primary == nil || u.Primary.Name != "可用余额" {
		t.Errorf("Primary = %+v, want 可用余额 dim", u.Primary)
	}

	// 账号字段 + Basic Info
	if u.AccountID != "k" || u.Vendor != "kimi" || u.Label != "Kimi" {
		t.Errorf("top fields wrong: %+v", u)
	}
	if u.Endpoint != "/v1/users/me/balance" {
		t.Errorf("Endpoint = %q, want /v1/users/me/balance", u.Endpoint)
	}
	if u.BaseURL != srv.URL {
		t.Errorf("BaseURL = %q, want %s", u.BaseURL, srv.URL)
	}
	if u.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set")
	}
}

// TestFetchUsageNonZeroCode 验证 code!=0 被拦截（不只看 HTTP 200）。
func TestFetchUsageNonZeroCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":401,"data":{},"status":false}`)
	}))
	defer srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "K")
	acc := domain.Account{ID: "k", Vendor: "kimi", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for code!=0, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set")
	}
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty on code!=0, got %+v", u.Dimensions)
	}
}

// TestFetchUsageServerDown 验证传输错误透传 + 账号字段仍填充。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close()

	t.Setenv("MOONSHOT_API_KEY", "K")
	acc := domain.Account{ID: "k", Vendor: "kimi", Label: "l", TokenEnv: "MOONSHOT_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server down")
	}
	if u.AccountID != "k" || u.Vendor != "kimi" || u.Label != "l" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/adapters/providers/kimi/ -v`
Expected: FAIL / 编译失败（`kimi` package 不存在）

- [ ] **Step 3: 实现 kimi.go**

```go
// <LICENSE_HEADER>

// Package kimi 实现 ports.UsageProvider，对接 Moonshot（Kimi）账户余额接口。
//
// 真实接口契约（platform.kimi.com/docs/api/balance）：
//   - GET {BaseURL}/v1/users/me/balance
//     默认 BaseURL = https://api.moonshot.cn（CNY）；国际版可覆盖为 https://api.moonshot.ai（USD）。
//   - 鉴权头：Authorization: Bearer <MOONSHOT_API_KEY> —— 【必须带 "Bearer " 前缀】。
//
// 成功响应 {code, data:{available_balance, voucher_balance, cash_balance}, scode, status}；
// 错误响应为 OpenAI 风格 {error:{message,type,code}}，结构不同，故先以 HTTP 状态守卫拦截，
// 再判 code==0。余额型：仅展示 available_balance，PercentUsed=-1，Currency 按 base 推断。
package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	defaultBaseURL = "https://api.moonshot.cn"
	usagePath      = "/v1/users/me/balance"
	httpTimeout    = 10 * time.Second

	sourceTag     = "api-balanced"
	nameAvailable = "可用余额"

	codeOK      = 0
	currencyCNY = "CNY"
	currencyUSD = "USD"
)

// 编译期断言：*Provider 实现 ports.UsageProvider。
var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 Kimi 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 Kimi Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Vendor 返回厂商标识。
func (p *Provider) Vendor() string { return "kimi" }

// apiResp 是 Moonshot 余额接口的成功响应结构。
type apiResp struct {
	Code int `json:"code"`
	Data struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
	Status bool `json:"status"`
}

// currencyFor 按 base URL 推断货币：moonshot.ai → USD，其余 → CNY。
// 本地 httptest server（127.0.0.1）走 CNY 默认分支。
func currencyFor(base string) string {
	if strings.Contains(base, "moonshot.ai") {
		return currencyUSD
	}
	return currencyCNY
}

// FetchUsage 拉取该账号余额，返回单维度余额型 VendorUsage。
// 出错时 VendorUsage 仍被填充（账号字段 + FetchedAt + Err）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.VendorUsage, error) {
	u := domain.VendorUsage{
		AccountID: acc.ID,
		Vendor:    "kimi",
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}

	key := os.Getenv(acc.TokenEnv)
	base := acc.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	u.BaseURL = base
	u.Endpoint = usagePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+usagePath, nil)
	if err != nil {
		u.Err = fmt.Errorf("kimi: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("kimi: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	// 状态守卫：错误响应体结构与成功态不同，先按 HTTP 状态拦截，避免误解码。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("kimi: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("kimi: decode response: %w", err)
		return u, u.Err
	}
	// 业务码守卫：不只看 HTTP 200，code!=0 视为失败。
	if r.Code != codeOK {
		u.Err = fmt.Errorf("kimi: non-zero code %d", r.Code)
		return u, u.Err
	}

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     r.Data.AvailableBalance,
		Currency:    currencyFor(base),
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0] // 余额型：Primary 指向余额维度（不调 SelectPrimary）
	return u, nil
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `go test ./internal/adapters/providers/kimi/ -v`
Expected: PASS（全部用例）

- [ ] **Step 5: 提交**

```bash
git add internal/adapters/providers/kimi/
git commit -m "feat(providers): add kimi balance adapter"
```

---

## Task 3: DeepSeek 余额 adapter

**Files:**
- Create: `internal/adapters/providers/deepseek/deepseek.go`
- Test: `internal/adapters/providers/deepseek/deepseek_test.go`

**Interfaces:**
- Consumes: `domain.Account`、Task 1 的 `UsageDimension.Balance`/`Currency`
- Produces: `deepseek.New() *Provider`，实现 `ports.UsageProvider`（`Vendor()="deepseek"`、`FetchUsage`）

**接口契约：**
- `GET {BaseURL}/user/balance`，默认 `https://api.deepseek.com`
- 鉴权 `Authorization: Bearer <API_KEY>`
- 响应 `{is_available, balance_infos:[{currency, total_balance, granted_balance, topped_up_balance}]}`，**金额字段全是 string**（需 `strconv.ParseFloat`，无 /100 换算），取 `balance_infos[0]`

- [ ] **Step 1: 写失败测试（deepseek_test.go）**

```go
// <LICENSE_HEADER>
package deepseek

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 DeepSeek /user/balance 响应金样本。金额是 string。
const goldenPayload = `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"110.00","granted_balance":"10.00","topped_up_balance":"100.00"}]}`

func TestVendorReturnsDeepSeek(t *testing.T) {
	if got := New().Vendor(); got != "deepseek" {
		t.Fatalf("Vendor() = %q, want deepseek", got)
	}
}

// TestFetchUsageGolden：
//   (a) Authorization = "Bearer KEY123"
//   (b) 路径 = /user/balance
//   (c) Balance = 110.0（"110.00" ParseFloat，无 /100），Currency = CNY，PercentUsed = -1
//   (d) Primary 指向该维度
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenPayload)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "KEY123")
	acc := domain.Account{ID: "d", Vendor: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want Bearer KEY123", gotAuth)
	}
	if gotPath != "/user/balance" {
		t.Errorf("path = %q, want /user/balance", gotPath)
	}

	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Name != "可用余额" {
		t.Errorf("dim.Name = %q, want 可用余额", d.Name)
	}
	if d.Balance != 110.0 {
		t.Errorf("dim.Balance = %v, want 110.0 (total_balance \"110.00\" parsed, no /100)", d.Balance)
	}
	if d.Currency != "CNY" {
		t.Errorf("dim.Currency = %q, want CNY (from balance_infos[0].currency)", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("dim.PercentUsed = %v, want -1", d.PercentUsed)
	}

	if u.Primary == nil || u.Primary.Name != "可用余额" {
		t.Errorf("Primary = %+v, want 可用余额 dim", u.Primary)
	}
	if u.Endpoint != "/user/balance" {
		t.Errorf("Endpoint = %q, want /user/balance", u.Endpoint)
	}
}

// TestFetchUsageUSDCurrency 验证 currency 取自 balance_infos[0].currency（USD 场景）。
func TestFetchUsageUSDCurrency(t *testing.T) {
	payload := `{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"3.00","granted_balance":"0","topped_up_balance":"3.00"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Vendor: "deepseek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Currency != "USD" {
		t.Errorf("Currency = %q, want USD", u.Dimensions[0].Currency)
	}
	if u.Dimensions[0].Balance != 3.0 {
		t.Errorf("Balance = %v, want 3.0", u.Dimensions[0].Balance)
	}
}

// TestFetchUsageNon200 验证非 2xx 被状态守卫拦截。
func TestFetchUsageNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "BAD")
	acc := domain.Account{ID: "d", Vendor: "deepseek", Label: "l", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
	if u.Err == nil {
		t.Error("u.Err should be set")
	}
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty on HTTP error, got %+v", u.Dimensions)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/adapters/providers/deepseek/ -v`
Expected: FAIL / 编译失败（package 不存在）

- [ ] **Step 3: 实现 deepseek.go**

```go
// <LICENSE_HEADER>

// Package deepseek 实现 ports.UsageProvider，对接 DeepSeek 账户余额接口。
//
// 真实接口契约（api-docs.deepseek.com/api/get-user-balance）：
//   - GET {BaseURL}/user/balance，默认 BaseURL = https://api.deepseek.com。
//   - 鉴权头：Authorization: Bearer <API_KEY>。
//
// 响应 {is_available, balance_infos:[{currency, total_balance, granted_balance,
// topped_up_balance}]}。金额字段全是 string（需 strconv.ParseFloat，单位即元/美元，
// 无 /100 换算）。total_balance = granted + topped_up，是「剩余」语义，无已用/百分比/重置。
package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	defaultBaseURL = "https://api.deepseek.com"
	usagePath      = "/user/balance"
	httpTimeout    = 10 * time.Second

	sourceTag     = "api-balanced"
	nameAvailable = "可用余额"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 DeepSeek 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 DeepSeek Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Vendor 返回厂商标识。
func (p *Provider) Vendor() string { return "deepseek" }

// apiResp 是 DeepSeek /user/balance 响应结构。金额字段为 string。
type apiResp struct {
	IsAvailable  bool          `json:"is_available"`
	BalanceInfos []balanceInfo `json:"balance_infos"`
}

// balanceInfo 是单币种余额明细。三个金额字段均为十进制字符串。
type balanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// FetchUsage 拉取该账号余额，返回单维度余额型 VendorUsage。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.VendorUsage, error) {
	u := domain.VendorUsage{
		AccountID: acc.ID,
		Vendor:    "deepseek",
		Label:     acc.Label,
		FetchedAt: time.Now(),
	}

	key := os.Getenv(acc.TokenEnv)
	base := acc.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	u.BaseURL = base
	u.Endpoint = usagePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+usagePath, nil)
	if err != nil {
		u.Err = fmt.Errorf("deepseek: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("deepseek: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("deepseek: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("deepseek: decode response: %w", err)
		return u, u.Err
	}
	if len(r.BalanceInfos) == 0 {
		u.Err = fmt.Errorf("deepseek: empty balance_infos")
		return u, u.Err
	}

	info := r.BalanceInfos[0]
	total, err := strconv.ParseFloat(info.TotalBalance, 64)
	if err != nil {
		u.Err = fmt.Errorf("deepseek: parse total_balance %q: %w", info.TotalBalance, err)
		return u, u.Err
	}

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     total,
		Currency:    info.Currency,
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `go test ./internal/adapters/providers/deepseek/ -v`
Expected: PASS（全部用例）

- [ ] **Step 5: 提交**

```bash
git add internal/adapters/providers/deepseek/
git commit -m "feat(providers): add deepseek balance adapter"
```

---

## Task 4: 余额型列表渲染 + formatMoneyShort

**Files:**
- Modify: `internal/adapters/ui/account_list.go`（`formatAccountLine`）
- Test: `internal/adapters/ui/account_list_test.go`（新增余额型用例）

**Interfaces:**
- Consumes: Task 1 的 `UsageDimension.Balance`/`Currency`
- Produces: `formatMoneyShort(balance, currency) string`（同 package，详情页也会用）

- [ ] **Step 1: 写失败测试**

在 `account_list_test.go` 末尾追加（package `ui`）：

```go
// TestFormatMoneyShort 验证余额短格式：CNY→¥、USD→$、>1000 缩写 k、1 位小数。
func TestFormatMoneyShort(t *testing.T) {
	cases := []struct {
		balance, want string
		currency      string
	}{
		{49.58894, "CNY", "¥49.6"},
		{3.0, "USD", "$3.0"},
		{1200.0, "CNY", "¥1.2k"},
		{0, "CNY", "¥0.0"},
	}
	for _, tc := range cases {
		if got := formatMoneyShort(tc.balance, tc.currency); got != tc.want {
			t.Errorf("formatMoneyShort(%v,%q) = %q, want %q", tc.balance, tc.currency, got, tc.want)
		}
	}
}

// TestFormatAccountLineBalance 验证余额型行渲染：含余额短格式、绿点（余额>0）、
// 灰色 miniBar（renderBar(-1,4) 自然得灰条）。
func TestFormatAccountLineBalance(t *testing.T) {
	balDim := domain.UsageDimension{Name: "可用余额", Balance: 49.58894, Currency: "CNY", PercentUsed: -1}
	u := domain.VendorUsage{AccountID: "k", Vendor: "kimi", Label: "Kimi-主力", Primary: &balDim, Dimensions: []domain.UsageDimension{balDim}}
	got := formatAccountLine(u)

	if !strings.Contains(got, "¥49.6") {
		t.Errorf("balance line should contain ¥49.6, got: %q", got)
	}
	if !strings.Contains(got, "["+colorGreen+"]") {
		t.Errorf("balance>0 should render green dot, got: %q", got)
	}
	if !strings.Contains(got, "["+colorGray+"]") {
		t.Errorf("balance line should have gray miniBar, got: %q", got)
	}
}

// TestFormatAccountLineBalanceDepleted 验证余额<=0 渲染红点。
func TestFormatAccountLineBalanceDepleted(t *testing.T) {
	balDim := domain.UsageDimension{Name: "可用余额", Balance: 0, Currency: "CNY", PercentUsed: -1}
	u := domain.VendorUsage{AccountID: "d", Vendor: "deepseek", Label: "DS", Primary: &balDim}
	got := formatAccountLine(u)
	if !strings.Contains(got, "["+colorRed+"]") {
		t.Errorf("balance<=0 should render red dot, got: %q", got)
	}
}
```

（若 `account_list_test.go` 未 import `domain`/`strings`，补 import。）

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/adapters/ui/ -run 'TestFormatMoneyShort|TestFormatAccountLineBalance' -v`
Expected: FAIL（`formatMoneyShort` undefined / 余额型行显示 `N/A`）

- [ ] **Step 3: 实现 formatMoneyShort + 改 formatAccountLine**

在 `account_list.go` 加 helper（需 import `math`）：

```go
// formatMoneyShort 余额短格式（列表用，1 位小数，>1000 缩写 k）。
func formatMoneyShort(balance float64, currency string) string {
	sym := currencySymbol(currency)
	if math.Abs(balance) >= 1000 {
		return fmt.Sprintf("%s%.1fk", sym, balance/1000)
	}
	return fmt.Sprintf("%s%.1f", sym, balance)
}
```

`currencySymbol` 放 `account_details.go`（Task 5 定义）；本 task 先在 `account_list.go` 暂加同函数？—— 为避免重复，**本 task 先在 `account_details.go` 顺手加 `currencySymbol`**（见下），`account_list.go` 直接调用（同 package）。

改 `formatAccountLine` 开头的 pctStr/dot/dotCol 计算块。**原代码**：

```go
	pctStr, dot := "N/A", "○"
	if u.Primary != nil {
		pctStr = fmt.Sprintf("%d%%", int(u.Primary.PercentUsed))
		dot = "●"
	}
	pct := primaryPercent(u)
	dotCol := StatusColor(pct)
```

**改为**：

```go
	pctStr, dot := "N/A", "○"
	dotCol := colorGray // N/A 默认灰点
	if u.Primary != nil && u.Primary.Currency != "" {
		// 余额型：显示余额 + 绿/红点（按余额正负）
		pctStr = formatMoneyShort(u.Primary.Balance, u.Primary.Currency)
		dot = "●"
		if u.Primary.Balance > 0 {
			dotCol = colorGreen
		} else {
			dotCol = colorRed
		}
	} else if u.Primary != nil {
		// 配额型：百分比 + StatusColor
		pctStr = fmt.Sprintf("%d%%", int(u.Primary.PercentUsed))
		dot = "●"
		dotCol = StatusColor(u.Primary.PercentUsed)
	}
	pct := primaryPercent(u) // 余额型 PercentUsed=-1 → renderBar(-1,4) 自然灰条
```

并在 `account_details.go` 加（Task 5 会复用）：

```go
// currencySymbol 返回货币符号：CNY→¥、USD→$、未知→空。
func currencySymbol(currency string) string {
	switch currency {
	case "CNY":
		return "¥"
	case "USD":
		return "$"
	default:
		return ""
	}
}
```

补 `account_list.go` import `"math"`。

- [ ] **Step 4: 运行测试，确认通过**

Run: `go test ./internal/adapters/ui/ -run 'TestFormatMoneyShort|TestFormatAccountLineBalance' -v`
Expected: PASS

- [ ] **Step 5: UI 回归 + 提交**

Run: `go test -race ./internal/adapters/ui/`
Expected: 全绿（配额型行不受影响，因分支条件 `Currency != ""` 仅余额型触发）

```bash
git add internal/adapters/ui/account_list.go internal/adapters/ui/account_details.go internal/adapters/ui/account_list_test.go
git commit -m "feat(ui): balance-type list row (money short + green/red dot + gray bar)"
```

---

## Task 5: 余额型详情渲染 + formatMoney

**Files:**
- Modify: `internal/adapters/ui/account_details.go`（`renderDimension`）
- Test: `internal/adapters/ui/account_details_test.go`（新增余额型用例）

**Interfaces:**
- Consumes: Task 1 的 `UsageDimension.Balance`/`Currency`、Task 4 的 `currencySymbol`
- Produces: `formatMoney(balance, currency) string`（详情用，2 位小数）

- [ ] **Step 1: 写失败测试**

在 `account_details_test.go` 末尾追加（package `ui`，需 import `domain`）：

```go
// TestFormatMoney 验证余额详情格式（2 位小数）。
func TestFormatMoney(t *testing.T) {
	if got := formatMoney(49.58894, "CNY"); got != "¥49.59" {
		t.Errorf("formatMoney(49.58894,CNY) = %q, want ¥49.59", got)
	}
	if got := formatMoney(3.0, "USD"); got != "$3.00" {
		t.Errorf("formatMoney(3.0,USD) = %q, want $3.00", got)
	}
}

// TestRenderDimensionBalance 验证余额型维度：显示 Balance 行，不画进度条（无 █/░/N/A%）。
func TestRenderDimensionBalance(t *testing.T) {
	dim := domain.UsageDimension{Name: "可用余额", Balance: 49.58894, Currency: "CNY", PercentUsed: -1}
	got := renderDimension(dim)

	if !strings.Contains(got, "可用余额") {
		t.Errorf("should contain dim name, got: %q", got)
	}
	if !strings.Contains(got, "¥49.59") {
		t.Errorf("should contain Balance ¥49.59, got: %q", got)
	}
	if strings.Contains(got, "█") || strings.Contains(got, "░") {
		t.Errorf("balance dim should NOT render progress bar, got: %q", got)
	}
	if strings.Contains(got, "N/A") {
		t.Errorf("balance dim should NOT show N/A percent, got: %q", got)
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/adapters/ui/ -run 'TestFormatMoney|TestRenderDimensionBalance' -v`
Expected: FAIL（`formatMoney` undefined；余额型仍画灰条+N/A）

- [ ] **Step 3: 实现 formatMoney + 改 renderDimension**

在 `account_details.go` 加（紧邻 `currencySymbol`）：

```go
// formatMoney 余额详情格式（2 位小数）。
func formatMoney(balance float64, currency string) string {
	return fmt.Sprintf("%s%.2f", currencySymbol(currency), balance)
}
```

在 `renderDimension` 中，写完维度名之后、画进度条之前插入余额型分支。**原代码**（写完 name 后直接）：

```go
	b.WriteString(fmt.Sprintf("  [%s::b]%s[-]\n", colorPrimary, name))

	// 进度条 + 百分比：独立一行。
	pct := dim.PercentUsed
```

**改为**（name 后插入余额型早返回）：

```go
	b.WriteString(fmt.Sprintf("  [%s::b]%s[-]\n", colorPrimary, name))

	// 余额型：只显示 Balance 行，不画进度条（余额无进度语义）。
	if dim.Currency != "" {
		b.WriteString(fmt.Sprintf("    [%s]%-10s[-]  [%s]%s[-]\n",
			colorSecondary, "Balance:", colorPrimary, formatMoney(dim.Balance, dim.Currency)))
		b.WriteString("\n")
		return b.String()
	}

	// 配额型（现有进度条逻辑不变）。
	pct := dim.PercentUsed
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `go test ./internal/adapters/ui/ -run 'TestFormatMoney|TestRenderDimensionBalance' -v`
Expected: PASS

- [ ] **Step 5: UI 回归 + 提交**

Run: `go test -race ./internal/adapters/ui/`
Expected: 全绿

```bash
git add internal/adapters/ui/account_details.go internal/adapters/ui/account_details_test.go
git commit -m "feat(ui): balance-type details dimension (no progress bar, money format)"
```

---

## Task 6: 装配（vendorOptions + 配色 + Registry + spec 同步）

**Files:**
- Modify: `internal/adapters/ui/account_form.go`（`vendorOptions`）
- Modify: `internal/adapters/ui/theme.go`（`vendorColor`）
- Modify: `internal/adapters/ui/theme_test.go`（`TestVendorTag_KnownVendors` 表）
- Modify: `cmd/main.go`（Registry + import）
- Modify: `docs/superpowers/specs/2026-07-27-fleetboard-design.md`（§9.2 配色清单）

**Interfaces:**
- Consumes: Task 2/3 的 `kimi.New()`/`deepseek.New()`
- Produces: 新 vendor 在下拉、配色、Registry 全链路可见

- [ ] **Step 1: 写失败测试（theme_test 补行）**

`theme_test.go` 的 `TestVendorTag_KnownVendors` 的 `cases` 切片，在 `{"copilot", ...}` 后追加：

```go
		{"deepseek", "#2563EB", "#FFFFFF"},
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `go test ./internal/adapters/ui/ -run TestVendorTag_KnownVendors -v`
Expected: FAIL（`VendorTag("deepseek")` 返回 gray fallback，与 `#2563EB/#FFFFFF` 不符）

- [ ] **Step 3: 实现（四处同步）**

**(a) `theme.go`** `vendorColor` map，在 `"copilot"` 行后加：

```go
	"deepseek":  {"#2563EB", "#FFFFFF"},
```

**(b) `account_form.go`** `vendorOptions`：

```go
var vendorOptions = []string{"glm", "minimax", "kimi", "deepseek"}
```

**(c) `cmd/main.go`** import 块加（在 `minimax` import 后）：

```go
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/deepseek"
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/kimi"
```

Registry 行（原 `providers.NewRegistry(glm.New(), minimax.New())`）改为：

```go
	reg := providers.NewRegistry(glm.New(), minimax.New(), kimi.New(), deepseek.New())
```

**(d) `docs/superpowers/specs/2026-07-27-fleetboard-design.md`** §9.2 的 vendor tag 配色行（原 `glm=#7C3AED...copilot=#0969DA` 那行）末尾追加 `、deepseek=#2563EB（蓝）`。

- [ ] **Step 4: 运行测试 + 构建，确认通过**

Run: `go test -race ./... && go build ./...`
Expected: 全绿 + 构建成功

- [ ] **Step 5: 手动联调验证（可选但推荐）**

配一个真实或 mock 账号到 `~/.fleetboard/config.yaml`，`make build && ./bin/fleetboard`，确认：
- Vendor 下拉含 kimi/deepseek
- 列表行余额型显示 `¥XX.X` + 绿点 + 灰条
- 详情页余额型显示 `Balance: ¥XX.XX`，无进度条

- [ ] **Step 6: 提交**

```bash
git add internal/adapters/ui/account_form.go internal/adapters/ui/theme.go internal/adapters/ui/theme_test.go cmd/main.go docs/superpowers/specs/2026-07-27-fleetboard-design.md
git commit -m "feat(ui): wire kimi/deepseek vendors (dropdown, color, registry)"
```

---

## Self-Review（写计划后自检）

**1. Spec 覆盖：**
- §3 数据模型扩展 → Task 1 ✓
- §4.1 Kimi adapter → Task 2 ✓
- §4.2 DeepSeek adapter → Task 3 ✓
- §5.1 列表余额型 → Task 4 ✓
- §5.2 详情余额型 → Task 5 ✓
- §6 触点（form/theme/main）→ Task 6 ✓
- §7 配色 → Task 6 ✓
- §8 测试策略 → 各 Task 内 TDD ✓
- §10 开放问题 → 已在 spec 标注，不影响实现

**2. 占位符扫描：** 无 TBD/TODO；`<LICENSE_HEADER>` 指 Global Constraints 的固定模板（boilerplate，非实现占位）；每步均含可执行代码或命令。

**3. 类型一致性：** `formatMoneyShort`/`formatMoney`/`currencySymbol` 在 Task 4/5 定义后被同 package 复用，签名一致；`kimi.New()`/`deepseek.New()` 在 Task 2/3 定义、Task 6 调用，一致；`UsageDimension.Balance`/`Currency` 在 Task 1 定义、后续 task 消费，一致。
