# Repository Guidelines

## Project Structure & Module Organization

`cmd/tui/main.go` holds CLI entrypoint and subcommand wiring. Core app state and Bubble Tea update logic live in `internal/app`. Mihomo controller HTTP client code sits in `internal/api`, compatibility normalization in `internal/compat`, profile persistence in `internal/profile`, and terminal rendering helpers in `internal/view`. Build output goes to `bin/`. Keep new code inside `internal/<area>` unless it must be public.

## Build, Test, and Development Commands

- `go build -o ./bin/mihomo-tui ./cmd/tui` builds local binary.
- `go test ./...` runs full unit and end-to-end test suite.
- `go test ./... -run TestPaneSwitch` runs one focused test while iterating.
- `./bin/mihomo-tui open --controller http://127.0.0.1:9090 --secret xxx` starts TUI against direct controller.
- `./bin/mihomo-tui profile add --name local --controller http://127.0.0.1:9090 --default` creates reusable local profile.

## Coding Style & Naming Conventions

Use standard Go formatting: tabs, `gofmt`, grouped imports, short receiver names. Prefer small package-focused files and explicit error returns over hidden control flow. Exported names use `CamelCase`; unexported helpers use `camelCase`; tests use `TestXxx` with behavior-oriented names such as `TestReconnectStateRecovery`. Keep CLI flags and user-facing strings consistent with existing `kebab-case` and plain wording.

## Testing Guidelines

Tests live beside implementation files as `*_test.go`. Current suite mixes unit tests with controller-backed end-to-end tests via `httptest`. Cover state transitions, API request shapes, compatibility parsing, and profile isolation. Add table-driven tests when multiple input cases share one behavior. Run `go test ./...` before opening PR; new behavior should ship with new or updated tests.

## Commit & Pull Request Guidelines

History is minimal, so keep commits short, imperative, and specific, for example `app: preserve active pane on reload`. Separate refactors from behavior changes when possible. PRs should explain user-visible effect, touched packages, and test coverage. Include terminal screenshots or short recordings for layout or interaction changes.

## Configuration Notes

Profile storage path can be controlled through `MIHOMO_TUI_CONFIG`. Tests clear this variable in `TestMain`; avoid depending on a developer-local config path in test code.
