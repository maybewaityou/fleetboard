<div align="center">

# fleetboard

**终端里的 AI Coding 套餐额度 / 余额仪表盘。**

一屏聚合 GLM、MiniMax、Kimi、DeepSeek 及自建中转平台（new-api / sub2api）的额度与余额——用了多少、何时重置、哪个号还能用。

[English](README.md) · [简体中文](README.zh-CN.md)

</div>

## ✨ 功能

- **一屏看全部厂商** —— 每行一个账号：标签、厂商色块、用量百分比、状态点。
- **额度 + 余额** —— 百分比窗口（GLM 5 小时 / 每周 / 每月、MiniMax）**与**账户余额（Kimi、DeepSeek、new-api、sub2api）。
- **最短窗口优先** —— 列表始终展示最短的那档额度（如 GLM 滚动 5 小时窗口），即便该档没有返回重置时间也照常置顶，最紧迫的额度始终可见。
- **两级刷新** —— `r` 刷新选中账号，`R` 刷新全部账号。
- **手动增删改** —— 新增 / 编辑 / 删除 / 置顶账号；配置存于 `~/.fleetboard/config.yaml`。
- **搜索与排序** —— `/` 过滤，`s`/`S` 循环排序（名称 / 用量 / 最近刷新）。
- **Tokyo Night 主题** TUI，移植自 `lazytmux` / `lazyssh` 工具家族。

## 📡 支持的厂商

| 厂商          | 类型   | 展示内容                          | `base_url` |
|---------------|--------|-----------------------------------|------------|
| `glm`         | 配额型 | 5 小时 / 每周 / 每月用量百分比窗口 | 可选       |
| `minimax`     | 配额型 | 用量百分比窗口                    | 可选       |
| `kimi`        | 余额型 | 可用余额（CNY / USD）             | 可选       |
| `deepseek`    | 余额型 | 可用余额（CNY）                   | 可选       |
| `siliconflow` | 余额型 | 可用余额（CNY）+ 充值/总额        | 可选       |
| `sub2api`     | 余额型 | 可用余额（USD）                   | **必填**   |
| `newapi`      | 余额型 | 可用余额（USD）                   | **必填**   |

配额型厂商返回 已用 / 上限 / 百分比 / 重置窗口；余额型厂商返回剩余余额。自建中转平台（`sub2api`、`new-api`）没有默认域名，`base_url` 必填。new-api 的 `provider` 取值为 `newapi`（无连字符）。

## 🔒 工作原理

fleetboard 读取账号配置，调用各厂商官方的用量 / 余额接口，归一化后渲染。token 从（账号指定的）环境变量读取，绝不落盘，也只发往该厂商自己的接口。本地解析 `~/.claude/` 用量文件不在范围内——服务端是唯一数据源。

## 📦 安装

### 方式一：Homebrew（macOS）

```bash
brew install maybewaityou/tap/fleetboard
```

较新版 Homebrew 首次安装若提示 tap 不可信：

```bash
brew trust maybewaityou/tap
```

### 方式二：从 Releases 下载二进制

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/maybewaityou/fleetboard/releases/latest | jq -r .tag_name)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
esac
curl -LJO "https://github.com/maybewaityou/fleetboard/releases/download/${LATEST_TAG}/fleetboard_${OS}_${ARCH}.tar.gz"
tar -xzf fleetboard_${OS}_${ARCH}.tar.gz
sudo mv fleetboard /usr/local/bin/
fleetboard
```

### 方式三：源码编译

```bash
git clone https://github.com/maybewaityou/fleetboard.git
cd fleetboard
make build
sudo mv bin/fleetboard /usr/local/bin/
# 或不安装直接运行
make run
```

### 配置

创建 `~/.fleetboard/config.yaml`：

> **破坏性变更 (v0.2.0)：** new-api 账号须将 `token_env` 迁移为 `access_token_env` + `user_id`（见下）。旧的 OpenAI billing 端点返回的是占位假数据，fleetboard 现改读 new-api 原生层。

```yaml
accounts:
  - id: glm-main
    provider: glm
    label: 智谱编码-主力
    token_env: GLM_API_KEY
  - id: kimi-main
    provider: kimi
    label: Kimi
    token_env: MOONSHOT_API_KEY
  - id: siliconflow-main
    provider: siliconflow
    label: SiliconFlow 主账号
    token_env: SILICONFLOW_API_KEY
  # 自建中转平台：base_url 必填（无默认域名）。
  - id: my-newapi
    provider: newapi
    label: new-api 中转
    base_url: https://relay.example.com
    access_token_env: NEWAPI_AT   # 存 access_token 的环境变量
    user_id: "16002"              # new-api 用户 ID
  - id: my-sub2api
    provider: sub2api
    label: sub2api 中转
    base_url: https://sub.example.com
    token_env: SUB2API_API_KEY
