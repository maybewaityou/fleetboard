# DeepSeek / Kimi 余额细分与账号状态 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 DeepSeek 与 Kimi 响应里被丢弃的余额细分（granted/topped-up、voucher/cash）与 DeepSeek 账号状态（is_available）接入详情页。

**Architecture:** 领域模型加字段 → 两个余额型 adapter 补接线（struct 字段早已声明，仅 FetchUsage 未读取）→ 详情页 UI 渲染细分行与状态行。TDD，每任务先红后绿。

**Tech Stack:** Go（见 `go.mod`）、标准库 `net/http`+`encoding/json`+`strconv`、`tview` TUI、表驱动 + httptest golden 测试、`gofumpt`/`go vet`/`golangci-lint`。

## Global Constraints

- 字段命名严格用 `Granted`/`ToppedUp`（**非** `GrantedBalance`/`ToppedUpBalance`）；状态字段 `Status`（**非** 复用 `APIKeyStatus`）。
- `Status` 取值常量：`is_available=true`→`"active"`，`false`→`"insufficient"`（英文，与 `APIKeyStatus` 风格一致）。
- 细分容错：DeepSeek 的 `granted_balance`/`topped_up_balance` 用 `strconv.ParseFloat`，失败时返回 0 值、用 `_` 忽略 error——主余额 total 已成功，细分缺失不得致命。
- `u.Status` 必须在 DeepSeek 解码成功后、金额解析之前填充，确保错误路径（如 `balance_infos` 空数组）也携带状态。
- Kimi 的 `voucher_balance`→`Granted`、`cash_balance`→`ToppedUp`；Kimi 无 is_available，`Status` 留空。
- UI 细分行仅详情页 `renderDimension` 余额分支渲染（非零才显示），列表 mini 视图不改。
- 每个 TDD 任务结束前 `go build ./...` 通过、本包 `go test` 绿，方可 commit。
- commit message 用 conventional commits（`feat(domain): ...` / `feat(deepseek): ...` 等）。
- 当前分支：`feat/balance-breakdown`（spec 已提交于其上）。

---

## File Structure

| 文件 | 职责 | 本计划改动 |
|------|------|-----------|
| `internal/core/domain/provider_usage.go` | 领域模型 | 加 `UsageDimension.Granted/ToppedUp` + `ProviderUsage.Status` |
| `internal/adapters/providers/deepseek/deepseek.go` | DeepSeek adapter | `FetchUsage` 补细分解析 + Status 填充 |
| `internal/adapters/providers/deepseek/deepseek_test.go` | DeepSeek 测试 | golden 补断言 + 容错/欠费新用例 |
| `internal/adapters/providers/kimi/kimi.go` | Kimi adapter | `FetchUsage` Dimensions 填 voucher/cash |
| `internal/adapters/providers/kimi/kimi_test.go` | Kimi 测试 | golden 补细分断言 |
| `internal/adapters/ui/account_details.go` | 详情页 UI | `renderDimension` 余额分支加细分行 + `Render` 加 Status 行 |
| `internal/adapters/ui/account_details_test.go` | UI 测试 | 细分渲染 + Status 行用例 |

---

### Task 1: 领域模型扩展

**Files:**
- Modify: `internal/core/domain/provider_usage.go`（`UsageDimension` 约第 91-92 行 `Balance`/`Currency` 之后；`ProviderUsage` 约第 53-55 行 `APIKeyStatus` 区域）

**Interfaces:**
- Produces: `UsageDimension.Granted float64`、`UsageDimension.ToppedUp float64`、`ProviderUsage.Status string` —— 后续 adapter/UI 任务依赖这些字段名。

- [ ] **Step 1: 加 `UsageDimension` 细分字段**

在 `provider_usage.go` 的 `UsageDimension` 结构体中，`Currency string` 字段之后（`MoneyLimit` 之前）插入：

```go
	// 余额细分（余额型 provider 可选）：Granted=赠送/赠券部分，ToppedUp=充值/现金部分。
	// DeepSeek 填 granted_balance/topped_up_balance；Kimi 填 voucher_balance/cash_balance。
	// 配额型与其他余额型 provider 零值=无，UI 不渲染。语义约定 Granted+ToppedUp==Balance。
	Granted  float64
	ToppedUp float64
```

