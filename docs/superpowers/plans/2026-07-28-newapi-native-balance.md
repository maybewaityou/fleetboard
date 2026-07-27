# new-api 原生余额与消耗接入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 new-api provider 的取数通道从伪装的 OpenAI billing 端点切换到 new-api 原生 `/api/*` 层，获取真实账户余额与近 7/30 天消耗摘要。

**Architecture:** 扩展现有 `newapi` 适配器（不新增 provider），废弃 `/v1/dashboard/billing/*`（假数据），改走 `/api/user/self`（余额）+ `/api/status`（换算因子）+ `/api/log/self/stat`（消耗）。鉴权用 `Authorization: Bearer <access_token>` + `New-Api-User: <user_id>` 双 header。`Account` 加两个字段，`ProviderUsage` 加 `Recent *RecentUsage` 摘要，详情页渲染新区块。

**Tech Stack:** Go 1.24.6 · tview/tcell（TUI）· httptest（测试）· yaml.v3（配置）· TDD（先红后绿）

## Global Constraints

- **鉴权**：每个 `/api/*` 请求必带 `Authorization: Bearer <access_token>` 与 `New-Api-User: <user_id>` 两个 header。
- **货币换算**：`USD = quota / quota_per_unit`；`quota_per_unit` 取自 `/api/status`，失败回退 `defaultQPU = 500000`。
- **错误前缀**：所有错误信息以 `newapi:` 开头；`u.Err` 非 nil 时不抑制已填字段。
- **质量门槛**：每步 `go build ./...`；每个 task 末 `go test ./...` 全绿；终态 `make quality`（gofumpt + go vet）通过。
- **文件头**：所有新建/修改的 `.go` 文件保留现有 Apache 2.0 license 头。
- **命名**：provider slug 固定 `newapi`（无连字符）；余额维度名 `Available balance`，Source `newapi`。
- **破坏性**：new-api 账号配置 `token_env` → `access_token_env` + `user_id`；版本 bump v0.2.0。

## File Structure

| 文件 | 责任 | 改动 |
|------|------|------|
| `internal/core/domain/account.go` | 账号配置模型 | 加 `AccessTokenEnv` + `UserID` 字段 |
| `internal/core/domain/provider_usage.go` | 用量结果模型 | 加 `RecentUsage` 类型 + `ProviderUsage.Recent` 字段 |
| `internal/adapters/providers/newapi/newapi.go` | new-api 适配器 | 重写：删 billing，加 4 端点 + 双 header 鉴权 + 换算 |
| `internal/adapters/providers/newapi/newapi_test.go` | 适配器测试 | 重写：删 billing 用例，加原生用例 |
| `internal/adapters/ui/account_details.go` | 详情页渲染 | 加 `renderRecent` + `Render` 接入 |
| `internal/adapters/ui/account_details_test.go` | 详情页测试 | 加 `TestRenderRecent` |
| `internal/adapters/ui/account_form.go` | 账号表单 | 加 2 字段 + provider 感知校验 + Prefill |
| `internal/adapters/ui/account_form_test.go` | 表单测试 | 加 newapi 校验 + Prefill 新字段用例 |
| `README.md` / `README.zh-CN.md` | 文档 | 更新 new-api 配置 + 迁移说明 |

---

## Task 1: domain 扩展（Account 凭证字段 + RecentUsage 摘要类型）

**Files:**
- Modify: `internal/core/domain/account.go`（`Account` 结构体，约第 35-42 行）
- Modify: `internal/core/domain/provider_usage.go`（`ProviderUsage` 结构体，约第 31-48 行）
- Test: `internal/core/domain/domain_test.go`

**Interfaces:**
- Produces: `Account.AccessTokenEnv string`、`Account.UserID string`；`RecentUsage{Window7d, Window30d float64; RPM, TPM int; Currency string}`；`ProviderUsage.Recent *RecentUsage`。Task 2-4 依赖这些类型。

- [ ] **Step 1: 写失败测试**

追加到 `internal/core/domain/domain_test.go`：

```go
// TestRecentUsageNilDefault 验证 ProviderUsage.Recent 默认 nil（UI 据此跳过区块）。
func TestRecentUsageNilDefault(t *testing.T) {
	var u ProviderUsage
	if u.Recent != nil {
		t.Errorf("Recent should default to nil, got %+v", u.Recent)
	}
}

// TestAccountNewFields 验证 new-api 凭证字段可构造。
func TestAccountNewFields(t *testing.T) {
	acc := Account{
		Provider:       "newapi",
		AccessTokenEnv: "NEWAPI_AT",
		UserID:         "16002",
	}
	if acc.AccessTokenEnv != "NEWAPI_AT" || acc.UserID != "16002" {
		t.Fatalf("new credential fields not set: %+v", acc)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/core/domain/ -run TestRecentUsageNilDefault -v`
