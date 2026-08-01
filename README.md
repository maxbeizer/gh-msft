# gh-msft

> [!WARNING]
> Currently a spike. Not sure if this will be worth it. API subject to change

A [GitHub CLI](https://cli.github.com/) extension to read and triage your
Microsoft 365 **mail and calendar** from the terminal.

## How it works (auth)

`gh-msft` performs **no authentication of its own** and stores **no credentials**.
It rides [WorkIQ](https://github.com/microsoft/work-iq): a user-local broker keeps
one WorkIQ MCP server (`workiq mcp`) warm and calls its deterministic, non-LLM
Microsoft Graph proxy (`fetch` / `do_action`). WorkIQ — backed by your operating
system's Microsoft SSO broker — owns authentication for your signed-in Microsoft
365 account. If WorkIQ can read your mail, so can `gh-msft`.

### Prerequisites

- [Node.js](https://nodejs.org/) (provides `npx`).
- WorkIQ set up and consented once. Accept the EULA with `gh msft accept-eula`
  (or `npx -y @microsoft/workiq accept-eula`); admin consent may be required on
  your tenant — see the WorkIQ docs.

By default the broker launches `npx -y @microsoft/workiq@latest mcp`. Override with:

- `WORKIQ_MCP_COMMAND` — full command line (e.g. `WORKIQ_MCP_COMMAND="workiq mcp"`).
- `WORKIQ_BIN` — path to a `workiq` binary (`<bin> mcp` is used).
- `WORKIQ_DIRECT_PROCESS=1` — bypass the broker and launch a private WorkIQ process
  for this command. Use this for diagnosis or when a local broker cannot start.

### WorkIQ broker lifecycle

The first mail or calendar command starts a broker in your user cache directory,
then later invocations reuse its authenticated WorkIQ connection. The broker listens
only on `127.0.0.1`, requires a random token stored in a user-only state file, and
stores no Microsoft credentials. It shuts down on termination signals; the next
command automatically replaces stale state, unavailable endpoints, or an older
broker protocol. On an interactive terminal, concise progress messages rotate while
WorkIQ starts and while inbox or calendar data loads. Scripted commands and JSON
output remain free of progress text.

To compare cold and warm startup on your machine:

```bash
time gh msft mail list --top 1 >/dev/null  # cold broker + WorkIQ startup
time gh msft mail list --top 1 >/dev/null  # warm broker
time gh msft cal --top 1 >/dev/null        # warm broker
```

If the broker repeatedly fails, run one command with
`WORKIQ_DIRECT_PROCESS=1` to distinguish a WorkIQ problem from a broker problem.
Remove the `gh-msft` directory under your system user cache directory only after
all `gh msft` commands have exited; the next invocation recreates it.

## Installation

```bash
gh extension install maxbeizer/gh-msft
```

## Usage

```bash
gh msft                           # interactive inbox and calendar
gh msft mail list                 # recent inbox messages
gh msft mail list --all           # include all mail, not just the inbox
gh msft mail list --top 50 --json # machine-readable output
gh msft mail archive <id>         # move a message to Archive
gh msft mail list --json | jq -r '.[].id' | gh msft mail archive --stdin
gh msft cal                       # upcoming calendar events
gh msft cal --json                # machine-readable output
gh msft accept-eula               # accept the WorkIQ EULA (run once)
gh msft tui                       # interactive inbox and calendar
gh msft tui --cal                 # start in calendar mode
```

### Interactive inbox (`gh msft` or `gh msft tui`)

The TUI has two modes: mail (inbox) and calendar (upcoming events). Press
`tab` to switch, or start in calendar mode with `--cal`.

Its app chrome, panels, headers, metadata, status, help, loading, and error
states use one visual system. The active tab is bracketed, the current row has
an `>` marker, and unread messages include a `NEW` label, so state stays clear
without color. Colors adapt to light or dark terminals and Lip Gloss falls back
to the same readable text hierarchy on terminals without color support. Narrow
terminals use compact panels and two-line inbox rows to preserve message context.

| Key        | Action                          |
| ---------- | ------------------------------- |
| `j` / `k`  | move down / up (also arrows)    |
| `g` / `G`  | jump to top / bottom            |
| `tab`      | switch mail / calendar          |
| `a`        | archive selected message (mail) |
| `enter`    | open selected message           |
| `esc`      | close open message              |
| `r`        | toggle read state (visual only) |
| `R`        | refresh current view            |
| `?`        | toggle help                     |
| `q`        | quit                            |

## Why this over WorkIQ directly?

WorkIQ's chat answers questions about your mail. `gh-msft` adds what a chat can't:
a fast **interactive TUI inbox**, **deterministic scriptable commands** (pipe to
`jq`, chain with other `gh` commands), and **no LLM latency** — the Graph proxy is
called directly.

## Development

```bash
make help          # see all targets
make build         # build binary
make test          # run tests
make ci            # build + vet + test-race
make install-local # install extension from checkout
make relink-local  # reinstall after changes
```

Tests use fakes/in-process pipes and **never launch WorkIQ or hit the network**.

### Layout

| Path                | Purpose                                             |
| ------------------- | --------------------------------------------------- |
| `internal/workiq`   | MCP stdio client for the WorkIQ Graph proxy         |
| `internal/mail`     | `MailProvider` + WorkIQ-backed implementation       |
| `internal/calendar` | `CalendarProvider` + WorkIQ-backed implementation   |
| `internal/cli`      | cobra command tree and output formatting            |
| `internal/tui`      | Bubble Tea interactive inbox                        |
| `internal/mstime`   | Microsoft Graph date/time parsing                   |

## Releasing

Tag a version to trigger a release build:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## What's included

| File | Purpose |
|------|---------|
| `Makefile` | Build, test, lint, install targets |
| `.goreleaser.yml` | Cross-platform binary releases |
| `.github/workflows/release.yml` | Automated releases on tag push |
| `.github/workflows/ci.yml` | CI on push/PR to main |
| `main.go` | Entry point (cobra + signal handling) |
| `AGENTS.md` | Guidance for AI coding agents |
