# AGENTS.md

Guidance for AI coding agents working in this repository. Human contributors may
also find it useful. This file complements
[`.github/copilot-instructions.md`](.github/copilot-instructions.md).

## What this project is

`gh-msft` is a [GitHub CLI](https://cli.github.com/) extension written in Go. It
follows the conventions of the
[gh-extension-template](https://github.com/maxbeizer/gh-extension-template):
a `main` package at the repo root using [cobra](https://github.com/spf13/cobra)
for command handling and signal-aware context cancellation.

## Project layout

| Path | Purpose |
| --- | --- |
| `main.go` | Entry point; root command wiring + signal handling |
| `go.mod` / `go.sum` | Module definition (`github.com/maxbeizer/gh-msft`) and checksums |
| `Makefile` | Canonical build/test/lint/install commands |
| `.goreleaser.yml` | Cross-platform release build config |
| `.github/workflows/` | CI (`ci.yml`) and release (`release.yml`) automation |

## Environment

- Go `1.24.13+` is required (`make check-go-version` enforces the minimum).
- The `gh` CLI must be installed and authenticated for extension install/run.

## Build, test, and validate

Prefer the `Makefile` targets — CI runs `make ci`, so match it locally:

```bash
make build   # go build -o bin/gh-msft .
make test    # go test ./...
make ci      # go build ./... && go vet ./... && go test -race
make fmt     # go fmt ./...
make tidy    # go mod tidy
```

Always run `make ci` before proposing changes. If you add or change
dependencies, run `make tidy` and commit the updated `go.mod`/`go.sum`.

## Local install

```bash
make install-local   # gh extension install .
make relink-local    # rebuild + reinstall after code changes
gh msft              # run the extension
```

## Conventions for agents

- Keep `main` package code at the repo root; put reusable logic in subpackages
  organized by functionality.
- Use only the Go standard library and existing dependencies (`cobra`) unless a
  new dependency is clearly justified. Add new deps via `go get` + `make tidy`.
- Write table-driven tests (`*_test.go`) in the same package; cover argument
  parsing and core logic. See `.github/copilot-instructions.md` for the pattern.
- Handle errors explicitly and return them as the last value. Provide clear,
  user-facing error messages; use exit code `0` for success, `1` for errors.
- Format with `gofmt` and keep functions small and focused.
- Preserve signal handling and `context` cancellation in `main.go` when editing.

## When changing the extension's public behavior

- Update `README.md` usage docs.
- Add an entry under `## [Unreleased]` in `CHANGELOG.md`.
- Keep `main.go`'s `Use` and `Short` fields accurate.

## Releasing

Releases are tag-driven. Tagging `vX.Y.Z` triggers `.github/workflows/release.yml`,
which runs goreleaser to publish binaries. Do not hand-edit release artifacts.

```bash
git tag v0.1.0
git push origin v0.1.0
```
