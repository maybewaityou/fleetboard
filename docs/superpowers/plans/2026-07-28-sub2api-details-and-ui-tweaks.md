# sub2api 详情扩展与三项 UI 优化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 列表金额改 2 位小数；删除确认显示账号名称并复刻 lazytmux 风格按钮；重写 sub2api 适配器对接 `/v1/usage` 真实双模式契约，详情页按模式动态展示配额/速率窗口/用量/套餐/状态全字段。

**Architecture:** 沿用六边形架构。需求③扩展 `domain`（`UsageDimension`/`RecentUsage`/`ProviderUsage` 各加字段，零值无害），sub2api 适配器按 `mode` 分支解析归一为余额维度 + 金额配额维度 + Recent，详情页复用现有 `renderDimension`/`renderRecent` 加分支动态渲染。需求①②为 UI 层局部改动。

**Tech Stack:** Go 1.24.6 · tview v0.42.0 · tcell v2.13.10 · Tokyo Night 调色板（`internal/adapters/ui/const.go`，逐字对齐 lazytmux）。

## Global Constraints

- 颜色常量一律取自 `const.go`（`colorAccent=#7aa2f7`、`colorRed=#f7768e`、`colorGreen=#9ece6a`、`colorYellow=#e0af68`、`colorSecondary=#565f89`、`colorPrimary=#c0caf5`、`colorTitle=#7dcfff`），禁止内联新 hex。
- `domain` 只允许**新增字段**，不得改名或改类型现有字段，确保 glm/minimax/kimi/deepseek/newapi 零行为变化。
- 语义提交：`type(scope): desc`（`feat`/`fix`/`improve`/`refactor`/`test`/`docs`）。当前在 `master`，首个提交前先 `git checkout -b feat/sub2api-details-ui-tweaks`。
- 测试命令：`go test ./...`；质量：`go vet ./...`、`golangci-lint run ./...`（若装了）。
- sub2api 鉴权不变（`Authorization: Bearer <key>`、`GET {BaseURL}/v1/usage`）。
- 参考 spec：`docs/superpowers/specs/2026-07-28-sub2api-details-and-ui-tweaks-design.md`。

---

### Task 1: 列表金额保留 2 位小数

**Files:**
- Modify: `internal/adapters/ui/account_list.go:228-240`（`formatMoneyShort`）
- Test: `internal/adapters/ui/account_list_test.go:348-368`（`TestFormatMoneyShort` 金样本）

**Interfaces:**
- Produces: `formatMoneyShort(balance float64, currency string) string`（签名不变，仅精度 1→2 位）。

- [ ] **Step 1: 更新金样本测试为 2 位小数**

把 `internal/adapters/ui/account_list_test.go` 的 `TestFormatMoneyShort` 用例 `want` 全部改为 2 位小数：

```go
func TestFormatMoneyShort(t *testing.T) {
	cases := []struct {
		balance  float64
		currency string
		want     string
	}{
		{49.58894, "CNY", "¥49.59"},
		{3.0, "USD", "$3.00"},
		{1200.0, "CNY", "¥1.20k"},
		{0, "CNY", "¥0.00"},
		// M2: 负值 — 负号在符号前，普通与 k 分支各一。
		{-50.0, "CNY", "-¥50.00"},
		{-1500.0, "USD", "-$1.50k"},
	}
	for _, tc := range cases {
		if got := formatMoneyShort(tc.balance, tc.currency); got != tc.want {
			t.Errorf("formatMoneyShort(%v,%q) = %q, want %q", tc.balance, tc.currency, got, tc.want)
		}
	}
}
```

