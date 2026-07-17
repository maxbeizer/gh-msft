# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org).

## [Unreleased]

### Added

- Initial release
- WorkIQ-backed Microsoft 365 access (no credentials stored by this tool):
  - `internal/workiq` MCP stdio client for the WorkIQ Graph proxy (`fetch` / `do_action`).
  - `MailProvider` and `CalendarProvider` interfaces with WorkIQ-backed implementations.
- Commands: `gh msft mail list [--top N] [--json]`, `gh msft mail archive <id...> [--stdin]`,
  `gh msft cal [--top N] [--json]`.
- Interactive inbox TUI (`gh msft tui`) built on Bubble Tea (navigate, archive, refresh).