- [ ] **Step 2: 加 `ProviderUsage.Status` 字段**

在 `provider_usage.go` 的 `ProviderUsage` 结构体中，`DaysUntilExpiry int` 字段之后插入：

```go
	// 账号可用状态（adapter 填充，UI 读取）。DeepSeek 由 is_available 映射：
	// true→"active"，false→"insufficient"。其他 provider 零值=无，UI 不渲染。
	// 与 APIKeyStatus（sub2api 的 key active/expired）语义不同，故独立成字段。
	Status string
```

- [ ] **Step 3: 编译 + vet 验证（纯字段添加，无新行为，无独立测试）**

Run: `go build ./... && go vet ./...`
Expected: 无报错（现有测试不受影响——零值字段向后兼容）。

- [ ] **Step 4: 跑全量测试确认未破坏**

Run: `go test ./...`
Expected: 全绿。

- [ ] **Step 5: Commit**

```bash
git add internal/core/domain/provider_usage.go
git commit -m "feat(domain): add Granted/ToppedUp breakdown and Status fields"
```

---

### Task 2: DeepSeek 余额细分（granted/topped-up，含容错）

**Files:**
- Modify: `internal/adapters/providers/deepseek/deepseek.go`（`FetchUsage`，约第 131-143 行 `total` 解析与 Dimensions 构建）
- Test: `internal/adapters/providers/deepseek/deepseek_test.go`

**Interfaces:**
- Consumes: `UsageDimension.Granted`/`ToppedUp`（Task 1）
- Produces: DeepSeek `Dimensions[0]` 携带细分值。

- [ ] **Step 1: 写失败测试 —— golden 补细分断言**

在 `deepseek_test.go` 的 `TestFetchUsageGolden` 内，紧跟现有 `d.Source` 断言（约第 91 行）之后追加：

```go
	if d.Granted != 10.0 {
		t.Errorf("dim.Granted = %v, want 10.0 (granted_balance)", d.Granted)
	}
	if d.ToppedUp != 100.0 {
		t.Errorf("dim.ToppedUp = %v, want 100.0 (topped_up_balance)", d.ToppedUp)
	}
```

- [ ] **Step 2: 写失败测试 —— 新增容错用例**

在 `deepseek_test.go` 末尾追加：

```go
// TestFetchUsageBadGrantedBalance 验证 granted_balance 解析失败不致命：
// 主余额 total 照常成功，Granted 留零值，ToppedUp 正常解析。
func TestFetchUsageBadGrantedBalance(t *testing.T) {
	payload := `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"5.00","granted_balance":"oops","topped_up_balance":"5.00"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err (bad granted must NOT fail): %v", err)
	}
	d := u.Dimensions[0]
	if d.Balance != 5.0 {
		t.Errorf("dim.Balance = %v, want 5.0 (total preserved)", d.Balance)
	}
	if d.Granted != 0 {
		t.Errorf("dim.Granted = %v, want 0 (parse failure → zero)", d.Granted)
	}
	if d.ToppedUp != 5.0 {
		t.Errorf("dim.ToppedUp = %v, want 5.0 (valid parse)", d.ToppedUp)
	}
}
```

- [ ] **Step 3: 跑测试验证失败**

Run: `go test ./internal/adapters/providers/deepseek/`
Expected: FAIL —— `dim.Granted = 0, want 10.0`（字段尚未填充）。

- [ ] **Step 4: 实现 —— 细分容错解析并填入 Dimensions**

在 `deepseek.go` 的 `FetchUsage` 中，找到 `total` 解析成功后的 Dimensions 构建（约第 137 行）。在 `total, err := strconv.ParseFloat(...)` 的错误判断之后、`u.Dimensions = ...` 之前插入两行解析，并在 Dimensions 字面量加两字段：

```go
	total, err := strconv.ParseFloat(info.TotalBalance, 64)
	if err != nil {
		u.Err = fmt.Errorf("deepseek: parse total_balance %q: %w", info.TotalBalance, err)
		return u, u.Err
	}
	// 细分容错解析：ParseFloat 失败返回 0 值，用 _ 忽略 err——
	// 主余额 total 已成功，细分缺失（=0）不致命，UI 自动跳过零值行。
	granted, _ := strconv.ParseFloat(info.GrantedBalance, 64)
	topped, _ := strconv.ParseFloat(info.ToppedUpBalance, 64)

	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     total,
		Currency:    info.Currency,
		PercentUsed: -1,
		Source:      sourceTag,
		Granted:     granted,
		ToppedUp:    topped,
	}}
	u.Primary = &u.Dimensions[0]
	return u, nil