Expected: COMPILE FAIL（`u.Recent undefined` / 字段不存在）

- [ ] **Step 3: 改 `account.go`，加两个凭证字段**

在 `Account` 结构体的 `TokenEnv` 字段后追加（保留现有字段与注释）：

```go
type Account struct {
	ID       string `yaml:"id"`
	Provider string `yaml:"provider"` // glm | minimax | kimi | ...
	Label    string `yaml:"label"`
	BaseURL  string `yaml:"base_url,omitempty"` // 可选，覆盖默认
	TokenEnv string `yaml:"token_env"`          // 环境变量名，token 从此读
	Pinned   bool   `yaml:"pinned,omitempty"`   // 置顶标记；UI 置顶排序 + 📌 marker

	// new-api 原生层凭证：当前仅 newapi provider 使用（omitempty，其他 provider 无感）。
	AccessTokenEnv string `yaml:"access_token_env,omitempty"` // 存 access_token 的环境变量名
	UserID         string `yaml:"user_id,omitempty"`          // new-api 用户 ID，作 New-Api-User header
}
```

- [ ] **Step 4: 改 `provider_usage.go`，加 RecentUsage 类型与 Recent 字段**

在 `ProviderUsage` 定义之前加新类型，并在结构体内加 `Recent` 字段：

```go
// RecentUsage 是近窗口消耗摘要（余额型 provider 的补充信息）。
// nil 表示该 provider 无此数据（UI 不渲染 Recent 区块）；零值结构体表示"拉到了但全是 0"。
type RecentUsage struct {
	Window7d  float64 // 近7天消耗（美元）
	Window30d float64 // 近30天消耗（美元）
	RPM       int     // 实时每分钟请求数
	TPM       int     // 实时每分钟 token 数
	Currency  string  // "USD"
}
```

在 `ProviderUsage` 结构体内（`Pinned bool` 字段之后、`BaseURL` 之前或末尾）加：

```go
	// 近窗口消耗摘要（adapter 填充，UI 读取）。nil = 该 provider 无此数据。
	Recent *RecentUsage
```

- [ ] **Step 5: 运行测试验证通过**

Run: `go test ./internal/core/domain/ -v`
Expected: PASS（含新测试 + 现有 domain 测试全绿）

- [ ] **Step 6: 提交**

```bash
git add internal/core/domain/account.go internal/core/domain/provider_usage.go internal/core/domain/domain_test.go
git commit -m "feat(domain): add new-api credential fields and RecentUsage type"
```

---

## Task 2: newapi 适配器完整重写（原生余额 + 消耗摘要）

**Files:**
- Modify: `internal/adapters/providers/newapi/newapi.go`（整体重写）
- Test: `internal/adapters/providers/newapi/newapi_test.go`（整体重写）

**Interfaces:**
- Consumes: `Account.AccessTokenEnv`、`Account.UserID`、`ProviderUsage.Recent`、`RecentUsage`（来自 Task 1）。
- Produces: `Provider.FetchUsage` 填充 `Dimensions[0].Balance`（真实余额）+ `Recent`（消耗摘要），仍是 `ports.UsageProvider`。

- [ ] **Step 1: 重写测试文件 `newapi_test.go`（全量替换）**

替换整个文件内容（保留 license 头）：

