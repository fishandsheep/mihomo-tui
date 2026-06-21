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

## Run

Direct controller:

```bash
./bin/mihomo-tui open --controller http://127.0.0.1:9090 --secret your-secret
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
