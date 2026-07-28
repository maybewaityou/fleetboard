# SiliconFlow（硅基流动）provider 接入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 `siliconflow` provider，对接硅基流动 `/v1/user/info`，把可用余额作为主余额展示、详情页补充「充值 / 总额」两行。

**Architecture:** 沿用六边形（ports & adapters），完全仿 `deepseek` adapter 模板（同为单凭证、纯余额型）。`UsageDimension` 加两个余额信息字段 `ChargeBalance`/`TotalBalance`（不复用 `Granted`/`ToppedUp`，避免破坏其「相加==Balance」约定）。adapter 解 `{code,message,status,data}` 信封，主余额严格解析、细分容错，错误路径仍返回带账号字段的 `ProviderUsage`。

**Tech Stack:** Go 1.24.6 · 标准库 `net/http`+`encoding/json`+`strconv`（无新依赖）· `tview` TUI · httptest golden 测试 · `gofumpt`/`go vet`/`golangci-lint`。

## Global Constraints

- Go 1.24.6；零新增第三方依赖（仅标准库 + 既有 tview/cobra/zap/yaml）。
- provider 标识严格小写 `"siliconflow"`；`Provider()` 返回此值。
- 货币固定 `"CNY"`（接口无货币字段，隐含人民币）。
- 金额字段均为 **string**，`strconv.ParseFloat` 解析；**主余额 `balance` 严格**（失败报 Err），**细分 `chargeBalance`/`totalBalance` 容错**（失败留零值，`_` 忽略 err）。
- 响应信封 `code == 20000` 为成功（区别于 HTTP 状态码）；非 20000 返回 Err。
- 账号状态：`data.status == "normal"` → `Status="active"`；其余非空取值保留原值；空串不填。
- 默认 base `https://api.siliconflow.cn`，端点 `/v1/user/info`，超时 10s，单凭证 `Authorization: Bearer <key>`。
- 鉴权 token 经账号 `token_env` 指定的环境变量读取（与 deepseek 一致）。
- 每个任务 TDD（先红后绿），结尾 `go build ./...` + `make test` 绿后 commit；commit message 用 conventional commits，结尾 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 不依赖 `data.name`/`data.image`/`data.email`（官方 2025-06-11 起不再返回）。

---

## File Structure

| 文件 | 动作 | 职责 |
|------|------|------|
| `internal/core/domain/provider_usage.go` | Modify | `UsageDimension` 加 `ChargeBalance`/`TotalBalance` 两字段 |
| `internal/core/domain/provider_usage_test.go` | Modify | 新字段零值/赋值测试 |
| `internal/adapters/providers/siliconflow/siliconflow.go` | Create | adapter：常量/类型/`Provider`/`New`/`FetchUsage` |
| `internal/adapters/providers/siliconflow/siliconflow_test.go` | Create | httptest golden + 错误守卫测试 |
| `cmd/main.go` | Modify | import + `NewRegistry(...)` 追加 `siliconflow.New()` |
| `internal/adapters/ui/account_form.go` | Modify | `providerOptions` 追加 `"siliconflow"`（末尾） |
| `internal/adapters/ui/account_form_test.go` | Modify | 断言 `providerOptions` 含 `siliconflow` |
| `internal/adapters/ui/account_details.go` | Modify | `renderDimension` 余额分支加 Charged/Total 两行 |
| `internal/adapters/ui/account_details_test.go` | Modify | SiliconFlow 细分渲染测试 |
| `README.md` / `README.zh-CN.md` | Modify | providers 表格 + 配置示例 |

---

## Task 1: domain — `UsageDimension` 加余额信息字段

**Files:**
- Modify: `internal/core/domain/provider_usage.go`（`UsageDimension` struct，`ToppedUp` 字段之后）
- Test: `internal/core/domain/provider_usage_test.go`

**Interfaces:**
- Produces: `UsageDimension.ChargeBalance float64`、`UsageDimension.TotalBalance float64`（零值无害，Task 2/5 消费）

- [ ] **Step 1: 写失败测试**

在 `provider_usage_test.go` 末尾追加：