```go
package newapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
)

func TestProviderReturnsSlug(t *testing.T) {
	if got := New().Provider(); got != "newapi" {
		t.Fatalf("Provider() = %q, want newapi", got)
	}
}

// nativeServer 构造一个 mock，按路径分发 4 个原生端点。
// stat 的 7d/30d 用 start_timestamp 区分：7d 窗口 start 更接近 now（> now-15d）。
func nativeServer(t *testing.T, userSelfStatus, statusStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("New-Api-User"); got != "16002" {
			t.Errorf("New-Api-User header = %q, want 16002", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer AT" {
			t.Errorf("Authorization = %q, want Bearer AT", got)
		}
		switch r.URL.Path {
		case "/api/user/self":
			w.WriteHeader(userSelfStatus)
			if userSelfStatus == 200 {
				fmt.Fprint(w, `{"data":{"quota":121992688,"used_quota":69281250,"request_count":29}}`)
			}
		case "/api/status":
			w.WriteHeader(statusStatus)
			if statusStatus == 200 {
				fmt.Fprint(w, `{"data":{"quota_per_unit":500000}}`)
			}
		case "/api/log/self/stat":
			start, _ := strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
			now := time.Now().Unix()
			if start > now-15*86400 { // 7d 窗口
				fmt.Fprint(w, `{"data":{"quota":25600000,"rpm":3,"tpm":1200}}`) // 51.20 USD
			} else { // 30d 窗口
				fmt.Fprint(w, `{"data":{"quota":69281250,"rpm":3,"tpm":1200}}`) // 138.56 USD
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func validAcc(base string) domain.Account {
	return domain.Account{
		ID: "n", Provider: "newapi", Label: "x", BaseURL: base,
		AccessTokenEnv: "NEWAPI_AT", UserID: "16002",
	}
}

// TestFetchUsage_NativeGolden 全流程：余额 = 121992688/500000 = 243.99；
// Recent.Window7d = 25600000/500000 = 51.2，Window30d = 69281250/500000 = 138.56。
func TestFetchUsage_NativeGolden(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 200, 200)
	defer srv.Close()

	u, err := New().FetchUsage(context.Background(), validAcc(srv.URL))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(u.Dimensions) != 1 || u.Dimensions[0].Balance != 243.985376 {
		t.Errorf("Balance = %v, want 243.99 (121992688/500000)", u.Dimensions[0].Balance)
	}
	if u.Dimensions[0].Currency != "USD" || u.Dimensions[0].PercentUsed != -1 {
		t.Errorf("dim wrong: %+v", u.Dimensions[0])
	}
	if u.Recent == nil {
		t.Fatal("Recent must not be nil when stat succeeds")
	}
	if u.Recent.Window7d != 51.2 || u.Recent.Window30d != 138.5625 {
		t.Errorf("Recent windows wrong: %+v", u.Recent)
	}
	if u.Recent.RPM != 3 || u.Recent.TPM != 1200 || u.Recent.Currency != "USD" {
		t.Errorf("Recent rpm/tpm/currency wrong: %+v", u.Recent)
	}
}

// TestFetchUsage_QPUFallback status 失败 → 用 defaultQPU(500000) 换算。
func TestFetchUsage_QPUFallback(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 200, 500) // status 500
	defer srv.Close()

	u, err := New().FetchUsage(context.Background(), validAcc(srv.URL))
	if err != nil {
		t.Fatalf("status 500 should fall back to defaultQPU, not error: %v", err)
	}
	if u.Dimensions[0].Balance != 243.985376 { // 仍用 500000
		t.Errorf("Balance = %v, want 243.99 with defaultQPU", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_StatDegraded stat 端点 404 → Recent=nil，余额仍正确。
func TestFetchUsage_StatDegraded(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/self":
			fmt.Fprint(w, `{"data":{"quota":121992688,"used_quota":69281250}}`)
		case "/api/status":
			fmt.Fprint(w, `{"data":{"quota_per_unit":500000}}`)
		case "/api/log/self/stat":
			w.WriteHeader(http.StatusNotFound) // stat 失败
		}
	}))
	defer srv.Close()

	u, err := New().FetchUsage(context.Background(), validAcc(srv.URL))
	if err != nil {
		t.Fatalf("stat 404 must not error: %v", err)
	}
	if u.Recent != nil {
		t.Errorf("Recent must be nil when stat fails, got %+v", u.Recent)
	}
	if u.Dimensions[0].Balance != 243.985376 {
		t.Errorf("Balance still correct despite stat fail: %v", u.Dimensions[0].Balance)
	}
}

// TestFetchUsage_UserSelfFails user/self 401 → 报错。
func TestFetchUsage_UserSelfFails(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 401, 200)
	defer srv.Close()

	if _, err := New().FetchUsage(context.Background(), validAcc(srv.URL)); err == nil {
		t.Fatal("user/self failure should error")
	}
}

// TestFetchUsage_MissingCreds 缺 access_token_env 或 user_id → 报错。
func TestFetchUsage_MissingCreds(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	srv := nativeServer(t, 200, 200)
	defer srv.Close()

	for _, acc := range []domain.Account{
		{ID: "n", Provider: "newapi", BaseURL: srv.URL, UserID: "16002"},            // 缺 AccessTokenEnv
		{ID: "n", Provider: "newapi", BaseURL: srv.URL, AccessTokenEnv: "NEWAPI_AT"}, // 缺 UserID
	} {
		if _, err := New().FetchUsage(context.Background(), acc); err == nil {
			t.Fatalf("missing creds should error: %+v", acc)
		}
	}
}

// TestFetchUsage_TokenNotSet access_token_env 配了但环境变量为空 → 报错。
func TestFetchUsage_TokenNotSet(t *testing.T) {
	t.Setenv("NEWAPI_AT", "") // 显式置空
	srv := nativeServer(t, 200, 200)
	defer srv.Close()

	if _, err := New().FetchUsage(context.Background(), validAcc(srv.URL)); err == nil {
		t.Fatal("empty access token should error")
	}
}

// TestFetchUsage_BaseURLRequired 自部署无默认 base。
func TestFetchUsage_BaseURLRequired(t *testing.T) {
	t.Setenv("NEWAPI_AT", "AT")
	acc := validAcc("")
	if _, err := New().FetchUsage(context.Background(), acc); err == nil {
		t.Fatal("expected error for missing base_url")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/adapters/providers/newapi/ -v`
