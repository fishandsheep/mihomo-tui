# mihomo-tui

`mihomo-tui` is a standalone terminal UI for Clash/Mihomo-compatible
`external-controller` APIs.

## Features

- multi-session controller switching
- mode switching: `rule`, `global`, `direct`
- independent TUN `on/off` toggle
- proxy group browsing
- node switching
- delay testing
- full-screen, resize-safe, lazygit-inspired terminal layout
- mouse-aware panes with double-click apply

## Quick Start

Requirements:

- Mihomo or Clash-compatible controller already running
- `external-controller` enabled, for example `http://127.0.0.1:9090`
- controller `secret` available if authentication enabled
- Node.js or Bun installed if you want to run from npm package

Run without installing:

```bash
npx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
```

Or with Bun:

```bash
bunx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
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
```

## Interaction

- `tab` cycles panes
- `j/k` or arrows move inside active pane
- `pgup/pgdn`, `home/end` scroll long panes
- `space` applies action in active pane
  - modes: switch mode
  - TUN: toggle `on/off`
  - nodes: switch proxy
- mouse
  - single click: focus/select
  - double click: apply action
  - wheel: scroll hovered pane

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
npx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
bunx mihomo-tui@latest open --controller http://127.0.0.1:9090 --secret your-secret
```

Saved profile:

```bash
./bin/mihomo-tui profile add --name local --controller http://127.0.0.1:9090 --secret your-secret --default
./bin/mihomo-tui open --profile local
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