同时把函数上方注释（`:346`）"1 位小数"改为"2 位小数"。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/adapters/ui/ -run TestFormatMoneyShort -v`
Expected: FAIL（现有实现仍输出 `¥49.6` 等 1 位）。

- [ ] **Step 3: 把 `formatMoneyShort` 的 `%.1f` 改为 `%.2f`**

`internal/adapters/ui/account_list.go:228-240`，四个格式化动词 `%.1f`→`%.2f`、`%.1fk`→`%.2fk`：

```go
// formatMoneyShort 余额短格式（列表用，2 位小数，>1000 缩写 k）。负值把负号置于符号之前
// （-¥50.00 而非 ¥-50.00），spec §3 容许负余额场景。
func formatMoneyShort(balance float64, currency string) string {
	sym := currencySymbol(currency)
	if balance < 0 {
		if math.Abs(balance) >= 1000 {
			return "-" + sym + fmt.Sprintf("%.2fk", -balance/1000)
		}
		return "-" + sym + fmt.Sprintf("%.2f", -balance)
	}
	if math.Abs(balance) >= 1000 {
		return fmt.Sprintf("%s%.2fk", sym, balance/1000)
	}
	return fmt.Sprintf("%s%.2f", sym, balance)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/adapters/ui/ -run TestFormatMoneyShort -v`
Expected: PASS。

- [ ] **Step 5: 全量回归 + 提交**

Run: `go test ./...`（首次提交前先建分支：`git checkout -b feat/sub2api-details-ui-tweaks`）
Expected: 全绿。

```bash
git add internal/adapters/ui/account_list.go internal/adapters/ui/account_list_test.go
git commit -m "improve(ui): list money short format to 2 decimal places"
```

---

### Task 2: 删除确认显示名称 + lazytmux 风格按钮

**Files:**
- Modify: `internal/adapters/ui/handlers.go:102-125`（`confirmDelete`，并新增辅助函数 `buildDeleteConfirmMessage`）
- Test: `internal/adapters/ui/account_details_test.go`（新增 `TestBuildDeleteConfirmMessage`；放此处因属 UI 文本构造）

**Interfaces:**
- Consumes: `t.accountList.GetSelected() (domain.ProviderUsage, bool)`（`account_list.go:97`）、`ProviderTag(provider) (bg,fg string)`（`theme.go:52`）、颜色常量（`const.go`）。
- Produces: `buildDeleteConfirmMessage(label, provider string) string`。

- [ ] **Step 1: 写失败测试 `TestBuildDeleteConfirmMessage`**

在 `internal/adapters/ui/account_details_test.go` 末尾追加：

```go
// TestBuildDeleteConfirmMessage 验证删除确认文案含账号名称与 provider、以及"不可撤销"提示。
func TestBuildDeleteConfirmMessage(t *testing.T) {
	got := buildDeleteConfirmMessage("GLM main", "glm")
	for _, want := range []string{"GLM main", "glm", "cannot be undone"} {
		if !strings.Contains(got, want) {
			t.Errorf("delete message missing %q, got: %q", want, got)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/adapters/ui/ -run TestBuildDeleteConfirmMessage -v`
Expected: FAIL（`buildDeleteConfirmMessage` 未定义）。

- [ ] **Step 3: 实现 `buildDeleteConfirmMessage`**

在 `internal/adapters/ui/handlers.go` 顶部 import 块加入 `"fmt"`（若未存在）。在 `confirmDelete` 上方新增：

```go
// buildDeleteConfirmMessage 构造删除确认文案：显示账号名称 + provider 标识，并附"不可撤销"提示。
// 纯文本（颜色集中在按钮上，与 lazytmux showKillConfirmModal 一致），便于 tview.Modal 居中渲染。
func buildDeleteConfirmMessage(label, provider string) string {
	if label == "" {
		label = "(unnamed)"
	}
	if provider == "" {
		provider = "?"
	}
	return fmt.Sprintf("Delete account 「%s」(%s)?\n\nThis action cannot be undone.", label, provider)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/adapters/ui/ -run TestBuildDeleteConfirmMessage -v`
Expected: PASS。

- [ ] **Step 5: 重写 `confirmDelete` 复刻 lazytmux 按钮/事件/快捷键**

替换 `internal/adapters/ui/handlers.go:102-125` 整个 `confirmDelete`：

```go
// confirmDelete 弹出确认对话框（lazytmux 风格）：显示账号名称 + provider；按钮 Cancel(蓝)/Delete(红)；
// 快捷键 d/D 确认（=触发键）、c/C 取消、ESC 取消；Cancel 默认聚焦（安全默认）。确认后调
// onDeleteAccount(selectedID) 并 Render 新数据集。
func (t *TUI) confirmDelete() {
	u, ok := t.accountList.GetSelected()
	if !ok || t.selectedID == "" {
		t.setStatusTemporary("[" + colorYellow + "]No account selected[-]")
		return
	}
	id := t.selectedID
	msg := buildDeleteConfirmMessage(u.Label, u.Provider)

	// doDelete 供 SetDoneFunc 与 SetInputCapture 两路共用（同 lazytmux killSession）。
	doDelete := func() {
		t.closeModal()
		if t.onDeleteAccount != nil {
			if usages := t.onDeleteAccount(id); usages != nil {
				// modal 回调在 tview 主循环执行——必须同步刷新，不能 QueueUpdateDraw，否则死锁。
				t.applyDataset(usages)
			}
		}
	}

	modal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{
			"[" + colorAccent + "]C[-]ancel", // 第一个 = 默认聚焦 = 安全默认
			"[" + colorRed + "]D[-]elete",    // 破坏性 = 红
		}).
		SetDoneFunc(func(buttonIndex int, _ string) {
			if buttonIndex == 1 { // 索引 1 = Delete；0 或 -1(ESC 透传) = Cancel
				doDelete()
				return
			}
			t.closeModal()
		})
	// 字母快捷键与按钮对应（lazytmux 模式：确认键=触发键 d）。
	modal.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Rune() {
		case 'd', 'D':
			doDelete()
			return nil
		case 'c', 'C':
			t.closeModal()
			return nil
		}
		if e.Key() == tcell.KeyESC {
			t.closeModal()
			return nil
		}
		return e
	})
	t.app.SetRoot(modal, true)
}
```

- [ ] **Step 6: 编译 + 全量回归**

Run: `go build ./... && go test ./...`
Expected: 全绿（`handlers.go` 已 import `tcell`/`tview`，仅需确认 `"fmt"` 已加）。

- [ ] **Step 7: 手动验证 + 提交**

手动（TUI 交互）：`make run`，选中一账号按 `d`，确认对话框显示 `「Label」(provider)`；`d` 删除、`c`/`ESC` 取消；Cancel 默认聚焦；按钮字母 C 蓝/D 红。

```bash
git add internal/adapters/ui/handlers.go internal/adapters/ui/account_details_test.go
git commit -m "improve(ui): delete confirm shows account name, lazytmux-style buttons & keys"
```

---

### Task 3: domain 扩展（sub2api 详情的共享基础）

**Files:**
- Modify: `internal/core/domain/provider_usage.go:55-78`（`RecentUsage`、`UsageDimension`、`ProviderUsage` 加字段）

**Interfaces:**
- Produces: `UsageDimension.MoneyLimit/MoneyUsed float64`；`RecentUsage.TodayCost/TotalCost float64, Today/TotalTokens/Requests int64, AvgDurationMs int64`；`ProviderUsage.APIKeyStatus string, ExpiresAt *time.Time, DaysUntilExpiry int`。

- [ ] **Step 1: 扩展 `RecentUsage`（加今日/累计统计字段）**

`internal/core/domain/provider_usage.go`，在 `RecentUsage` 结构体的 `TPM int` 之后、`Currency string` 之前插入：

```go
type RecentUsage struct {
	Window7d  float64 // 近7天消耗（美元）
	Window30d float64 // 近30天消耗（美元）
	RPM       int     // 实时每分钟请求数
	TPM       int     // 实时每分钟 token 数

	// 今日/累计统计（sub2api usage.today/total 填充；其他 provider 零值=无）。
	TodayCost     float64
	TotalCost     float64
	TodayTokens   int64
	TotalTokens   int64
	TodayRequests int64
	TotalRequests int64
	AvgDurationMs int64

	Currency string // "USD"
}
```

- [ ] **Step 2: 扩展 `UsageDimension`（加金额配额字段）**

在 `UsageDimension` 的 `Currency string` 之后追加：

```go
	// 金额型配额窗口（USD）：sub2api 的 rate_limits 与订阅日/周/月限额。非零时 renderDimension
	// 走金额配额分支（显示 $used/$limit + 进度条）；token 型 provider 不填，零值跳过。
	// 金额剩余复用 Balance 字段（Balance = MoneyLimit - MoneyUsed）。
	MoneyLimit float64
	MoneyUsed  float64
```

- [ ] **Step 3: 扩展 `ProviderUsage`（加 API Key 状态/过期）**

在 `ProviderUsage` 的 `Recent *RecentUsage` 之后追加：

```go
	// sub2api API Key 状态与过期（其他 provider 零值/nil=无）。
	APIKeyStatus    string
	ExpiresAt       *time.Time
	DaysUntilExpiry int
```

- [ ] **Step 4: 编译 + 全量回归（纯结构扩展，无行为变化）**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: 全绿（新字段零值不影响现有 provider 与测试）。

- [ ] **Step 5: 提交**

```bash
git add internal/core/domain/provider_usage.go
git commit -m "feat(domain): add money-quota, recent today/total, apikey status fields"
```

---

### Task 4: sub2api 适配器重写（对接 `/v1/usage` 双模式契约）

**Files:**
- Modify: `internal/adapters/providers/sub2api/sub2api.go`（替换 `apiResp`、重写 `FetchUsage`、新增 helper）
- Test: `internal/adapters/providers/sub2api/sub2api_test.go`（替换金样本、新增三模式测试）

**Interfaces:**
- Consumes: Task 3 的 `UsageDimension.MoneyLimit/MoneyUsed`、`RecentUsage.Today*/Total*`、`ProviderUsage.APIKeyStatus/ExpiresAt/DaysUntilExpiry`。
- Produces: `Provider.FetchUsage` 返回的 `ProviderUsage` 承载余额维度 + 金额配额维度 + Recent + 状态。

- [ ] **Step 1: 写三模式金样本测试（替换旧金样本）**

把 `internal/adapters/providers/sub2api/sub2api_test.go` 顶部的 `goldenUsage` 常量及注释（`:27-29`）替换为三个模式金样本：

```go
// 三模式金样本（字段取自 Wei-Shaw/sub2api gateway_handler.go 的真实响应结构）。
const goldenWallet = `{"mode":"unrestricted","isValid":true,"planName":"钱包余额","remaining":42.5,"unit":"USD","balance":42.5}`

const goldenQuota = `{
  "mode":"quota_limited","isValid":true,"status":"active","unit":"USD",
  "quota":{"limit":100,"used":37.5,"remaining":62.5,"unit":"USD"},
  "remaining":62.5,
  "rate_limits":[
    {"window":"5h","limit":20,"used":7,"remaining":13,"reset_at":"2026-07-28T05:00:00Z"},
    {"window":"1d","limit":50,"used":12,"remaining":38,"reset_at":"2026-07-28T00:00:00Z"}
  ],
  "expires_at":"2026-12-31T00:00:00Z","days_until_expiry":156,
  "usage":{"today":{"requests":10,"total_tokens":3050,"cost":1.5},"total":{"requests":100,"total_tokens":30000,"cost":15.0},"average_duration_ms":2500,"rpm":5,"tpm":1500}
}`

const goldenSubscription = `{
  "mode":"unrestricted","isValid":true,"planName":"Weekly plan","remaining":12.5,"unit":"USD",
  "subscription":{"daily_usage_usd":2.5,"weekly_usage_usd":7.5,"monthly_usage_usd":50.0,"daily_limit_usd":5.0,"weekly_limit_usd":20.0,"monthly_limit_usd":100.0,"weekly_window_start":"2026-07-13T00:30:00+08:00","expires_at":"2026-12-31T00:00:00Z"},
  "usage":{"today":{"requests":3,"total_tokens":800,"cost":0.4},"total":{"requests":30,"total_tokens":8000,"cost":4.0},"rpm":1,"tpm":400}
}`
```

把现有 `TestFetchUsageGolden`（`:39-82`）替换为三个模式测试：

```go
// 钱包模式：balance 归一为 Primary.Balance，仅 1 个维度，PlanLevel=钱包余额。
func TestFetchUsage_WalletMode(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		fmt.Fprint(w, goldenWallet)
	}))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "MyRelay", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotAuth != "Bearer KEY" || gotPath != "/v1/usage" {
		t.Errorf("auth/path wrong: %q %q", gotAuth, gotPath)
	}
	if len(u.Dimensions) != 1 {
		t.Fatalf("wallet: len(Dimensions) = %d, want 1", len(u.Dimensions))
	}
	if u.Dimensions[0].Balance != 42.5 || u.Dimensions[0].Currency != "USD" || u.Dimensions[0].PercentUsed != -1 {
		t.Errorf("wallet primary wrong: %+v", u.Dimensions[0])
	}
	if u.PlanLevel != "钱包余额" {
		t.Errorf("PlanLevel = %q, want 钱包余额", u.PlanLevel)
	}
}