Expected: COMPILE FAIL（`getJSON` 签名不匹配 / billing 路径常量仍在 / `Recent` 未填充）

- [ ] **Step 3: 重写实现文件 `newapi.go`（全量替换，保留 license 头）**

替换 `package newapi` 之后的所有内容：

```go
// Package newapi 实现 ports.UsageProvider，对接 new-api（one-api fork）中转平台原生管理层。
//
// 真实接口契约（QuantumNous/new-api，实测 kuaipao.pro）：
//   - GET {BaseURL}/api/user/self                         → data.quota（剩余，内部单位）
//   - GET {BaseURL}/api/status                            → data.quota_per_unit（换算因子）
//   - GET {BaseURL}/api/log/self/stat?start&end           → data.quota（区间消耗）+ rpm/tpm
//   - 鉴权 Authorization: Bearer <access_token> + New-Api-User: <user_id>（双 header）
//
// 余额 = quota / quota_per_unit（美元）。quota_per_unit 失败回退 500000。
// stat 失败时 Recent=nil（消耗为次要信息，余额不受影响）。
// new-api 自部署，BaseURL 必填。
package newapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/maybewaityou/fleetboard/internal/core/domain"
	"github.com/maybewaityou/fleetboard/internal/core/ports"
)

const (
	userSelfPath = "/api/user/self"
	statusPath   = "/api/status"
	logStatPath  = "/api/log/self/stat"

	defaultQPU  = 500000            // quota_per_unit 回退默认
	window7d    = 7 * 24 * time.Hour
	window30d   = 30 * 24 * time.Hour
	httpTimeout = 10 * time.Second

	nameAvailable = "Available balance"
	sourceTag     = "newapi"
)

var _ ports.UsageProvider = (*Provider)(nil)

// Provider 是 new-api 余额适配器。零值不可用，请用 New 构造。
type Provider struct {
	hc *http.Client
}

// New 构造一个 new-api Provider，HTTP 客户端超时 10s。
func New() *Provider { return &Provider{hc: &http.Client{Timeout: httpTimeout}} }

// Provider 返回厂商标识。
func (p *Provider) Provider() string { return "newapi" }

type userSelfResp struct {
	Data struct {
		Quota     int64 `json:"quota"`
		UsedQuota int64 `json:"used_quota"`
	} `json:"data"`
}

type statusResp struct {
	Data struct {
		QuotaPerUnit float64 `json:"quota_per_unit"`
	} `json:"data"`
}

type statResp struct {
	Data struct {
		Quota int64 `json:"quota"`
		RPM   int   `json:"rpm"`
		TPM   int   `json:"tpm"`
	} `json:"data"`
}

// FetchUsage 拉取该账号真实余额与近窗口消耗。
// user/self 为核心（失败报错）；status 决定换算因子（失败回退）；stat 为辅助（失败 Recent=nil）。
func (p *Provider) FetchUsage(ctx context.Context, acc domain.Account) (domain.ProviderUsage, error) {
	u := domain.ProviderUsage{
		AccountID: acc.ID,
		Provider:  "newapi",
		Label:     acc.Label,
		FetchedAt: time.Now(),
		BaseURL:   acc.BaseURL,
		Endpoint:  userSelfPath,
	}
	if acc.BaseURL == "" {
		u.Err = fmt.Errorf("newapi: base_url is required (self-hosted, no default)")
		return u, u.Err
	}
	if acc.AccessTokenEnv == "" || acc.UserID == "" {
		u.Err = fmt.Errorf("newapi: access_token_env and user_id are required")
		return u, u.Err
	}
	accessToken := os.Getenv(acc.AccessTokenEnv)
	if accessToken == "" {
		u.Err = fmt.Errorf("newapi: access token not set in env %q", acc.AccessTokenEnv)
		return u, u.Err
	}

	// 1) user/self — 余额（核心，失败整体报错）。
	us := &userSelfResp{}
	if err := p.getJSON(ctx, acc.BaseURL+userSelfPath, accessToken, acc.UserID, us); err != nil {
		u.Err = fmt.Errorf("newapi: user/self: %w", err)
		return u, u.Err
	}

	// 2) status — 换算因子（失败回退 defaultQPU）。
	qpu := float64(defaultQPU)
	if st, err := p.getStatus(ctx, acc.BaseURL, accessToken, acc.UserID); err == nil && st > 0 {
		qpu = st
	}
	usd := func(q int64) float64 { return float64(q) / qpu }

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     usd(us.Data.Quota),
		Currency:    "USD",
		PercentUsed: -1,
		Source:      sourceTag,
	}}
	u.Primary = &u.Dimensions[0]

	// 3) stat — 近窗口消耗（辅助，失败 Recent=nil）。
	now := time.Now()
	statURL := func(window time.Duration) string {
		return fmt.Sprintf("%s%s?start_timestamp=%d&end_timestamp=%d",
			acc.BaseURL, logStatPath, now.Add(-window).Unix(), now.Unix())
	}
	s7, err7 := p.getStat(ctx, statURL(window7d), accessToken, acc.UserID)
	s30, err30 := p.getStat(ctx, statURL(window30d), accessToken, acc.UserID)
	if err7 == nil && err30 == nil {
		u.Recent = &domain.RecentUsage{
			Window7d:  usd(s7.Data.Quota),
			Window30d: usd(s30.Data.Quota),
			RPM:       s7.Data.RPM,
			TPM:       s7.Data.TPM,
			Currency:  "USD",
		}
	}
	return u, nil
}

// getStatus 取 quota_per_unit；失败返回 0（调用方回退默认）。
func (p *Provider) getStatus(ctx context.Context, base, bearer, newUser string) (float64, error) {
	st := &statusResp{}
	if err := p.getJSON(ctx, base+statusPath, bearer, newUser, st); err != nil {
		return 0, err
	}
	return st.Data.QuotaPerUnit, nil
}

// getStat 取区间消耗统计；失败返回 error（调用方据此决定 Recent 取舍）。
func (p *Provider) getStat(ctx context.Context, url, bearer, newUser string) (*statResp, error) {
	s := &statResp{}
	if err := p.getJSON(ctx, url, bearer, newUser, s); err != nil {
		return nil, err
	}
	return s, nil
}

// getJSON 发 GET（带 Bearer + New-Api-User 双 header）并解码进 out；非 2xx 或解码失败返回错误。
func (p *Provider) getJSON(ctx context.Context, url, bearer, newUser string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("New-Api-User", newUser)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/adapters/providers/newapi/ -v`
Expected: PASS（全部 7 个测试）

