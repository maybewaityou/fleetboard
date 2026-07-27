<div align="center">

# fleetboard

**A terminal dashboard for AI coding-plan usage &amp; balance across providers.**

See quota and balance for GLM, MiniMax, Kimi, DeepSeek, and self-hosted relays
(new-api / sub2api) in one screen — how much is used, when it resets, and which
account still has headroom.

[English](README.md) · [简体中文](README.zh-CN.md)

</div>

## ✨ Features

- **One screen, all providers** — each account is a row: label, provider chip, usage %, status dot.
- **Quota + balance** — percentage windows (GLM 5h/weekly/monthly, MiniMax) **and** account balance (Kimi, DeepSeek, new-api, sub2api).
- **Nearest-window priority** — the list surfaces the quota window that resets soonest, so the most urgent tier is always visible.
- **Two refresh granularities** — `r` re-fetches the selected account, `R` re-fetches all.
- **Manual CRUD** — add / edit / delete / pin accounts; config lives in `~/.fleetboard/config.yaml`.
- **Search & sort** — `/` to filter, `s`/`S` to cycle sort (name / usage / refreshed).
- **Tokyo Night themed** TUI, ported from the `lazytmux` / `lazyssh` tool family.

## 📡 Supported providers

| Provider   | Type    | Shows                                  | `base_url`  |
|------------|---------|----------------------------------------|-------------|
| `glm`      | Quota   | 5h / weekly / monthly usage % windows  | optional    |
| `minimax`  | Quota   | usage % window                         | optional    |
| `kimi`     | Balance | available balance (CNY / USD)          | optional    |
| `deepseek` | Balance | available balance (CNY)                | optional    |
| `sub2api`  | Balance | available balance (USD)                | **required** |
| `newapi`   | Balance | available balance (USD)                | **required** |

Quota-type providers report used / limit / percent / reset window; balance-type
providers report a remaining balance. Self-hosted relays (`sub2api`, `new-api`)
have no default domain, so `base_url` is mandatory. The `provider` value for
new-api is `newapi` (no hyphen).

## 🔒 How it works

fleetboard reads your account config, calls each provider's official usage/balance API, normalizes the result, and renders it. Tokens are read from environment variables (named per account) and never written to disk or sent anywhere except the provider's own API. Local parsing of `~/.claude/` usage files is intentionally out of scope — the server is the source of truth.

## 📦 Installation

### Option 1: Homebrew (macOS)

```bash
brew install maybewaityou/tap/fleetboard
```

On newer Homebrew, if the first install warns about an untrusted tap:

```bash
brew trust maybewaityou/tap
```

### Option 2: Download a binary from Releases

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

### Option 3: Build from source

```bash
git clone https://github.com/maybewaityou/fleetboard.git
cd fleetboard
make build
sudo mv bin/fleetboard /usr/local/bin/
# Or run it directly without installing
make run
```

### Configuration

Create `~/.fleetboard/config.yaml`:

> **Breaking (v0.2.0):** new-api accounts must migrate `token_env` → `access_token_env` + `user_id` (see below). The old OpenAI billing endpoints returned placeholder data; fleetboard now reads new-api's native layer.

```yaml
accounts:
  - id: glm-main
    provider: glm
    label: GLM main
    token_env: GLM_API_KEY
  - id: kimi-main
    provider: kimi
    label: Kimi
    token_env: MOONSHOT_API_KEY
  # Self-hosted relays: base_url is required (no default domain).
  - id: my-newapi
    provider: newapi
    label: new-api relay
    base_url: https://relay.example.com
    access_token_env: NEWAPI_AT   # Access token from new-api backend
    user_id: "16002"              # new-api user ID
  - id: my-sub2api
    provider: sub2api
    label: sub2api relay
    base_url: https://sub.example.com
    token_env: SUB2API_API_KEY
refresh:
  on_start: true
  interval: 5m
ui:
  theme: tokyo-night
```

**Getting `access_token` and `user_id`**:
- `access_token`: Go to new-api backend → Settings → System Access Token → Generate.
- `user_id`: In browser, open F12 → Network → Any `/api/*` request's `New-Api-User` header, or `localStorage.getItem('user.id')`.

> **Note:** new-api's OpenAI-compatible billing endpoints (`/v1/dashboard/billing/*`) return fake placeholder data. fleetboard uses the native `/api/*` layer to fetch real balance and recent 7d/30d usage.

Export the tokens the accounts reference, then run `fleetboard`.

> **Note for `homebrew-tap` maintainers:** the release workflow pushes the formula via the `HOMEBREW_TAP_GITHUB_TOKEN` repo secret — a PAT with `contents:write` on `maybewaityou/homebrew-tap`.

## ⌨️ Key Bindings

| Key | Action | Key | Action |
|-----|--------|-----|--------|
| `↑↓` | Move | `r` | Refresh selected |
| `←/→` | Focus list / details | `R` | Refresh all |
| `/` | Search | `a` | New account |
| `s`/`S` | Cycle sort | `e` | Edit account |
| `p` | Pin / unpin | `d` | Delete account |
| `?` | Help | `q` | Quit |

## 🏗 Architecture

fleetboard follows a hexagonal (ports &amp; adapters) layout, shared with `lazytmux`/`lazyssh`:

```
cmd/main.go                          → cobra root: load config + wire adapters
internal/core/domain/                → Account / ProviderUsage / UsageDimension
internal/core/ports/                 → UsageProvider / ConfigStore / View
internal/core/services/              → Aggregator: concurrent fetch, fault-isolated
internal/adapters/providers/         → one adapter per provider (glm, minimax, kimi, deepseek, ...)
internal/adapters/config/yaml/       → ~/.fleetboard/config.yaml (atomic write + backups)
internal/adapters/ui/                → tview TUI (Tokyo Night)
```

Adding a provider = drop one file in `internal/adapters/providers/<name>/` and register it in `cmd/main.go`.

## 🤝 Contributing

Semantic commit messages: `type(scope): short description`
(`feat`, `fix`, `improve`, `refactor`, `docs`, `test`, `ci`, `chore`).

## ⭐ Support

If fleetboard saves you some time, a star is appreciated.

### ☕ Sponsor

If you'd like to support development:

<a href="https://www.buymeacoffee.com/maybewaityou" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" width="200" /></a>

**WeChat Pay / Alipay**

<table>
  <tr>
    <td align="center">
      <img src="./docs/resources/donate-wechat.jpg" alt="WeChat Pay" width="180" />
      <br/>
      <b>WeChat Pay</b>
    </td>
    <td width="80"></td>
    <td align="center">
      <img src="./docs/resources/donate-alipay.jpg" alt="Alipay" width="180" />
      <br/>
      <b>Alipay</b>
    </td>
  </tr>
</table>

## 🙏 Acknowledgments

- [`lazytmux`](https://github.com/maybewaityou/lazytmux) / `lazyssh` — the TUI layout, theme, and architecture fleetboard is ported from.
- [`cc-switch`](https://github.com/farion1231/cc-switch) — reference for provider usage endpoints.

## License

Apache-2.0. See [LICENSE](LICENSE).