// 配额模式：Balance=quota.remaining；维度=1 primary + 1 Total quota + 2 rate_limits；Recent 非空；状态/过期填充。
func TestFetchUsage_QuotaMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, goldenQuota) }))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "q", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != 62.5 {
		t.Errorf("quota primary Balance = %v, want 62.5", u.Dimensions[0].Balance)
	}
	// 1 primary + 1 Total quota + 2 rate_limits = 4
	if len(u.Dimensions) != 4 {
		t.Fatalf("quota: len(Dimensions) = %d, want 4", len(u.Dimensions))
	}
	// Total quota 维度（index 1）
	tq := u.Dimensions[1]
	if tq.Name != "Total quota" || tq.MoneyLimit != 100 || tq.MoneyUsed != 37.5 || tq.Balance != 62.5 {
		t.Errorf("Total quota dim wrong: %+v", tq)
	}
	// 第一个 rate_limit 窗口（index 2）带 ResetsAt
	rl := u.Dimensions[2]
	if rl.Name != "5h window" || rl.MoneyLimit != 20 || rl.ResetsAt.IsZero() {
		t.Errorf("5h window dim wrong: %+v", rl)
	}
	if u.Recent == nil || u.Recent.TodayCost != 1.5 || u.Recent.TotalTokens != 30000 || u.Recent.RPM != 5 {
		t.Errorf("Recent wrong: %+v", u.Recent)
	}
	if u.APIKeyStatus != "active" || u.DaysUntilExpiry != 156 || u.ExpiresAt == nil {
		t.Errorf("status/expiry wrong: %+v", u)
	}
}