- [ ] **Step 5: 全量构建 + 测试**

Run: `go build ./... && go test ./...`
Expected: build ok，全包测试通过（含 ui 包——表单/详情页尚未改，但 domain 字段加了 omitempty 不破坏）

- [ ] **Step 6: 提交**

```bash
git add internal/adapters/providers/newapi/newapi.go internal/adapters/providers/newapi/newapi_test.go
git commit -m "feat(newapi): switch to native /api layer for real balance and usage"
```

---

## Task 3: UI 详情页 Recent 区块

**Files:**
- Modify: `internal/adapters/ui/account_details.go`（`Render` 方法约第 106-110 行 + 文件末尾加 `renderRecent`）
- Test: `internal/adapters/ui/account_details_test.go`

**Interfaces:**
- Consumes: `domain.RecentUsage`、`domain.ProviderUsage.Recent`（Task 1）；复用现有 `basicInfoLine`、`formatMoney`、`colorTitle`。
- Produces: `renderRecent(domain.RecentUsage) string`，在 `Render` 末尾按 `u.Recent != nil` 调用。

- [ ] **Step 1: 写失败测试**

追加到 `internal/adapters/ui/account_details_test.go`：

```go
// TestRenderRecent 验证 Recent 区块渲染键值行；nil 时不渲染。
func TestRenderRecent(t *testing.T) {
	// 有 Recent
	got := renderRecent(domain.RecentUsage{Window7d: 51.2, Window30d: 138.56, RPM: 3, TPM: 1200, Currency: "USD"})
	for _, want := range []string{"Usage (recent)", "7-day:", "$51.20", "30-day:", "$138.56", "Live:", "3 rpm / 1200 tpm"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderRecent missing %q, got: %q", want, got)
		}
	}
}

// TestRenderSkipsRecentWhenNil 验证 Render 在 Recent=nil 时不输出 Usage 区块，非空时输出。
// AccountDetails 内嵌 *tview.TextView，用 GetText(true) 读取渲染后的文本断言。
func TestRenderSkipsRecentWhenNil(t *testing.T) {
	d := NewAccountDetails()
	u := domain.ProviderUsage{
		Provider:   "newapi",
		Label:      "x",
		Dimensions: []domain.UsageDimension{{Name: "Available balance", Balance: 1, Currency: "USD", PercentUsed: -1}},
	}
	u.Primary = &u.Dimensions[0]

	// Recent=nil → 不输出 Usage 区块。
	d.Render(u)
	if strings.Contains(d.GetText(true), "Usage (recent)") {
		t.Error("Render should NOT output Usage block when Recent is nil")
	}

	// Recent 非空 → 输出 Usage 区块。
	u.Recent = &domain.RecentUsage{Window7d: 5, Currency: "USD"}
	d.Render(u)
	if !strings.Contains(d.GetText(true), "Usage (recent)") {
		t.Error("Render should output Usage block when Recent is set")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/adapters/ui/ -run TestRenderRecent -v`