```

- [ ] **Step 5: 跑测试验证通过**

Run: `go test ./internal/adapters/providers/deepseek/`
Expected: PASS（golden 细分断言 + 容错用例均绿）。

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/providers/deepseek/deepseek.go internal/adapters/providers/deepseek/deepseek_test.go
git commit -m "feat(deepseek): surface granted/topped-up balance breakdown"
```

---

### Task 3: DeepSeek 账号状态（is_available → Status）

**Files:**
- Modify: `internal/adapters/providers/deepseek/deepseek.go`（`FetchUsage`，约第 115-123 行 decode 与空数组守卫之间）
- Test: `internal/adapters/providers/deepseek/deepseek_test.go`

**Interfaces:**
- Consumes: `ProviderUsage.Status`（Task 1）、`apiResp.IsAvailable`（struct 已有字段）
- Produces: DeepSeek `u.Status` = `"active"`/`"insufficient"`。

- [ ] **Step 1: 写失败测试 —— golden 补 Status 断言**

在 `TestFetchUsageGolden` 内，账号字段断言之前（约第 97 行 `// (d) Primary` 块之后）追加：

```go
	if u.Status != "active" {
		t.Errorf("Status = %q, want active (is_available=true in golden)", u.Status)
	}
```

- [ ] **Step 2: 写失败测试 —— 新增欠费用例**

在 `deepseek_test.go` 末尾追加：

```go
// TestFetchUsageUnavailable 验证 is_available=false 映射为 Status="insufficient"，
// 余额照常返回（欠费但仍有余额数据）。
func TestFetchUsageUnavailable(t *testing.T) {
	payload := `{"is_available":false,"balance_infos":[{"currency":"CNY","total_balance":"0.50","granted_balance":"0.50","topped_up_balance":"0"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	t.Setenv("DEEPSEEK_API_KEY", "K")
	acc := domain.Account{ID: "d", Provider: "deepseek", Label: "DeepSeek", TokenEnv: "DEEPSEEK_API_KEY", BaseURL: srv.URL}
	u, err := New().FetchUsage(context.Background(), acc)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if u.Status != "insufficient" {
		t.Errorf("Status = %q, want insufficient (is_available=false)", u.Status)
	}
	if u.Dimensions[0].Balance != 0.5 {
		t.Errorf("Balance = %v, want 0.5 (balance still returned when unavailable)", u.Dimensions[0].Balance)
	}
}
```

- [ ] **Step 3: 跑测试验证失败**

Run: `go test ./internal/adapters/providers/deepseek/`
Expected: FAIL —— `Status = "", want active`。

- [ ] **Step 4: 实现 —— 解码后立即填 Status（先于金额解析）**

在 `deepseek.go` 的 `FetchUsage` 中，找到 decode 成功块与空数组守卫：

```go
	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("deepseek: decode response: %w", err)
		return u, u.Err
	}
	if len(r.BalanceInfos) == 0 {
		u.Err = fmt.Errorf("deepseek: empty balance_infos")
		return u, u.Err
	}
```

在 decode 块之后、空数组守卫之前插入 Status 填充：

```go
	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		u.Err = fmt.Errorf("deepseek: decode response: %w", err)
		return u, u.Err
	}
	// 账号可用状态：先于金额解析填充，确保错误路径（如空 balance_infos）也携带状态。
	if r.IsAvailable {
		u.Status = "active"
	} else {
		u.Status = "insufficient"
	}
	if len(r.BalanceInfos) == 0 {
		u.Err = fmt.Errorf("deepseek: empty balance_infos")
		return u, u.Err
	}
```

- [ ] **Step 5: 跑测试验证通过**

Run: `go test ./internal/adapters/providers/deepseek/`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/providers/deepseek/deepseek.go internal/adapters/providers/deepseek/deepseek_test.go
git commit -m "feat(deepseek): map is_available to account Status"
```

---

### Task 4: Kimi 余额细分（voucher/cash）