// 订阅模式：Balance=remaining；维度=1 primary + 3 日/周/月；PlanLevel=Weekly plan。
func TestFetchUsage_SubscriptionMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, goldenSubscription) }))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "KEY")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "sub", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != 12.5 {
		t.Errorf("sub primary Balance = %v, want 12.5", u.Dimensions[0].Balance)
	}
	// 1 primary + 日/周/月 = 4
	if len(u.Dimensions) != 4 {
		t.Fatalf("sub: len(Dimensions) = %d, want 4", len(u.Dimensions))
	}
	if u.PlanLevel != "Weekly plan" {
		t.Errorf("PlanLevel = %q, want Weekly plan", u.PlanLevel)
	}
}
```

更新 `TestFetchUsage_NegativeBalance`（`:85-99`）为钱包模式负余额金样本：

```go
func TestFetchUsage_NegativeBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"mode":"unrestricted","planName":"钱包余额","remaining":-3.25,"unit":"USD","balance":-3.25}`)
	}))
	defer srv.Close()
	t.Setenv("SUB2API_KEY", "K")
	acc := domain.Account{ID: "s", Provider: "sub2api", Label: "neg", TokenEnv: "SUB2API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if u.Dimensions[0].Balance != -3.25 {
		t.Errorf("Balance = %v, want -3.25", u.Dimensions[0].Balance)
	}
}
```