Expected: COMPILE FAIL（`renderRecent` undefined）

- [ ] **Step 3: 在 `account_details.go` 文件末尾加 `renderRecent`**

```go
// renderRecent 渲染近窗口消耗摘要区块：
//
//	Usage (recent)
//	  7-day:        $51.20
//	  30-day:       $138.56
//	  Live:         3 rpm / 1200 tpm
//
// 复用 basicInfoLine（pad10 对齐）与 formatMoney（带货币符号），风格与 Basic Info 一致。
func renderRecent(r domain.RecentUsage) string {
	var b strings.Builder
	b.WriteString("\n[" + colorTitle + "::b]Usage (recent)[-]\n")
	b.WriteString(basicInfoLine("7-day", formatMoney(r.Window7d, r.Currency)))
	b.WriteString(basicInfoLine("30-day", formatMoney(r.Window30d, r.Currency)))
	b.WriteString(basicInfoLine("Live", fmt.Sprintf("%d rpm / %d tpm", r.RPM, r.TPM)))
	return b.String()
}
```

确认文件已 `import "fmt"`（现有 `basicInfoLine` 已用 fmt，已导入）。

- [ ] **Step 4: 在 `Render` 方法末尾接入**

在 `account_details.go::Render` 的 `for _, dim := range dims` 循环之后、`d.SetText(b.String())` 之前插入：

```go
	// Recent 消耗摘要（仅余额型 provider 填充；nil 时跳过，其他 provider 无感）。
	if u.Recent != nil {
		b.WriteString(renderRecent(*u.Recent))
	}
```

- [ ] **Step 5: 运行测试验证通过**

Run: `go test ./internal/adapters/ui/ -run "TestRenderRecent|TestRender" -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/adapters/ui/account_details.go internal/adapters/ui/account_details_test.go
git commit -m "feat(ui): render recent usage block in account details"
```

---

## Task 4: 账号表单加 new-api 凭证字段 + provider 感知校验

**Files:**
- Modify: `internal/adapters/ui/account_form.go`（字段常量约第 27-32、placeholder 约第 38-43、构造约第 61-76、Prefill 约 109-118、submit 约 122-136）
- Test: `internal/adapters/ui/account_form_test.go`

**Interfaces:**
- Consumes: `Account.AccessTokenEnv`、`Account.UserID`（Task 1）。
- Produces: 表单新增 `AccessTokenEnv` / `UserID` 输入；`submit` 按 provider 分支校验（newapi 要 AccessTokenEnv+UserID，其余要 TokenEnv）。

- [ ] **Step 1: 写失败测试**

追加到 `internal/adapters/ui/account_form_test.go`：