**Files:**
- Modify: `internal/adapters/providers/kimi/kimi.go`（`FetchUsage`，约第 139-145 行 Dimensions 构建）
- Test: `internal/adapters/providers/kimi/kimi_test.go`

**Interfaces:**
- Consumes: `UsageDimension.Granted`/`ToppedUp`（Task 1）、`apiResp.Data.VoucherBalance`/`CashBalance`（struct 已有字段）
- Produces: Kimi `Dimensions[0].Granted`=voucher、`ToppedUp`=cash。

- [ ] **Step 1: 写失败测试 —— golden 补细分 + Status 断言**

在 `kimi_test.go` 的 `TestFetchUsageGolden` 内，现有 `d.Source` 断言（约第 104 行）之后追加：

```go
	if d.Granted != 46.58893 {
		t.Errorf("dim.Granted = %v, want 46.58893 (voucher_balance)", d.Granted)
	}
	if d.ToppedUp != 3.00001 {
		t.Errorf("dim.ToppedUp = %v, want 3.00001 (cash_balance)", d.ToppedUp)
	}
```

并在该用例的账号字段断言之前追加（验证 Kimi 不填状态）：

```go
	if u.Status != "" {
		t.Errorf("Status = %q, want empty (kimi has no is_available)", u.Status)
	}
```

- [ ] **Step 2: 跑测试验证失败**

Run: `go test ./internal/adapters/providers/kimi/`
Expected: FAIL —— `dim.Granted = 0, want 46.58893`。

- [ ] **Step 3: 实现 —— Dimensions 填 voucher/cash**

在 `kimi.go` 的 `FetchUsage` 中，找到 Dimensions 构建（约第 139 行），加 `Granted`/`ToppedUp` 两字段：

```go
	u.Dimensions = []domain.UsageDimension{{
		Name:        nameAvailable,
		Balance:     r.Data.AvailableBalance,
		Currency:    currencyFor(base),
		PercentUsed: -1,
		Source:      sourceTag,
		Granted:     r.Data.VoucherBalance, // 赠送券 → Granted
		ToppedUp:    r.Data.CashBalance,    // 现金 → ToppedUp
	}}
	u.Primary = &u.Dimensions[0] // 余额型：Primary 指向余额维度（不调 SelectPrimary）
	return u, nil
```

- [ ] **Step 4: 跑测试验证通过**

Run: `go test ./internal/adapters/providers/kimi/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/providers/kimi/kimi.go internal/adapters/providers/kimi/kimi_test.go
git commit -m "feat(kimi): surface voucher/cash balance breakdown"
```

---

### Task 5: 详情页 UI —— 余额细分行 + Status 行

**Files:**
- Modify: `internal/adapters/ui/account_details.go`（`renderDimension` 余额分支约第 210-215 行；`Render` 的 Basic Info 约第 83-85 行）
- Test: `internal/adapters/ui/account_details_test.go`

**Interfaces:**
- Consumes: `UsageDimension.Granted`/`ToppedUp`、`ProviderUsage.Status`（Task 1）
- Produces: 详情页渲染细分行（非零）与 Status 行（非空）。

- [ ] **Step 1: 写失败测试 —— 余额细分渲染**

在 `account_details_test.go` 末尾追加：

```go
// TestRenderDimensionBalanceBreakdown 验证余额型维度含非零细分时输出 Granted/Topped up 行。
func TestRenderDimensionBalanceBreakdown(t *testing.T) {
	dim := domain.UsageDimension{
		Name: "Available balance", Balance: 110, Currency: "CNY",
		Granted: 10, ToppedUp: 100, PercentUsed: -1,
	}
	got := renderDimension(dim)
	for _, want := range []string{"Balance:", "¥110.00", "Granted:", "¥10.00", "Topped up:", "¥100.00"} {
		if !strings.Contains(got, want) {
			t.Errorf("breakdown missing %q, got: %q", want, got)
		}
	}
}
```

- [ ] **Step 2: 写失败测试 —— 零细分不渲染**

在现有 `TestRenderDimensionBalance`（约第 116 行）末尾追加一条断言，确认零细分不输出 Granted 行：

```go
	if strings.Contains(got, "Granted:") {
		t.Errorf("zero Granted should not render Granted line, got: %q", got)
	}
```

- [ ] **Step 3: 写失败测试 —— Status 行**