refresh:
  on_start: true
  interval: 5m
ui:
  theme: tokyo-night
```

**获取 access_token 与 user_id**：
- access_token：new-api 后台 → 个人设置 → 系统访问令牌 → 生成。
- user_id：浏览器 F12 → Network → 任一 `/api/` 请求的 `New-Api-User` 请求头，或 Local Storage 的 `user.id`。

> 注：new-api 的 OpenAI 兼容 billing 端点（`/v1/dashboard/billing/*`）返回的是占位假数据，
> fleetboard 改用原生 `/api/*` 层获取真实余额与近 7/30 天消耗。

导出账号引用的 token 环境变量，然后运行 `fleetboard`。

> **homebrew-tap 维护者注意：** 发布工作流通过仓库 secret `HOMEBREW_TAP_GITHUB_TOKEN`（对 `maybewaityou/homebrew-tap` 有 `contents:write` 的 PAT）推送 formula。

## ⌨️ 快捷键

| 键 | 动作 | 键 | 动作 |
|----|------|----|------|
| `↑↓` | 移动 | `r` | 刷新选中 |
| `←/→` | 列表 / 详情切换焦点 | `R` | 刷新全部 |
| `/` | 搜索 | `a` | 新增账号 |
| `s`/`S` | 循环排序 | `e` | 编辑账号 |
| `p` | 置顶 / 取消 | `d` | 删除账号 |
| `?` | 帮助 | `q` | 退出 |

## 🏗 架构

fleetboard 采用六边形（端口适配器）架构，与 `lazytmux`/`lazyssh` 一致：

```
cmd/main.go                          → cobra 根命令：加载配置 + 装配依赖
internal/core/domain/                → Account / ProviderUsage / UsageDimension
internal/core/ports/                 → UsageProvider / ConfigStore / View
internal/core/services/              → Aggregator：并发拉取，单点失败不连坐
internal/adapters/providers/         → 每家厂商一个 adapter（glm、minimax、kimi、deepseek …）
internal/adapters/config/yaml/       → ~/.fleetboard/config.yaml（原子写 + 备份）
internal/adapters/ui/                → tview TUI（Tokyo Night）
```

新增厂商 = 在 `internal/adapters/providers/<name>/` 放一个文件，并在 `cmd/main.go` 注册。

## 🤝 贡献

语义化提交：`type(scope): 简短描述`
（`feat`、`fix`、`improve`、`refactor`、`docs`、`test`、`ci`、`chore`）。

## ⭐ 支持

如果 fleetboard 帮到了你，欢迎点个 Star。

### ☕ 赞助

如果愿意支持开发：

<a href="https://www.buymeacoffee.com/maybewaityou" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" width="200" /></a>

**微信 / 支付宝**

<table>
  <tr>
    <td align="center">
      <img src="./docs/resources/donate-wechat.jpg" alt="微信" width="180" />
      <br/>
      <b>微信</b>
    </td>
    <td width="80"></td>
    <td align="center">
      <img src="./docs/resources/donate-alipay.jpg" alt="支付宝" width="180" />
      <br/>
      <b>支付宝</b>
    </td>
  </tr>
</table>

## 🙏 致谢

- [`lazytmux`](https://github.com/maybewaityou/lazytmux) / `lazyssh` —— fleetboard 移植的 TUI 布局、主题与架构。
- [`cc-switch`](https://github.com/farion1231/cc-switch) —— 厂商用量端点的参考。

## 许可证

Apache-2.0，详见 [LICENSE](LICENSE)。