```go
// TestAccountFormSubmitNewapi newapi 必填 AccessTokenEnv + UserID（TokenEnv 可空）。
func TestAccountFormSubmitNewapi(t *testing.T) {
	f := NewAccountForm()
	f.input(afFieldLabel).SetText("kuaipao")
	f.providerDropDown().SetCurrentOption(5) // newapi（providerOptions 第 6 项，idx=5）
	f.input(afFieldAccessTokenEnv).SetText("NEWAPI_AT")
	f.input(afFieldUserID).SetText("16002")

	var got domain.Account
	called := false
	f.OnSubmit(func(acc domain.Account) { got = acc; called = true })
	f.submit()

	if !called {
		t.Fatal("submit should fire for valid newapi (AccessTokenEnv+UserID)")
	}
	if got.AccessTokenEnv != "NEWAPI_AT" || got.UserID != "16002" {
		t.Fatalf("newapi creds not captured: %+v", got)
	}
}

// TestAccountFormSubmitNewapiRejectsMissing newapi 缺凭证不触发 submit。
func TestAccountFormSubmitNewapiRejectsMissing(t *testing.T) {
	f := NewAccountForm()
	f.input(afFieldLabel).SetText("kuaipao")
	f.providerDropDown().SetCurrentOption(5) // newapi
	f.input(afFieldAccessTokenEnv).SetText("NEWAPI_AT")
	// UserID 留空
	called := false
	f.OnSubmit(func(domain.Account) { called = true })
	f.submit()
	if called {
		t.Fatal("submit must not fire when newapi UserID missing")
	}
}

// TestAccountFormPrefillNewapi 验证 Prefill 回填 new-api 凭证字段。
func TestAccountFormPrefillNewapi(t *testing.T) {
	f := NewAccountForm()
	f.Prefill(domain.Account{
		Provider: "newapi", Label: "l", BaseURL: "http://b",
		AccessTokenEnv: "NEWAPI_AT", UserID: "16002",
	})
	if f.text(afFieldAccessTokenEnv) != "NEWAPI_AT" {
		t.Errorf("AccessTokenEnv not prefilled: %q", f.text(afFieldAccessTokenEnv))
	}
	if f.text(afFieldUserID) != "16002" {
		t.Errorf("UserID not prefilled: %q", f.text(afFieldUserID))
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/adapters/ui/ -run TestAccountFormSubmitNewapi -v`
Expected: COMPILE FAIL（`afFieldAccessTokenEnv` undefined）

- [ ] **Step 3: 改字段常量与 placeholder**

在 `account_form.go` 的 `const ( afFieldLabel = iota ... )` 块追加两个字段：

```go
const (
	afFieldLabel = iota
	afFieldProvider
	afFieldBaseURL
	afFieldTokenEnv
	afFieldAccessTokenEnv // new-api 原生层 access_token 环境变量名
	afFieldUserID         // new-api 用户 ID
)
```

在 placeholder 常量块追加：

```go
const (
	phLabel          = "e.g. GLM main"
	phProvider       = "Select provider"
	phBaseURL        = "leave empty for default"
	phTokenEnv       = "e.g. GLM_API_KEY"
	phAccessTokenEnv = "new-api: e.g. NEWAPI_AT"
	phUserID         = "new-api user id, e.g. 16002"
)
```

- [ ] **Step 4: 在 `NewAccountForm` 构造里加两个输入字段**

在 `f.form.AddInputField("TokenEnv:", ...)` 之后追加两行：

```go
	f.form.AddInputField("AccessTokenEnv:", "", 0, nil, nil)
	f.form.AddInputField("UserID:", "", 0, nil, nil)
```

在 placeholder 设置段（`f.input(afFieldTokenEnv).SetPlaceholder(...)` 之后）追加：

```go
	f.input(afFieldAccessTokenEnv).SetPlaceholder(phAccessTokenEnv).SetPlaceholderTextColor(tcell.GetColor(colorSecondary))
	f.input(afFieldUserID).SetPlaceholder(phUserID).SetPlaceholderTextColor(tcell.GetColor(colorSecondary))
```

- [ ] **Step 5: 改 `Prefill` 回填新字段**

在 `Prefill` 方法内 `f.input(afFieldTokenEnv).SetText(acc.TokenEnv)` 之后追加：

```go
	f.input(afFieldAccessTokenEnv).SetText(acc.AccessTokenEnv)
	f.input(afFieldUserID).SetText(acc.UserID)
```

- [ ] **Step 6: 改 `submit` 为 provider 感知校验**