保留 `TestFetchUsage_BaseURLRequired`、`TestFetchUsage_Non200`、`TestProviderReturnsSlug` 不变。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/adapters/providers/sub2api/ -v`
Expected: FAIL（旧 `apiResp` 解析不出新金样本字段，断言失败 / 维度数不符）。

- [ ] **Step 3: 替换 `apiResp` 为完整结构**

`internal/adapters/providers/sub2api/sub2api.go`，把 `apiResp`（`:58-62`）替换为：

```go
// apiResp 是 /v1/usage 响应结构（Wei-Shaw/sub2api gateway_handler.go 双模式契约）。
type apiResp struct {
	Mode            string            `json:"mode"` // quota_limited | unrestricted
	IsValid         bool              `json:"isValid"`
	Status          string            `json:"status"`
	PlanName        string            `json:"planName"`
	Remaining       float64           `json:"remaining"`
	Unit            string            `json:"unit"`
	Balance         float64           `json:"balance"` // 钱包模式
	Quota           *quotaResp        `json:"quota"`   // 配额模式
	RateLimits      []rateLimitResp   `json:"rate_limits"`
	Subscription    *subscriptionResp `json:"subscription"` // 订阅模式
	Usage           *usageResp        `json:"usage"`
	ExpiresAt       *time.Time        `json:"expires_at"`
	DaysUntilExpiry *int              `json:"days_until_expiry"`
}

type quotaResp struct {
	Limit     float64 `json:"limit"`
	Used      float64 `json:"used"`
	Remaining float64 `json:"remaining"`
	Unit      string  `json:"unit"`
}

type rateLimitResp struct {
	Window    string     `json:"window"`
	Limit     float64    `json:"limit"`
	Used      float64    `json:"used"`
	Remaining float64    `json:"remaining"`
	ResetAt   *time.Time `json:"reset_at"`
}

type subscriptionResp struct {
	DailyUsageUSD     float64    `json:"daily_usage_usd"`
	WeeklyUsageUSD    float64    `json:"weekly_usage_usd"`
	MonthlyUsageUSD   float64    `json:"monthly_usage_usd"`
	DailyLimitUSD     *float64   `json:"daily_limit_usd"`
	WeeklyLimitUSD    *float64   `json:"weekly_limit_usd"`
	MonthlyLimitUSD   *float64   `json:"monthly_limit_usd"`
	WeeklyWindowStart *time.Time `json:"weekly_window_start"`
	ExpiresAt         *time.Time `json:"expires_at"`
}

type usageResp struct {
	Today             usageBucket `json:"today"`
	Total             usageBucket `json:"total"`
	AverageDurationMs int64       `json:"average_duration_ms"`
	Rpm               int         `json:"rpm"`
	Tpm               int         `json:"tpm"`
}

