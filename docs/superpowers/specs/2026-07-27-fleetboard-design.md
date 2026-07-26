# fleetboard 设计规格

> 状态：草案 v1 · 日期：2026-07-27 · 作者：MeePwn

## 1. 概述

**fleetboard** 是一个终端（TUI）仪表盘，聚合用户在**各家 AI Coding 厂商**（智谱 GLM、MiniMax、Kimi、Anthropic、OpenAI、Cursor、Copilot 等）的**订阅/额度用量**，实时回答三个问题：

1. **现在用了多少？** —— 按百分比 + 绝对总量展示
2. **什么时候重置？** —— 各家重置窗口不同（5 小时滚动 / 每日 / 每月），显示倒计时
3. **哪个号还能用？** —— 多账号横向对比，一眼挑出未触顶的号

数据**全部来自各厂商服务端接口**，本地零解析。应用职责仅三件事：**配置账号 → 调接口取用量 → 归一化展示**。

## 2. 背景与动机

- 用户在多台设备（OPPO Pad mini/Termux、MacBook Pro、Arch Linux、Windows、N 台 Linux）上通过 Tailscale 互联，重度使用多家 AI Coding 服务。
- 各家额度模型五花八门（Claude 5 小时滚动窗口、MiniMax Token Plan 按月、国内厂商各异），跨多账号时根本记不清"哪家还能用、何时重置"。
- 现有工具 `cc-switch`（farion1231/cc-switch）主打**供应商配置切换**，用量查询是附属且社区反馈经常失效（Issue #735「新增官方账号剩余额度统计」、#1105、#1928、#2300 均在呼吁）。fleetboard 反过来以**用量监控为核心**。
- UI/交互/架构**严格继承作者自己的 `lazytmux` / `lazyssh` 工具栈**（Go + cobra + tview + Tokyo Night + 六边形架构），保证一致的肌肉记忆与代码风格。

## 3. 目标与非目标

### 目标（MVP）
- 自管配置（`~/.fleetboard/config.yaml`）：供应商 + 账号 + token + URL
- 并发调用各厂商服务端接口取用量，归一化为统一展示模型
- TUI 总览：每行一个账号，平台彩色 tag + 百分比 + 状态点；右侧详情面板
- 两种刷新：选中账号刷新 / 全部账号刷新
- 首批对接：智谱 GLM、MiniMax（接口已验证可得），Kimi（待确认端点）

### 非目标（MVP 不做，留待后续）
- 不解析本地 `~/.claude/` 用量文件（数据一律以服务端为准）
- 不做跨设备聚合（账户级余额天然跨设备，MVP 单机即可）
- 不做配置切换（那是 cc-switch 的职责，fleetboard 专注用量展示）
- 不做历史趋势图表（后续迭代）
- 不做 GUI（Termux 只能跑 TUI）

## 4. MVP 范围与交付分期

| 阶段 | 内容 | 验证标准 |
|------|------|---------|
| **P0 骨架** | 六边形架构骨架 + 配置读写 + TUI 壳（移植 lazytmux 布局）+ mock adapter | 能加载配置、展示空列表、footer/快捷键可响应 |
| **P1 首批 adapter** | GLM adapter + MiniMax adapter（真实接口） | 两个真实账号能拉到用量并展示进度条 |
| **P2 完善** | Kimi adapter + 容错/重试/缓存 + 详情面板 + 两种刷新 | 多账号并发拉取，单点失败不连坐 |
| **P3 扩展** | Anthropic / OpenAI / Cursor / 国内其他厂商 adapter | 按 adapter 模式增量接入 |

## 5. 整体架构（六边形 / 端口适配器，继承 lazy 系列）

```
cmd/main.go                          → cobra 根命令，加载配置 + 装配依赖
internal/core/domain/                → Account / VendorUsage / ResetPolicy 领域模型
internal/core/ports/                 → UsageProvider / ConfigStore / View 端口
internal/core/services/              → 聚合服务：并发调多账号 → 归一化展示模型
internal/adapters/providers/         → 各厂商 adapter（每个调对应服务端接口）
   ├── glm/        glm.go
   ├── minimax/    minimax.go
   ├── kimi/       kimi.go
   └── ...         （按需扩展）
internal/adapters/config/yaml/       → ~/.fleetboard/config.yaml 读写（原子写 + 备份，学 lazyssh）
internal/adapters/ui/                → tview TUI（Tokyo Night，移植 lazytmux）
   ├── tui.go        布局装配（header + content[left:search+list | right:details] + statusbar）
   ├── header.go / account_list.go / account_details.go / status_bar.go / search_bar.go
   ├── account_form.go / help.go / sort.go / theme.go / const.go
internal/logger/                     → zap → ~/.fleetboard/fleetboard.log
```