将 `submit` 方法整体替换为：

```go
// submit 校验并提交。必填项按 provider 分支：
//   - newapi：AccessTokenEnv + UserID（TokenEnv 不再使用）
//   - 其他：TokenEnv（沿用旧规则）
// ID 不在此设置（新增时由 cmd/main 用 domain.GenerateAccountID 生成）。
func (f *AccountForm) submit() {
	label := f.text(afFieldLabel)
	_, provider := f.providerDropDown().GetCurrentOption()
	if label == "" || provider == "" {
		return
	}
	acc := domain.Account{
		Provider:       provider,
		Label:          label,
		BaseURL:        f.text(afFieldBaseURL),
		TokenEnv:       f.text(afFieldTokenEnv),
		AccessTokenEnv: f.text(afFieldAccessTokenEnv),
		UserID:         f.text(afFieldUserID),
	}
	if provider == "newapi" {
		if acc.AccessTokenEnv == "" || acc.UserID == "" {
			return
		}
	} else {
		if acc.TokenEnv == "" {
			return
		}
	}
	if f.onSubmit != nil {
		f.onSubmit(acc)
	}
}
```

- [ ] **Step 7: 运行测试验证通过**

Run: `go test ./internal/adapters/ui/ -v`
Expected: PASS（含新测试 + 现有 `TestAccountFormSubmitValid`（glm+TokenEnv 仍触发）、`TestAccountFormSubmitRejectsMissingRequired`（空表单不触发））

- [ ] **Step 8: 提交**

```bash
git add internal/adapters/ui/account_form.go internal/adapters/ui/account_form_test.go
git commit -m "feat(ui): add new-api credential fields with provider-aware validation"
```

---

## Task 5: README 文档与迁移说明

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: 无代码依赖；仅文档化 Task 1-4 的配置契约。

- [ ] **Step 1: 定位现有 new-api 文档段**

Run: `grep -n "newapi\|new-api" README.md README.zh-CN.md`
找到现有 new-api 配置说明的位置（relay-platforms 那批加入）。

- [ ] **Step 2: 更新 `README.zh-CN.md` 的 new-api 配置段**

将原 `token_env`（sk-key）说明替换为原生层配置（access_token_env + user_id）。在 new-api 配置示例处使用：

```yaml
- id: n1
  provider: newapi
  base_url: https://your-newapi.example.com
  access_token_env: NEWAPI_AT   # 存 access_token 的环境变量
  user_id: "16002"              # new-api 用户 ID
```

并补充获取方式说明：

```markdown
**获取 access_token 与 user_id**：
- access_token：new-api 后台 → 个人设置 → 系统访问令牌 → 生成。
- user_id：浏览器 F12 → Network → 任一 `/api/` 请求的 `New-Api-User` 请求头，或 Local Storage 的 `user.id`。

> 注：new-api 的 OpenAI 兼容 billing 端点（`/v1/dashboard/billing/*`）返回的是占位假数据，
> fleetboard 改用原生 `/api/*` 层获取真实余额与近 7/30 天消耗。
```

- [ ] **Step 3: 同步更新 `README.md`（英文）**

对应英文段做同样替换（`access_token_env` / `user_id` / 获取说明的英文版）。

- [ ] **Step 3b: bump 版本到 v0.2.0（破坏性配置变更）**

修改 `makefile` 第 2 行：

```makefile
VERSION  ?= v0.2.0
```

- [ ] **Step 4: 全量质量检查**

Run: `make quality && make test`
Expected: gofumpt + go vet 通过；`go test -race -cover ./...` 全绿。

- [ ] **Step 5: 提交**

```bash
git add README.md README.zh-CN.md
git commit -m "docs(readme): document new-api native access_token + user_id config"
```

---

## 完成标准（Definition of Done）

- [ ] 5 个 task 全部提交，`make quality` + `make test` 全绿。
- [ ] new-api 账号用 `access_token_env` + `user_id` 配置后，详情页显示真实余额（≈后台一致）+ `Usage (recent)` 区块（7d/30d/rpm/tpm）。
- [ ] 其他 provider（glm/minimax/kimi/deepseek/sub2api）行为不变（回归：`Recent=nil` 不渲染新区块，表单 TokenEnv 校验不变）。
- [ ] 真机校准：用 kuaipao.pro 实测，余额对得上后台 ≈244 USD（Task 2 实现后人工跑一次确认）。
- [ ] 版本 bump v0.2.0（破坏性配置变更）。