type usageBucket struct {
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`
}
```

> 说明：`usageBucket` 只保留 fleetboard 用到的字段（requests/total_tokens/cost）；input/output/cache token 等未展示字段不解码（YAGNI），后续要显示再补 json tag。

- [ ] **Step 4: 重写 `FetchUsage` 的解析段（解码之后，`return` 之前）**

把 `sub2api.go:108-117`（从 `u.Dimensions = ...` 到 `return u, nil` 以及 FetchUsage 的闭合 `}`）替换为下面的逻辑——它包含 FetchUsage 的结尾 `return u, nil` + 闭合 `}`，并在函数之后新增独立的 `moneyQuotaDim` 函数：

```go
	const usd = "USD"
	// 1) Primary 余额维度（Dimensions[0]）：归一三种模式的"剩余"。
	primary := domain.UsageDimension{
		Name:        nameAvailable,
		Currency:    usd,
		PercentUsed: -1,
		Source:      sourceTag,
	}
	switch {
	case r.Mode == "quota_limited" && r.Quota != nil:
		primary.Balance = r.Quota.Remaining
	case r.Mode == "unrestricted" && r.Subscription != nil:
		primary.Balance = r.Remaining
	default: // 钱包余额模式（或未知模式兜底取 balance）
		primary.Balance = r.Balance
	}
	dims := []domain.UsageDimension{primary}

	// 2) 金额配额维度：配额模式追加 Total quota + 各 rate_limit 窗口。
	if r.Mode == "quota_limited" && r.Quota != nil && r.Quota.Limit > 0 {
		dims = append(dims, moneyQuotaDim("Total quota", r.Quota.Limit, r.Quota.Used, r.Quota.Remaining, usd, time.Time{}))
		for _, rl := range r.RateLimits {
			var reset time.Time
			if rl.ResetAt != nil {
				reset = *rl.ResetAt
			}
			dims = append(dims, moneyQuotaDim(rl.Window+" window", rl.Limit, rl.Used, rl.Remaining, usd, reset))
		}
	}
	// 订阅模式追加日/周/月限额维度。
	if r.Mode == "unrestricted" && r.Subscription != nil {
		s := r.Subscription
		if s.DailyLimitUSD != nil && *s.DailyLimitUSD > 0 {
			dims = append(dims, moneyQuotaDim("Daily limit", *s.DailyLimitUSD, s.DailyUsageUSD, *s.DailyLimitUSD-s.DailyUsageUSD, usd, time.Time{}))
		}
		if s.WeeklyLimitUSD != nil && *s.WeeklyLimitUSD > 0 {
			var reset time.Time
			if s.WeeklyWindowStart != nil {
				reset = *s.WeeklyWindowStart
			}
			dims = append(dims, moneyQuotaDim("Weekly limit", *s.WeeklyLimitUSD, s.WeeklyUsageUSD, *s.WeeklyLimitUSD-s.WeeklyUsageUSD, usd, reset))
		}
		if s.MonthlyLimitUSD != nil && *s.MonthlyLimitUSD > 0 {
			dims = append(dims, moneyQuotaDim("Monthly limit", *s.MonthlyLimitUSD, s.MonthlyUsageUSD, *s.MonthlyLimitUSD-s.MonthlyUsageUSD, usd, time.Time{}))
		}
	}

	u.Provider = "sub2api"
	u.Dimensions = dims
	u.Primary = &u.Dimensions[0]
	u.PlanLevel = r.PlanName
	u.APIKeyStatus = r.Status
	if r.ExpiresAt != nil {
		u.ExpiresAt = r.ExpiresAt
	}
	if r.DaysUntilExpiry != nil {
		u.DaysUntilExpiry = *r.DaysUntilExpiry
	}
	if r.Usage != nil {
		u.Recent = &domain.RecentUsage{
			TodayCost:     r.Usage.Today.Cost,
			TotalCost:     r.Usage.Total.Cost,
			TodayTokens:   r.Usage.Today.TotalTokens,
			TotalTokens:   r.Usage.Total.TotalTokens,
			TodayRequests: r.Usage.Today.Requests,
			TotalRequests: r.Usage.Total.Requests,
			RPM:           r.Usage.Rpm,
			TPM:           r.Usage.Tpm,
			AvgDurationMs: r.Usage.AverageDurationMs,
			Currency:      usd,
		}
	}
	return u, nil
}

// moneyQuotaDim 构造一个金额配额维度（sub2api rate_limits / 订阅周期）。reset 零值=无重置时间。
func moneyQuotaDim(name string, limit, used, remaining float64, currency string, reset time.Time) domain.UsageDimension {
	pct := -1.0
	if limit > 0 {
		pct = used / limit * 100
	}
	return domain.UsageDimension{
		Name:        name,
		MoneyLimit:  limit,
		MoneyUsed:   used,
		Balance:     remaining,
		Currency:    currency,
		PercentUsed: pct,
		ResetsAt:    reset,
		Source:      sourceTag,
	}
}
```

注意：`FetchUsage` 开头已构造 `u := domain.ProviderUsage{AccountID, Provider, Label, FetchedAt, ...}`（`:67-72`），上面片段中的 `u.Provider = "sub2api"` 是冗余保险（开头已设），可保留或删；`u.BaseURL`/`u.Endpoint` 在 `:79-80` 已设，不要重复删除。

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/adapters/providers/sub2api/ -v`
Expected: 三个模式测试 + NegativeBalance + BaseURLRequired + Non200 全 PASS。

- [ ] **Step 6: 全量回归 + 提交**

Run: `go test ./...`
Expected: 全绿。

```bash
git add internal/adapters/providers/sub2api/sub2api.go internal/adapters/providers/sub2api/sub2api_test.go
git commit -m "feat(sub2api): rewrite adapter for /v1/usage dual-mode contract (quota/subscription/wallet)"
```

---

### Task 5: 详情页渲染扩展（金额配额分支 + Recent 分组 + 状态/过期）

**Files:**
- Modify: `internal/adapters/ui/account_details.go`（`renderDimension` 加金额分支、`renderRecent` 分组按非零、`Render` Basic Info 加状态/过期）
- Test: `internal/adapters/ui/account_details_test.go`（新增金额维度、Recent today/total、状态行测试）

**Interfaces:**
- Consumes: Task 3 的 `UsageDimension.MoneyLimit/MoneyUsed`、`RecentUsage.Today*/Total*/AvgDurationMs`、`ProviderUsage.APIKeyStatus/ExpiresAt/DaysUntilExpiry`。

- [ ] **Step 1: 写失败测试**

在 `internal/adapters/ui/account_details_test.go` 末尾追加：

```go
// TestRenderDimensionMoneyQuota 验证金额配额维度：含 $used/$limit 与进度条，且不走纯 Balance 余额分支。
func TestRenderDimensionMoneyQuota(t *testing.T) {
	dim := domain.UsageDimension{Name: "5h window", MoneyLimit: 20, MoneyUsed: 7, Balance: 13, Currency: "USD", PercentUsed: 35, ResetsAt: time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)}
	got := renderDimension(dim)
	for _, want := range []string{"5h window", "$7.00 / $20.00", "35%", "Resets:"} {
		if !strings.Contains(got, want) {
			t.Errorf("money-quota dim missing %q, got: %q", want, got)
		}
	}
}

