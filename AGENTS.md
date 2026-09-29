# Repository Guidelines

## Project Structure

Dual-repo: a Go Bubble Tea TUI plus a Bun npm workspace that only distributes prebuilt binaries.

- Go code: `cmd/tui/main.go` (CLI entrypoint, subcommands, `var version` set via ldflags); `internal/app` (state + update loop), `internal/api` (controller HTTP client), `internal/compat` (normalization), `internal/profile` (persistence), `internal/view` (rendering). Keep new code in `internal/<area>`.
- `packages/` is generated distribution scaffolding: `cli` and `mihomo-tui` launcher packages plus six platform packages. npm scope is `@qinshower/*` even though the Go module is `github.com/metacubex/mihomo-tui`.
- `scripts/lib/platforms.mjs` is the single source of truth for platform IDs, package names, and binary names; release scripts all import it.
- `bin/`, `dist/`, and `packages/*/bin/*` are build outputs (gitignored). Never commit binaries.

## Commands

- `go build -o ./bin/mihomo-tui ./cmd/tui` — local binary.
- `go test ./...` — full suite (unit + httptest-backed controller e2e).
- `go test ./internal/app -run TestPaneSwitch` — one focused test.
- `bun install` — required before any `bun run` script; Bun is the package manager (no npm/yarn lockfile).
- `./bin/mihomo-tui open --controller http://127.0.0.1:9090 --secret xxx` — run against a controller; also `--unix-socket /path`.
- `./bin/mihomo-tui profile add --name local --controller http://127.0.0.1:9090 --default` — reusable profile; then `open --profile local`.

## Release Pipeline

Order matters; each step consumes the previous one's output:

1. `bun run prepare:release <version>` — rewrites `version` across root and all `packages/*/package.json` (leaves uncommitted changes).
2. `bun run build:binaries --version <version>` — cross-compiles all six platforms with `CGO_ENABLED=0` and `-ldflags "-X main.version=..."` into `dist/npm/`.
3. `bun run build:packages` — copies `dist/npm` binaries into `packages/*/bin/`; fails if step 2 was skipped.
4. `bun run test:launchers` — smoke tests install/launcher behavior on the host platform; requires steps 2–3.
5. `bun run publish:packages --version <version>` — publishes to npm; dist-tag defaults to the prerelease suffix (`v0.3.0-beta.1` → `beta`) or `latest`; `--tag next` / `--dry-run` override.

CI (`.github/workflows/release.yml`) runs `go test ./...` first, then this chain, then attaches tar/zip archives to a GitHub release. Triggers on `v*` tags or manual dispatch. There is no PR CI — run tests locally.

## Coding Style

Standard Go: `gofmt`, tabs, grouped imports, short receivers, explicit error returns. CLI flags and user-facing strings use `kebab-case` plain wording. Package-focused files over large ones.

Commits follow Conventional Commits with a scope, matching history: `fix(app): preserve route when auto group is down`, `build(npm): ...`, `ci(release): ...`.

## Testing

Tests live beside code as `*_test.go`; e2e tests spin up mock controllers with `httptest`. Cover state transitions, API request shapes, compatibility parsing, and profile isolation. Prefer table-driven tests. New behavior ships with tests; run `go test ./...` before committing.

## Configuration

`MIHOMO_TUI_CONFIG` overrides the profile storage path. `internal/app` tests clear it in `TestMain` — never depend on a developer-local config path in test code.
