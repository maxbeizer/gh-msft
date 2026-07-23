# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org).

## [Unreleased]

## [0.1.0] - 2026-07-23

### Added

- WorkIQ-backed Microsoft 365 access (no credentials stored by this tool):
  - `internal/workiq` MCP stdio client for the WorkIQ Graph proxy (`fetch` / `do_action`).
  - `MailProvider` and `CalendarProvider` interfaces with WorkIQ-backed implementations.
- Commands: `gh msft mail list [--top N] [--json]`, `gh msft mail archive <id...> [--stdin]`,
  `gh msft cal [--top N] [--json]`.
- Interactive inbox TUI (`gh msft tui`) built on Bubble Tea (navigate, archive, refresh).
- Startup progress spinner on stderr while WorkIQ launches and data loads, so a
  cold start no longer looks like a hang. Shown only on an interactive terminal;
  piped/`--json` output on stdout stays clean.

### Fixed

- Read WorkIQ tool results from `structuredContent` (where WorkIQ actually returns
  `fetch`/`do_action` payloads) in addition to `content[].text`. Previously every live
  call failed with `decode fetch envelope: unexpected end of JSON input`.
- Bound the WorkIQ startup handshake with a timeout (default 60s, override via
  `WORKIQ_STARTUP_TIMEOUT`) so a cold `npx` launch fails fast with a clear message
  instead of hanging indefinitely.
- Send WorkIQ's stderr to the null device instead of `io.Discard`, and kill the
  WorkIQ **process group** on close. Previously `Close` hung after every command
  (the caller had to Ctrl-C) because `Wait` blocked on a stderr copy goroutine that
  WorkIQ's grandchildren kept open; orphaned WorkIQ processes (and their popup
  windows) also lingered. Close now terminates the whole tree and never blocks.