// TestRenderRecentTodayTotal 验证 sub2api 今日/累计统计行（Window7d/30d 为零时不显示）。
func TestRenderRecentTodayTotal(t *testing.T) {
	got := renderRecent(domain.RecentUsage{TodayCost: 1.5, TotalCost: 15.0, TodayTokens: 3050, TotalTokens: 30000, RPM: 5, TPM: 1500, AvgDurationMs: 2500, Currency: "USD"})
	for _, want := range []string{"Today:", "$1.50", "Total:", "$15.00", "Live:", "5 rpm / 1500 tpm", "Avg:", "2500ms"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderRecent(today/total) missing %q, got: %q", want, got)
		}
	}
	if strings.Contains(got, "7-day:") || strings.Contains(got, "30-day:") {
		t.Errorf("zero Window7d/30d should not render, got: %q", got)
	}
}
```

测试文件需 import `"time"`（在 import 块加）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/adapters/ui/ -run 'TestRenderDimensionMoneyQuota|TestRenderRecentTodayTotal' -v`
Expected: FAIL（金额分支未实现；Today/Total 未渲染）。

- [ ] **Step 3: `renderDimension` 加金额配额分支**

`internal/adapters/ui/account_details.go:156`，在 `renderDimension` 函数体中、现有余额分支 `if dim.Currency != "" {`（`:168`）**之前**插入金额配额分支：

```go
	// 金额配额型（sub2api rate_limits / 订阅周期）：显示 $used/$limit + 进度条 + 重置。
	if dim.MoneyLimit > 0 {
		pct := dim.PercentUsed
		bar := renderBar(pct, barWidth)
		pctStr := "N/A"
		if pct >= 0 {
			pctStr = fmt.Sprintf("%d%%", int(pct))
		}
		fmt.Fprintf(&b, "    %s  [%s]%s[-]\n", bar, colorPrimary, pctStr)
		fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s / %s[-]\n",
			colorSecondary, "Used:", colorPrimary,
			formatMoney(dim.MoneyUsed, dim.Currency), formatMoney(dim.MoneyLimit, dim.Currency))
		if !dim.ResetsAt.IsZero() {
			fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
				colorSecondary, "Resets:", colorPrimary, dim.ResetsAt.Local().Format("2006-01-02 15:04"))
		}
		b.WriteString("\n")
		return b.String()
	}
```

