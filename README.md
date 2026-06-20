# mihomo-tui

`mihomo-tui` is a standalone terminal UI for Clash/Mihomo-compatible
`external-controller` APIs.

## Features

- multi-session controller switching
- mode switching: `rule`, `global`, `direct`
- proxy group browsing
- node switching
- delay testing
- resize-safe, lazygit-inspired terminal layout

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