```go
// TestUsageDimensionSiliconFlowFields 验证 SiliconFlow 余额信息字段可赋值/读取，零值默认。
// 注意：provider_usage_test.go 为 package domain（内部测试），直接用 UsageDimension，无 domain. 前缀。
func TestUsageDimensionSiliconFlowFields(t *testing.T) {
	d := UsageDimension{
		Name: "Available balance", Balance: 0.88, Currency: "CNY",
		ChargeBalance: 88.0, TotalBalance: 88.88,
	}
	if d.ChargeBalance != 88.0 {
		t.Errorf("ChargeBalance = %v, want 88.0", d.ChargeBalance)
	}
	if d.TotalBalance != 88.88 {
		t.Errorf("TotalBalance = %v, want 88.88", d.TotalBalance)
	}
	// 零值默认：未填时为 0（UI 据此跳过渲染）
	zero := UsageDimension{}
	if zero.ChargeBalance != 0 || zero.TotalBalance != 0 {
		t.Errorf("zero-value fields should be 0, got charge=%v total=%v", zero.ChargeBalance, zero.TotalBalance)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/core/domain/ -run TestUsageDimensionSiliconFlowFields -v`
Expected: FAIL（`d.ChargeBalance undefined` / `unknown field`）

- [ ] **Step 3: 加字段**

在 `provider_usage.go` 的 `UsageDimension` struct 中，`ToppedUp float64` 字段之后（`MoneyLimit` 之前）插入：

```go
	// SiliconFlow 余额信息（adapter 填充，UI 读取）。零值=无，UI 不渲染。
	// 与 Granted/ToppedUp（剩余拆分，相加=Balance）语义不同：这里是 API 原值，
	// 不做相加约定——官方未保证 chargeBalance/totalBalance 与 balance 的恒等关系。
	// 仅 siliconflow provider 填充；配额型与其他余额型 provider 零值=无。
	ChargeBalance float64
	TotalBalance  float64
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/core/domain/ -run TestUsageDimensionSiliconFlowFields -v`
Expected: PASS

- [ ] **Step 5: 全量构建 + 测试**

Run: `go build ./... && go test ./...`
Expected: 全绿（新字段零值，不影响既有测试）

- [ ] **Step 6: Commit**

```bash
git add internal/core/domain/provider_usage.go internal/core/domain/provider_usage_test.go
git commit -m "feat(domain): add ChargeBalance/TotalBalance to UsageDimension

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: siliconflow adapter — 骨架 + 成功路径

**Files:**
- Create: `internal/adapters/providers/siliconflow/siliconflow.go`
- Test: `internal/adapters/providers/siliconflow/siliconflow_test.go`

**Interfaces:**
- Consumes: `domain.Account`（`ID`/`Provider`/`Label`/`BaseURL`/`TokenEnv`）、`domain.UsageDimension.ChargeBalance`/`TotalBalance`（Task 1）、`ports.UsageProvider`
- Produces: `siliconflow.New() *Provider`、`(*Provider).Provider() string`（返回 `"siliconflow"`）、`(*Provider).FetchUsage(ctx, acc) (domain.ProviderUsage, error)` —— Task 4 装配消费

- [ ] **Step 1: 写失败测试（Provider 标识 + golden 成功路径）**

创建 `internal/adapters/providers/siliconflow/siliconflow_test.go`：

```go
// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in License boilerplate...（复制 deepseek_test.go:1-13 的 Apache 头）

package siliconflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

// goldenPayload 是 SiliconFlow /v1/user/info 响应金样本。金额是 string。
const goldenPayload = `{"code":20000,"message":"OK","status":true,"data":{"id":"user123","balance":"0.88","status":"normal","chargeBalance":"88.00","totalBalance":"88.88"}}`

func TestProviderReturnsSiliconFlow(t *testing.T) {
	if got := New().Provider(); got != "siliconflow" {
		t.Fatalf("Provider() = %q, want siliconflow", got)
	}
}