- [ ] **Step 4: `renderRecent` 改为分组按非零渲染**

替换 `internal/adapters/ui/account_details.go:325-332` 整个 `renderRecent`：

```go
// renderRecent 渲染近窗口消耗摘要区块，按字段分组、非零才显示对应行（避免 sub2api 的 7d/30d
// 零值显示 $0.00；newapi 填 7d/30d 时仍显示）。Live（RPM/TPM）始终显示。
func renderRecent(r domain.RecentUsage) string {
	var b strings.Builder
	b.WriteString("\n[" + colorTitle + "::b]Usage (recent)[-]\n")
	// 窗口组（newapi 等余额型 relay）。
	if r.Window7d != 0 {
		b.WriteString(basicInfoLine("7-day", formatMoney(r.Window7d, r.Currency)))
	}
	if r.Window30d != 0 {
		b.WriteString(basicInfoLine("30-day", formatMoney(r.Window30d, r.Currency)))
	}
	// 今日/累计组（sub2api usage.today/total）。
	if r.TodayCost != 0 || r.TodayTokens != 0 {
		b.WriteString(basicInfoLine("Today", fmt.Sprintf("%s · %s tok · %d req",
			formatMoney(r.TodayCost, r.Currency), compactInt(r.TodayTokens, ""), r.TodayRequests)))
	}
	if r.TotalCost != 0 || r.TotalTokens != 0 {
		b.WriteString(basicInfoLine("Total", fmt.Sprintf("%s · %s tok",
			formatMoney(r.TotalCost, r.Currency), compactInt(r.TotalTokens, ""))))
	}
	// 实时速率（始终）。
	b.WriteString(basicInfoLine("Live", fmt.Sprintf("%d rpm / %d tpm", r.RPM, r.TPM)))
	// 平均耗时。
	if r.AvgDurationMs != 0 {
		b.WriteString(basicInfoLine("Avg", fmt.Sprintf("%dms", r.AvgDurationMs)))
	}
	return b.String()
}
```

- [ ] **Step 5: `Render` 的 Basic Info 加 API Key 状态与过期行**

`internal/adapters/ui/account_details.go:81-86`，在 `providerInfoLine(u.Provider)` 之后插入 API Key 状态行，在 `basicInfoLine("Refreshed", refreshed)` 之后插入 Expires 行：

```go
	b.WriteString(basicInfoLine("Plan", plan))
	b.WriteString(providerInfoLine(u.Provider))
	if u.APIKeyStatus != "" {
		b.WriteString(basicInfoLine("API Key", u.APIKeyStatus))
	}
	b.WriteString(basicInfoLine("BaseURL", firstNonEmpty(u.BaseURL, "—")))
	b.WriteString(basicInfoLine("Endpoint", firstNonEmpty(u.Endpoint, "—")))
	b.WriteString(basicInfoLine("Refreshed", refreshed))
	if u.ExpiresAt != nil {
		exp := u.ExpiresAt.Local().Format("2006-01-02")
		if u.DaysUntilExpiry != 0 {
			exp += fmt.Sprintf(" (%dd left)", u.DaysUntilExpiry)
		}
		b.WriteString(basicInfoLine("Expires", exp))
	}
	b.WriteString(basicInfoLine("Pinned", pinnedStr(u.Pinned)))
```

- [ ] **Step 6: 跑测试确认通过**

Run: `go test ./internal/adapters/ui/ -v`
Expected: 全绿（含新测试，且现有 `TestRenderRecent`/`TestRenderDimensionBalance` 仍过——Window7d=51.2 非零仍显、余额维度 MoneyLimit=0 走原分支）。

- [ ] **Step 7: 全量回归 + lint + 提交**

Run: `go test ./... && go vet ./...`
Expected: 全绿。

```bash
git add internal/adapters/ui/account_details.go internal/adapters/ui/account_details_test.go
git commit -m "feat(ui): sub2api details — money-quota dims, recent today/total, status/expiry"
```

---

## 完成验证（全部 task 后）

- [ ] `go test ./...` 全绿。
- [ ] `make run` 接入三种 sub2api 账号（配额/订阅/钱包各一），确认详情页按模式动态显示、缺失字段隐藏、余额不再错显 0。
- [ ] 列表金额 2 位小数、列对齐无错位。
- [ ] 删除确认：名称+provider 显示、d 删除 / c·ESC 取消、Cancel 默认聚焦、按钮 C 蓝 D 红。
- [ ] （可选）合并分支到 master / 推送 PR。
