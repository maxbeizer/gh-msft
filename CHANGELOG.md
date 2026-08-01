# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com),
and this project adheres to [Semantic Versioning](https://semver.org).

## [Unreleased]

## [0.4.0] - 2026-08-01

### Added

- Calendar events in the interactive TUI now open into a scrollable detail view
  with description, organizer, attendees, location, and explicit meeting and
  Outlook link availability. `j` opens the supported
  `onlineMeeting.joinUrl`; `o` opens the event `webLink`.
- `gh msft mail view <message-id>` displays a message's metadata and safe
  plain-text body, with `--json` for clean scriptable output.
- A cohesive, adaptive visual design system for the interactive TUI, including
  consistent app chrome, panels, headers, selections, unread indicators,
  metadata, status, help, loading, and failure states. Narrow and no-color
  terminals retain textual state markers and readable layouts.
- Running `gh msft` without a subcommand now launches the interactive inbox and
  calendar TUI, matching `gh msft tui`.
- Interactive WorkIQ startup and inbox/calendar loading now use deterministic,
  rotating progress messages while leaving scripted and JSON output clean.
- Vim-style `j`/`k` and `g`/`G` navigation is now consistent across mail,
  calendar, and scrollable message details, while preserving arrow-key and
  home/end alternatives. Contextual footers and expanded help now reflect the
  active mode, including loading and error recovery controls.
- `mail archive --json` now emits a stable `{"archived":[...]}` action result
  after every requested message is archived. Existing `mail list --json` and
  `cal --json` array payloads remain unchanged.
- Redesigned TUI inbox and calendar lists for scanability: mail rows now show
  sender, subject, received time, unread state, and selection; calendar events
  are grouped by day with clear all-day, same-day, and multi-day labels.
- TUI panels now use the available terminal width instead of shrinking to the
  longest rendered row.
- TUI inbox and calendar lists now stay within the terminal height while keeping
  the selected row visible during navigation.

## [0.3.0] - 2026-08-01

### Added

- A user-scoped local WorkIQ broker now keeps one authenticated MCP connection warm
  across `gh msft` commands, with token-protected loopback transport, stale-state
  recovery, and `WORKIQ_DIRECT_PROCESS=1` for direct-process diagnosis or fallback.
- WorkIQ startup progress now explains that the initial startup can take a few
  seconds and later commands reuse the local broker.

## [0.2.0] - 2026-07-27

### Added

- `gh msft accept-eula` command to accept the WorkIQ End User License Agreement
  from the extension. WorkIQ tool errors caused by an unaccepted EULA now hint at
  running it.
- `gh msft mail list --all` and `gh msft tui --mail-all` flags to load all mail;
  mail commands and TUI mail mode now default to Inbox-only.
- `enter` in the inbox TUI opens the selected message in a detail view showing the
  subject, received time, full `From`/`To` addresses, and the message body (HTML mail
  is converted to plain text). `esc`/`enter` returns to the list. The provider gains a
  `Body(ctx, id)` method and `ListInbox` now selects `toRecipients`.
- Calendar mode in the TUI: press `tab` to switch between mail and calendar, or
  start in calendar mode with `gh msft tui --cal`.
- All-day and multi-day calendar events render clearly: all-day events show the
  date plus `all day` (no `00:00`), and events that span days show the end date.

### Fixed

- TUI inbox now adapts email subject width to the terminal on resize, showing
  more or less of the title, and clears the screen on resize to avoid artifacts.
- `make install-local` and `make relink-local` now work from worktree checkout
  paths whose basename does not start with `gh-`.

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
