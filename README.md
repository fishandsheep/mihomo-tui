# mihomo-tui

`mihomo-tui` is a standalone terminal UI for Clash/Mihomo-compatible
`external-controller` APIs.

## Quick Start

Requirements:

- Mihomo or Clash-compatible controller already running
- `external-controller` enabled, for example `http://127.0.0.1:9090` or unix socket `/tmp/verge/verge-mihomo.sock`
- controller `secret` available if authentication enabled
- Node.js or Bun installed if you want to run from npm package

Run without installing:

```bash
npx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
npx mihomo-tui@latest open --unix-socket /tmp/verge/verge-mihomo.sock --secret your-secret
```

Or with Bun:

```bash
bunx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
bunx mihomo-tui@latest open --unix-socket /tmp/verge/verge-mihomo.sock --secret your-secret
```

Save controller as reusable profile:

```bash
npx mihomo-tui@latest profile add \
  --name local \
  --controller http://127.0.0.1:9090 \
  --secret your-secret \
  --default
```

Open later with saved profile:

```bash
npx mihomo-tui@latest open --profile local
```

Check installed version:

```bash
npx mihomo-tui@latest --version
```

If you prefer local binary:

```bash
go build -o ./bin/mihomo-tui ./cmd/tui
./bin/mihomo-tui open --controller http://127.0.0.1:9090 --secret your-secret
./bin/mihomo-tui open --unix-socket /tmp/verge/verge-mihomo.sock --secret your-secret
```

## Features

- multi-session controller switching
- mode switching: `rule`, `global`, `direct`
- independent TUN `on/off` toggle
- proxy group browsing
- node switching
- nested group node visibility for fallback/auto-select entries
- delay testing via `http://cp.cloudflare.com`
- public IP info in Main via `https://ipinfo.io/json`, refreshed every 60s
- full-screen, resize-safe, lazygit-inspired terminal layout
- mouse-aware panes with double-click apply
- mode-aware group filtering: `rule` shows `Halsh Cloud`, `global` shows `GLOBAL`

## Interaction

- `tab` cycles panes
- `j/k` or arrows move inside active pane
- `pgup/pgdn`, `home/end` scroll long panes
- `space` applies action in active pane
  - modes: switch mode
  - TUN: toggle `on/off`
  - nodes: switch proxy
- `r` refreshes controller data and public IP info
- mouse
  - single click: focus/select
  - double click: apply action
  - wheel: scroll hovered pane

## Main Pane

The Main pane shows controller/session details, selected group/node details, event
history, and public IP info from `ipinfo.io`. IP info refreshes every 60 seconds;
the Main header and Inspector detail show the refresh countdown.

When the active controller exposes `mixed-port` or `port` in `/configs`, IP info
requests are sent through that local Mihomo proxy port so changing Nodes updates
the observed public IP. Unix socket controllers cannot expose a proxy host, so IP
info falls back to direct requests unless a controller URL is used.

Fallback, URLTest, and selector entries in the Nodes pane show their concrete
selected node, for example `故障转移  [VIP1 英国]  [up] -`.

## Build

```bash
go build -o ./bin/mihomo-tui ./cmd/tui
```

Package distribution:

```bash
npx mihomo-tui@latest --version
bunx mihomo-tui@latest --version
```

## Run

Direct controller:

```bash
./bin/mihomo-tui open --controller http://127.0.0.1:9090 --secret your-secret
./bin/mihomo-tui open --unix-socket /tmp/verge/verge-mihomo.sock --secret your-secret
npx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
bunx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
```

Saved profile:

```bash
./bin/mihomo-tui profile add --name local --controller http://127.0.0.1:9090 --secret your-secret --default
./bin/mihomo-tui profile add --name verge --unix-socket /tmp/verge/verge-mihomo.sock --secret your-secret
./bin/mihomo-tui open --profile local
./bin/mihomo-tui open --profile verge
```

## Test

```bash
go test ./...
```

## Release

Prepare version metadata across npm workspace packages:

```bash
bun run prepare:release v0.3.0-beta.1
```

Build release payloads and smoke test package launchers:

```bash
bun run build:binaries --version v0.3.0-beta.1
bun run build:packages
bun run test:launchers
```

Publish all npm packages with dist-tag derived from version:

```bash
bun run publish:packages --version v0.3.0-beta.1
```

Override tag or dry-run when needed:

```bash
bun run publish:packages --version v0.3.0-rc.1 --tag next
bun run publish:packages --version v0.3.0-beta.1 --dry-run
```

GitHub Actions `release` workflow runs on `v*` tags and also supports manual dispatch.