在 `account_details_test.go` 末尾追加：

```go
// TestRenderStatusLine 验证 Status 非空时 Basic Info 含 Status 行；空时不渲染。
func TestRenderStatusLine(t *testing.T) {
	d := NewAccountDetails()
	u := domain.ProviderUsage{
		Provider: "deepseek", Label: "DS", Status: "active",
		Dimensions: []domain.UsageDimension{{Name: "Available balance", Balance: 1, Currency: "CNY", PercentUsed: -1}},
	}
	u.Primary = &u.Dimensions[0]

	d.Render(u)
	if got := d.GetText(true); !strings.Contains(got, "Status:") || !strings.Contains(got, "active") {
		t.Errorf("should render 'Status: active', got: %q", got)
	}

	u.Status = "" // 空 Status 不渲染该行
	d.Render(u)
	if strings.Contains(d.GetText(true), "Status:") {
		t.Error("empty Status should not render Status line")
	}
}
```

- [ ] **Step 4: 跑测试验证失败**

Run: `go test ./internal/adapters/ui/`
Expected: FAIL —— 缺 "Granted:" / "Topped up:" / "Status:"。

- [ ] **Step 5: 实现 —— renderDimension 余额分支加细分行**

在 `account_details.go` 的 `renderDimension` 中，找到余额分支（`// 余额型：只显示 Balance 行` 注释处），替换为：

```go
	// 余额型：显示 Balance 行，不画进度条（余额无进度语义）；非零细分追加 Granted/Topped up。
	if dim.Currency != "" {
		fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
			colorSecondary, "Balance:", colorPrimary, formatMoney(dim.Balance, dim.Currency))
		if dim.Granted != 0 {
			fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
				colorSecondary, "Granted:", colorPrimary, formatMoney(dim.Granted, dim.Currency))
		}
		if dim.ToppedUp != 0 {
			fmt.Fprintf(&b, "    [%s]%-10s[-]  [%s]%s[-]\n",
				colorSecondary, "Topped up:", colorPrimary, formatMoney(dim.ToppedUp, dim.Currency))
		}
		b.WriteString("\n")
		return b.String()
	}
```

- [ ] **Step 6: 实现 —— Render 加 Status 行**

在 `account_details.go` 的 `Render` 中，找到 `APIKeyStatus` 渲染块（约第 83-85 行），在其后追加 Status：

```go
	if u.APIKeyStatus != "" {
		b.WriteString(basicInfoLine("API Key", u.APIKeyStatus))
	}
	if u.Status != "" {
		b.WriteString(basicInfoLine("Status", u.Status))
	}
```

- [ ] **Step 7: 跑测试验证通过**

Run: `go test ./internal/adapters/ui/`
Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/ui/account_details.go internal/adapters/ui/account_details_test.go
git commit -m "feat(ui): render balance breakdown and account status in details"
```

---

### Task 6: 全量回归与质量门禁

**Files:**
- 无源码改动（仅验证；若 `make quality` 触发格式化则提交格式修正）。

- [ ] **Step 1: 全量测试（race + cover）**

Run: `make test`
Expected: 全绿，无 race 警告。

- [ ] **Step 2: 质量（fmt + vet）**

Run: `make quality`
Expected: 无报错。若 `gofumpt`/`go fmt` 改动了文件：

```bash
git add -A && git commit -m "style: gofmt balance breakdown changes"
```

- [ ] **Step 3: lint**

Run: `make lint`
Expected: 无新增告警。

- [ ] **Step 4: 二进制构建确认**

Run: `go build ./...`
Expected: 成功。

---

## Self-Review 结论

- **Spec 覆盖**：§3 模型 → Task 1；§4.1 deepseek 细分+容错 → Task 2，Status → Task 3；§4.2 kimi → Task 4；§5 UI → Task 5；§6 测试用例（golden 补断言、unavailable、bad-granted、kimi 细分、UI 细分/零值/Status）均落到对应任务的 Step。无遗漏。
- **占位符扫描**：无 TBD/TODO；每个代码 step 含真实可跑代码。
- **类型一致性**：`Granted`/`ToppedUp`（float64）、`Status`（string）在 domain→adapter→UI 全链路命名一致；`granted, _ :=` 容错形态与 spec §4.1 修正后一致。