// TestFetchUsageGolden：
//
//	(a) Authorization = "Bearer KEY123"，Content-Type = application/json
//	(b) 路径 = /v1/user/info
//	(c) Balance = 0.88（"0.88" ParseFloat，无换算），Currency = CNY，PercentUsed = -1
//	(d) ChargeBalance = 88.0，TotalBalance = 88.88
//	(e) Primary 指向该维度
//	(f) Status = active（data.status="normal"）
func TestFetchUsageGolden(t *testing.T) {
	var gotAuth, gotCT, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenPayload)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "KEY123")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}

	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// (a)(b) 鉴权 + Content-Type + 路径
	if gotAuth != "Bearer KEY123" {
		t.Errorf("Authorization = %q, want Bearer KEY123", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotPath != "/v1/user/info" {
		t.Errorf("path = %q, want /v1/user/info", gotPath)
	}

	// (c)(d) 余额型维度 + 细分
	if len(u.Dimensions) != 1 {
		t.Fatalf("len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	d := u.Dimensions[0]
	if d.Name != "Available balance" {
		t.Errorf("dim.Name = %q, want Available balance", d.Name)
	}
	if d.Balance != 0.88 {
		t.Errorf("dim.Balance = %v, want 0.88 (balance parsed)", d.Balance)
	}
	if d.Currency != "CNY" {
		t.Errorf("dim.Currency = %q, want CNY", d.Currency)
	}
	if d.PercentUsed != -1 {
		t.Errorf("dim.PercentUsed = %v, want -1", d.PercentUsed)
	}
	if d.Source != "api-balanced" {
		t.Errorf("dim.Source = %q, want api-balanced", d.Source)
	}
	if d.ChargeBalance != 88.0 {
		t.Errorf("dim.ChargeBalance = %v, want 88.0 (chargeBalance)", d.ChargeBalance)
	}
	if d.TotalBalance != 88.88 {
		t.Errorf("dim.TotalBalance = %v, want 88.88 (totalBalance)", d.TotalBalance)
	}

	// (e) Primary 指向余额维度
	if u.Primary == nil || u.Primary.Name != "Available balance" {
		t.Errorf("Primary = %+v, want Available balance dim", u.Primary)
	}

	// (f) 账号状态：data.status="normal" → Status="active"
	if u.Status != "active" {
		t.Errorf("Status = %q, want active (data.status=normal)", u.Status)
	}

	// 账号字段 + Basic Info
	if u.AccountID != "sf" || u.Provider != "siliconflow" || u.Label != "SiliconFlow" {
		t.Errorf("top fields wrong: %+v", u)
	}
	if u.Endpoint != "/v1/user/info" {
		t.Errorf("Endpoint = %q, want /v1/user/info", u.Endpoint)
	}
	if u.BaseURL != srv.URL {
		t.Errorf("BaseURL = %q, want %s", u.BaseURL, srv.URL)
	}
	if u.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set")
	}
	if u.FetchedAt.After(time.Now()) {
		t.Error("FetchedAt must not be in the future")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/adapters/providers/siliconflow/ -v`
Expected: 编译失败 / FAIL（`siliconflow.New undefined`）

- [ ] **Step 3: 实现 adapter（完整 FetchUsage，含守卫）**

创建 `internal/adapters/providers/siliconflow/siliconflow.go`：

```go
// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// ...（复制 deepseek.go:1-13 的 Apache 头）

// Package siliconflow 实现 ports.UsageProvider，对接硅基流动账户信息接口。
//
// 真实接口契约（docs.siliconflow.com/cn/api-reference/userinfo/get-user-info）：
//   - GET {BaseURL}/v1/user/info，默认 BaseURL = https://api.siliconflow.cn。
//   - 鉴权头：Authorization: Bearer <API_KEY>。
//
// 响应 {code,message,status,data:{balance,chargeBalance,totalBalance,status,...}}。
// code==20000 为成功（业务码，区别于 HTTP 状态码）。三个金额字段全是 string（需
// strconv.ParseFloat，单位即元，无换算）。balance 为当前可用余额；chargeBalance/
// totalBalance 为充值/总额（API 原值，不做相加约定，区别于 deepseek 的 granted/topped_up）。
package siliconflow

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
	defaultBaseURL = "https://api.siliconflow.cn"
	usagePath      = "/v1/user/info"
	httpTimeout    = 10 * time.Second

	sourceTag     = "api-balanced"
	nameAvailable = "Available balance"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 SiliconFlow 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 SiliconFlow Provider，HTTP 客户端超时 10s。
func New() *Provider {
	return &Provider{hc: &http.Client{Timeout: httpTimeout}}
}

// Provider 返回厂商标识。
func (p *Provider) Provider() string { return "siliconflow" }

// apiResp 是 SiliconFlow /v1/user/info 响应信封。金额字段为 string。
type apiResp struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Status  bool     `json:"status"`
	Data    userInfo `json:"data"`
}

// userInfo 是 data 对象。金额字段为十进制字符串。
type userInfo struct {
	ID            string `json:"id"`
	Balance       string `json:"balance"`
	Status        string `json:"status"`
	ChargeBalance string `json:"chargeBalance"`
	TotalBalance  string `json:"totalBalance"`
}

// FetchUsage 拉取该账号余额，返回单维度余额型 ProviderUsage。
// 出错时 ProviderUsage 仍被填充（账号字段 + FetchedAt + Err）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "siliconflow",
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+usagePath, http.NoBody)
	if err != nil {
		u.Err = fmt.Errorf("siliconflow: build request: %w", err)
		return u, u.Err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		u.Err = fmt.Errorf("siliconflow: request: %w", err)
		return u, u.Err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.Err = fmt.Errorf("siliconflow: HTTP %d", resp.StatusCode)
		return u, u.Err
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("siliconflow: decode response: %w", err)
		return u, u.Err
	}
	// 业务信封校验：code==20000 为成功（区别于 HTTP 状态码）。
	if r.Code != 20000 {
		u.Err = fmt.Errorf("siliconflow: code %d: %s", r.Code, r.Message)
		return u, u.Err
	}

	// 账号状态：先于金额解析填充，确保错误路径也携带状态。
	// normal→active；其余非空取值保留原值（frozen/banned 等）；空串不填。
	if r.Data.Status == "normal" {
		u.Status = "active"
	} else if r.Data.Status != "" {
		u.Status = r.Data.Status
	}

	// 主余额严格解析：失败即整体失败。
	balance, err := strconv.ParseFloat(r.Data.Balance, 64)
	if err != nil {
		u.Err = fmt.Errorf("siliconflow: parse balance %q: %w", r.Data.Balance, err)
		return u, u.Err
	}
	// 细分容错解析：ParseFloat 失败返回 0 值，用 _ 忽略 err——
	// 主余额已成功，细分缺失（=0）不致命，UI 自动跳过零值行。
	charge, _ := strconv.ParseFloat(r.Data.ChargeBalance, 64)
	total, _ := strconv.ParseFloat(r.Data.TotalBalance, 64)

	u.Dimensions = []domain.UsageDimension{{
		Name:          nameAvailable,
		Balance:       balance,
		Currency:      "CNY",
		PercentUsed:   -1,
		Source:        sourceTag,
		ChargeBalance: charge,
		TotalBalance:  total,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/adapters/providers/siliconflow/ -v`
Expected: `TestProviderReturnsSiliconFlow` + `TestFetchUsageGolden` PASS

- [ ] **Step 5: 全量构建 + 测试**

Run: `go build ./... && go test ./...`
Expected: 全绿

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/providers/siliconflow/siliconflow.go internal/adapters/providers/siliconflow/siliconflow_test.go
git commit -m "feat(siliconflow): add SiliconFlow balance provider

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: adapter 错误与边界守卫测试

**Files:**
- Test: `internal/adapters/providers/siliconflow/siliconflow_test.go`（追加）

**Interfaces:**
- Consumes: Task 2 的 `FetchUsage` 实现及其守卫（HTTP/decode/code/ParseFloat）
- Produces: 无（纯回归覆盖）

> 预期：HTTP 非 2xx、decode 失败、主余额解析失败、信封 code≠20000 等守卫已在 Task 2 实现，下列测试多数应直接通过；它们是回归保护。若某项失败，说明对应守卫缺失，补实现使其通过。

- [ ] **Step 1: 追加错误/边界测试**

在 `siliconflow_test.go` 末尾追加（每个测试用独立 httptest server + `t.Setenv`，仿 `deepseek_test.go` 风格）：

```go
// TestFetchUsageNon200 验证非 2xx 被状态守卫拦截，错误路径仍填充账号字段。
func TestFetchUsageNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"code":40100,"message":"invalid api key"}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "BAD")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
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
	if u.Primary != nil {
		t.Errorf("Primary should be nil on HTTP error, got %+v", u.Primary)
	}
	if u.AccountID != "sf" || u.Provider != "siliconflow" || u.Label != "SiliconFlow" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}

// TestFetchUsageBadJSON 验证解码失败返回错误且填充 u.Err。
func TestFetchUsageBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not-json`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on decode error")
	}
}

// TestFetchUsageBadEnvelope 验证 HTTP 200 但业务 code≠20000 被信封守卫拦截。
func TestFetchUsageBadEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":40100,"message":"invalid api key","status":false,"data":{}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for code!=20000, got nil")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on bad envelope")
	}
}

// TestFetchUsageBadBalance 验证主余额非数字时整体失败（严格解析）。
func TestFetchUsageBadBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":20000,"message":"OK","status":true,"data":{"balance":"oops","status":"normal","chargeBalance":"88.00","totalBalance":"88.88"}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for non-numeric balance, got nil")
	}
	if len(u.Dimensions) != 0 {
		t.Errorf("Dimensions should be empty when balance parse fails, got %+v", u.Dimensions)
	}
}

// TestFetchUsageBadChargeBalance 验证细分解析失败不致命：主余额照常成功，细分留零值。
func TestFetchUsageBadChargeBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":20000,"message":"OK","status":true,"data":{"balance":"5.00","status":"normal","chargeBalance":"oops","totalBalance":"oops"}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err (bad chargeBalance must NOT fail): %v", err)
	}
	d := u.Dimensions[0]
	if d.Balance != 5.0 {
		t.Errorf("dim.Balance = %v, want 5.0 (balance preserved)", d.Balance)
	}
	if d.ChargeBalance != 0 {
		t.Errorf("dim.ChargeBalance = %v, want 0 (parse failure → zero)", d.ChargeBalance)
	}
	if d.TotalBalance != 0 {
		t.Errorf("dim.TotalBalance = %v, want 0 (parse failure → zero)", d.TotalBalance)
	}
}

// TestFetchUsageStatusNonNormal 验证非 normal 状态保留原值字符串。
func TestFetchUsageStatusNonNormal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":20000,"message":"OK","status":true,"data":{"balance":"1.00","status":"frozen","chargeBalance":"0","totalBalance":"1.00"}}`)
	}))
	defer srv.Close()

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.Status != "frozen" {
		t.Errorf("Status = %q, want frozen (preserved original)", u.Status)
	}
}