**设计原则**：
- `UsageProvider` 是唯一对外数据端口；新增一家厂商 = 加一个 `providers/<name>.go`，零侵入。
- UI 层只依赖 `core` 的归一化模型，不直接耦合任何厂商接口。
- 配置/日志/状态目录统一在 `~/.fleetboard/`（学 lazytmux 的 `~/.lazytmux/`）。

## 6. 核心数据模型

```go
// internal/core/domain/account.go
type Account struct {
    ID       string   // 配置内唯一 id，如 "glm-main"
    Vendor   string   // "glm" | "minimax" | "kimi" | "anthropic" | "openai" | ...
    Label    string   // 显示名，如 "智谱编码-主力"
    BaseURL  string   // 可选，覆盖默认（支持中转/网关）
    TokenEnv string   // 环境变量名，token 从此读取，不明文落盘
}

// internal/core/domain/reset_policy.go
type ResetPolicy string
const (
    ResetRolling5h ResetPolicy = "rolling5h" // 5 小时滚动窗口
    ResetDaily     ResetPolicy = "daily"     // 每日 00:00 重置
    ResetMonthly   ResetPolicy = "monthly"   // 每月 1 日 00:00 重置
    ResetCustom    ResetPolicy = "custom"    // 接口直接返回 resetsAt
)

// internal/core/domain/vendor_usage.go
type VendorUsage struct {
    AccountID    string
    Vendor       string
    Label        string
    Used         int64       // 接口返回的已用量
    Limit        int64       // 接口返回的上限；缺失时无百分比
    PercentUsed  float64     // Used / Limit，Limit<=0 时为 -1（N/A）
    Remaining    int64
    ResetsAt     time.Time   // 接口返回 or 按 ResetPolicy 推算
    Source       string      // "api-balanced" | "api-estimate"，标可信度
    Raw          string      // 原始响应摘要（调试用，脱敏）
    FetchedAt    time.Time
    Err          error       // 单账号失败不连坐，UI 标红继续展示其他
}
```

## 7. Provider adapter 接口与首批实现

```go
// internal/core/ports/usage_provider.go
type UsageProvider interface {
    Vendor() string
    FetchUsage(ctx context.Context, acc domain.Account) (domain.VendorUsage, error)
}
```

**首批三个 adapter**（每个极薄：一个 HTTP GET + 一个响应解析）：

| adapter | 端点 / 来源 | 鉴权 | 备注 |
|---------|------------|------|------|
| `providers/glm` | 智谱 Coding Plan 用量接口 | API Key | 参考官方 `zai-coding-plugins/glm-plan-usage` 插件规格 |
| `providers/minimax` | `GET https://api.minimaxi.com/v1/token_plan/remains` | 订阅 Key | 端点已由 openclaw 文档确认 |
| `providers/kimi` | 待确认（platform.kimi.com console） | API Key | 先留 stub，端点明确后补 |

响应解析统一产出 `VendorUsage`；接口不返回上限时 `Limit=0`（百分比显示 N/A）。

## 8. 配置文件 `~/.fleetboard/config.yaml`

```yaml
accounts:
  - id: glm-main
    vendor: glm
    label: 智谱编码-主力
    token_env: GLM_API_KEY          # token 从环境变量读，不明文落盘
  - id: minimax-pro
    vendor: minimax
    label: MiniMax Token Plan
    token_env: MINIMAX_API_KEY
    base_url: https://api.minimaxi.com   # 可选，覆盖默认
refresh:
  on_start: true
  interval: 5m                         # 后台自动刷新间隔（仅全部刷新）
ui:
  theme: tokyo-night
```

- 写入：原子写（临时文件 + rename）+ 滚动备份（最多保留 10 份），学 lazyssh 的非破坏性写入。
- 文件权限 `0600`。

## 9. UI 设计（严格遵循 lazytmux 布局）

### 9.1 布局（直接复刻 lazytmux `buildLayout`，比例 3:2）

```
root = FlexRow {
    header        (高 2)
    content = FlexColumn {
        left  (3/5) = FlexRow { search_bar (高 3), account_list }
        right (2/5) = FlexRow { account_details }
    }
    status_bar    (高 1)
}
```

### 9.2 列表项（单行 `tview.List`，无迷你进度条）

每行内容：`配置名` + `[平台 tag · 带背景色]` + `百分比` + `状态点`

- 平台 tag：用 tview `[fg:bg]` 双色语法渲染色块，背景色取自 `theme.go` 的 `vendorColor` map：
  - `glm`=#7C3AED（紫）、`minimax`=#EF4444（红）、`kimi`=#06B6D4（青）、`anthropic`=#D97757（橙）、`openai`=#10A37F（绿）、`cursor`=#6366F1（靛）、`copilot`=#0969DA（蓝）