// TestFetchUsageServerDown 验证传输错误透传 + 账号字段仍填充。
func TestFetchUsageServerDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, goldenPayload)
	}))
	srv.Close() // 关闭 server，触发传输错误

	t.Setenv("SILICONFLOW_API_KEY", "K")
	acc := domain.Account{ID: "sf", Provider: "siliconflow", Label: "SiliconFlow", TokenEnv: "SILICONFLOW_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error when server down")
	}
	if u.Err == nil {
		t.Error("u.Err should be set on transport error")
	}
	if u.AccountID != "sf" || u.Provider != "siliconflow" || u.Label != "SiliconFlow" {
		t.Errorf("error-path fields wrong: %+v", u)
	}
}
```

- [ ] **Step 2: 运行测试验证通过**

Run: `go test ./internal/adapters/providers/siliconflow/ -v`
Expected: 全部 PASS（守卫已在 Task 2 实现；若 `TestFetchUsageBadEnvelope` 失败，说明 code 守卫缺失——回 Task 2 的 `siliconflow.go` 补 `if r.Code != 20000` 分支后重跑）

- [ ] **Step 3: 全量测试（含 -race）**

Run: `go test -race ./...`
Expected: 全绿

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/providers/siliconflow/siliconflow_test.go
git commit -m "test(siliconflow): cover error guards and edge cases

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: 装配 — registry 注册 + 表单下拉

**Files:**
- Modify: `cmd/main.go`（import 块 `:31-54` + `NewRegistry` 调用 `:124`）
- Modify: `internal/adapters/ui/account_form.go:37`（`providerOptions`）
- Test: `internal/adapters/ui/account_form_test.go`

**Interfaces:**
- Consumes: Task 2 的 `siliconflow.New()`
- Produces: `siliconflow` 在运行时 registry 可用、表单可选

> 关键：`providerOptions` 追加 `"siliconflow"` 到**末尾**（idx=6），保持现有 index 不变（newapi 仍 idx=5，`account_form_test.go:91/112` 的 `SetCurrentOption(5)` 不受影响）。

- [ ] **Step 1: 写失败测试（providerOptions 含 siliconflow）**

在 `account_form_test.go` 末尾追加：

```go
// TestProviderOptionsContainsSiliconFlow 验证 siliconflow 已加入下拉选项（末尾）。
func TestProviderOptionsContainsSiliconFlow(t *testing.T) {
	found := false
	for _, v := range providerOptions {
		if v == "siliconflow" {
			found = true
		}
	}
	if !found {
		t.Errorf("providerOptions missing siliconflow: %v", providerOptions)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/adapters/ui/ -run TestProviderOptionsContainsSiliconFlow -v`
Expected: FAIL（`providerOptions missing siliconflow`）

- [ ] **Step 3: 改 `account_form.go`**

把 `internal/adapters/ui/account_form.go:37`：

```go
var providerOptions = []string{"glm", "minimax", "kimi", "deepseek", "sub2api", "newapi"}
```

改为：

```go
var providerOptions = []string{"glm", "minimax", "kimi", "deepseek", "sub2api", "newapi", "siliconflow"}
```

- [ ] **Step 4: 改 `cmd/main.go` — import**

在 import 块中，`newapi` 与 `sub2api` 之间插入（字母序 `siliconflow` < `sub2api`），即 `:46-47` 之间：

```go
	"github.com/maybewaityou/fleetboard/internal/adapters/providers/siliconflow"
```

（最终顺序：deepseek / glm / kimi / minimax / newapi / siliconflow / sub2api）

- [ ] **Step 5: 改 `cmd/main.go` — NewRegistry**

把 `:124`：

```go
	reg := providers.NewRegistry(glm.New(), minimax.New(), kimi.New(), deepseek.New(), sub2api.New(), newapi.New())
```

改为：

```go
	reg := providers.NewRegistry(glm.New(), minimax.New(), kimi.New(), deepseek.New(), sub2api.New(), newapi.New(), siliconflow.New())
```

- [ ] **Step 6: 运行测试 + 构建验证**

Run: `go test ./internal/adapters/ui/ -run TestProviderOptionsContainsSiliconFlow -v && go build ./...`
Expected: 测试 PASS + 构建成功

- [ ] **Step 7: 全量测试**

Run: `go test ./...`
Expected: 全绿

- [ ] **Step 8: Commit**

```bash
git add cmd/main.go internal/adapters/ui/account_form.go internal/adapters/ui/account_form_test.go
git commit -m "feat: wire siliconflow provider into registry and form

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 5: UI — 详情页 Charged/Total 细分行

**Files:**
- Modify: `internal/adapters/ui/account_details.go`（`renderDimension` 余额分支 `:213-226`）
- Test: `internal/adapters/ui/account_details_test.go`

**Interfaces:**
- Consumes: Task 1 的 `UsageDimension.ChargeBalance`/`TotalBalance`
- Produces: SiliconFlow 余额维度在详情页渲染 Charged/Total 行

- [ ] **Step 1: 写失败测试**

在 `account_details_test.go` 末尾追加（仿 `TestRenderDimensionBalanceBreakdown:138`）：

```go
// TestRenderDimensionSiliconFlowBreakdown 验证 SiliconFlow 余额维度输出 Charged/Total 行。
func TestRenderDimensionSiliconFlowBreakdown(t *testing.T) {
	dim := domain.UsageDimension{
		Name: "Available balance", Balance: 0.88, Currency: "CNY",
		ChargeBalance: 88.0, TotalBalance: 88.88, PercentUsed: -1,
	}
	got := renderDimension(dim)
	for _, want := range []string{"Balance:", "¥0.88", "Charged:", "¥88.00", "Total:", "¥88.88"} {
		if !strings.Contains(got, want) {
			t.Errorf("siliconflow breakdown missing %q, got: %q", want, got)
		}
	}
}

// TestRenderDimensionSiliconFlowZeroBreakdown 验证零值细分不渲染 Charged/Total 行。
func TestRenderDimensionSiliconFlowZeroBreakdown(t *testing.T) {
	dim := domain.UsageDimension{
		Name: "Available balance", Balance: 5.0, Currency: "CNY", PercentUsed: -1,
	}
	got := renderDimension(dim)
	if strings.Contains(got, "Charged:") {
		t.Errorf("zero ChargeBalance should not render Charged line, got: %q", got)
	}
	if strings.Contains(got, "Total:") {
		t.Errorf("zero TotalBalance should not render Total line, got: %q", got)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/adapters/ui/ -run TestRenderDimensionSiliconFlow -v`
Expected: `TestRenderDimensionSiliconFlowBreakdown` FAIL（缺 `Charged:` / `Total:`）

- [ ] **Step 3: 加渲染分支**

在 `account_details.go` 的 `renderDimension` 余额分支，`ToppedUp` 判断块之后（`:223` `}` 之后、`:224` `b.WriteString("\n")` 之前）插入：

```go
		if dim.ChargeBalance != 0 {
			fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
				colorSecondary, "Charged:", colorPrimary, formatMoney(dim.ChargeBalance, dim.Currency))
		}
		if dim.TotalBalance != 0 {
			fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
				colorSecondary, "Total:", colorPrimary, formatMoney(dim.TotalBalance, dim.Currency))
		}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/adapters/ui/ -run TestRenderDimensionSiliconFlow -v`
Expected: 两个测试 PASS

- [ ] **Step 5: 全量测试**

Run: `go test ./...`
Expected: 全绿

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/ui/account_details.go internal/adapters/ui/account_details_test.go
git commit -m "feat(ui): render SiliconFlow Charged/Total balance breakdown

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 6: README 文档 + 全量验证收尾

**Files:**
- Modify: `README.md`（providers 表格 + 配置示例）
- Modify: `README.zh-CN.md`（同）

**Interfaces:**
- Consumes: 全部前序任务

- [ ] **Step 1: 更新 `README.md` providers 表格**

在 `README.md` 的 providers 表格中，`deepseek` 行之后插入一行（保持表格列对齐）：

```markdown
| `siliconflow` | Balance | available balance (CNY) + charged/total | optional    |
```

- [ ] **Step 2: 更新 `README.md` 配置示例**

在配置示例的 `accounts:` 列表中追加（`kimi` 示例之后）：

```yaml
  - id: siliconflow-main
    provider: siliconflow
    label: SiliconFlow main
    token_env: SILICONFLOW_API_KEY
```

- [ ] **Step 3: 更新 `README.zh-CN.md`**

对 `README.zh-CN.md` 做对应中文化改动：表格加 `siliconflow` 行（描述：可用余额（CNY）+ 充值/总额），配置示例同上。

- [ ] **Step 4: 格式化 + lint + 全量测试**

Run: `make fmt && make lint && make test`
Expected: `gofumpt`/`go fmt` 无变更（或仅格式化）、`golangci-lint` 零问题、`go test -race -cover ./...` 全绿

- [ ] **Step 5: 覆盖率抽查**

Run: `go test -cover ./internal/adapters/providers/siliconflow/`
Expected: siliconflow 包覆盖率 > 90%（adapter 逻辑简单，golden + 错误测试应几乎全覆盖）

- [ ] **Step 6: Commit**

```bash
git add README.md README.zh-CN.md
git commit -m "docs: document siliconflow provider in README

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

- [ ] **Step 7: 最终全量验证**

Run: `make quality && make test`
Expected: 全绿，工作树干净（`git status` clean）