- 状态点 `●`：`<70%` 绿 / `70-90%` 黄 / `>90%` 红 / 无数据或失败 灰 `○`
- 选中行：整行高亮（`colorSelected` 背景），与 lazytmux 一致

### 9.3 详情面板（右侧）

选中账号的详细信息：
- **大进度条**（三色：<70% 绿 / 70-90% 黄 / >90% 红）
- 已用 / 上限 / 剩余（带"⚠ 接近上限"提示）
- 重置时间（"2d 后（每月 1 日 00:00）"格式）
- 接口名（如 `token_plan/remains`）
- 拉取时间 + 数据来源（`api-balanced` / `api-estimate`）
- 失败时显示错误原因（红色）

### 9.4 footer（严格遵循 lazytmux `status_bar.go`）

单行居中 `TextView`，快捷键字母 cyan 高亮，组间用 `•` 分隔：

```
↑↓ Navigate • r Refresh • R Refresh All • a New • e Edit • d Delete • / Search • s Sort • ? Help • q Quit
```

- **`r` = 刷新选中账号**，**`R` = 刷新全部账号**（两种刷新，大小写区分粒度）
- **右侧不显示**上次刷新时间（已移入详情面板的"拉取"行）
- 临时错误用 `SetStatus(msg)` 覆盖提示，稍后 `ResetHints()` 恢复
- 空状态用 `ShowEmpty()`（仅 `a New • ? Help • q Quit`）

### 9.5 快捷键总表

| 键 | 动作 | 键 | 动作 |
|----|------|----|------|
| `↑↓` | 导航 | `r` | 刷新选中 |
| `←/→` | 列表 ↔ 详情切换焦点 | `R` | 刷新全部 |
| `Enter` | （详情聚焦/查看原始数据） | `a` | 新增账号 |
| `/` | 搜索 | `e` | 编辑账号 |
| `s`/`S` | 切换排序字段 | `d` | 删除账号 |
| `?` | 帮助 | `q` | 退出 |

## 10. 运行时（刷新 / 缓存 / 容错）

- **启动**：加载配置 → 全量拉取一次（`refresh.on_start`）
- **两种刷新**：
  - `r`：仅重新拉取当前选中账号
  - `R`：并发重新拉取所有账号
- **后台自动刷新**：按 `refresh.interval`（默认 5m）定时全部刷新
- **并发**：所有账号并发拉取；单账号失败**不连坐**，UI 标红继续展示其他
- **缓存**：`~/.fleetboard/cache.json`（带 TTL），避免重启立刻重打接口
- **请求**：每请求超时（默认 10s）+ 指数退避重试（最多 2 次）
- **日志**：`~/.fleetboard/fleetboard.log`（zap，脱敏）

## 11. 安全

- token 默认走 `token_env`（环境变量），不明文落 yaml
- 配置文件权限 `0600`；原子写 + 备份
- 日志脱敏：token / Authorization 头永不打印
- 不存储、不转发任何 token 到第三方（仅直连各厂商官方接口）

## 12. 测试策略

- **adapter**：`httptest.Server` mock 各厂商响应，每家一个 golden test（响应快照 → 解析 → 断言归一化结果）
- **聚合服务**：测并发拉取 + 单点失败隔离（一个失败不影响其他）
- **配置读写**：测原子性 + 备份保留数量
- **UI**：只测归一化数据与 `formatAccountLine` 的渲染输出，不测 tview 渲染本身
- 沿用 lazytmux 的测试风格（表驱动 + golden）

## 13. 技术栈与工程约定（对齐 lazy 系列）

- Go + cobra + tview/tcell + zap
- 六边形架构（ports & adapters）
- Tokyo Night 主题（移植 lazytmux `theme.go`）
- Homebrew tap `maybewaityou/tap` + goreleaser 交叉编译（darwin/linux × amd64/arm64）
- 语义化提交：`type(scope): 简短描述`（feat/fix/improve/refactor/docs/test/ci/chore）
- 中英双 README
- License：Apache 2.0（与 lazytmux/lazyssh 一致）

## 14. 开放问题（实现阶段确认）

1. **Kimi 用量端点**：需在 platform.kimi.com console 确认是否有公开用量 API；若无，Kimi adapter 暂留 stub 或回退到"用户手填上限"。
2. **删除键**：当前定 `d` = Delete（fleetboard 无 detach 概念，与 lazytmux 的 d=Detach 不同）。可在实现时调整。
3. **GLM 接口细节**：需读 `zai-coding-plugins/glm-plan-usage` 源码确认精确端点与响应字段。
4. **平台配色**：`vendorColor` 色值为初步建议，可在实现时按品牌色微调。
5. **`Enter` 键语义**：详情面板常驻右侧，`Enter` 可用于"聚焦详情"或"打开该平台控制台 URL"，待定。
