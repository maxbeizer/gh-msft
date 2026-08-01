# gh-msft

A [GitHub CLI](https://cli.github.com/) extension to read and triage your
Microsoft 365 **mail and calendar** from the terminal.

> [!NOTE]
> Experimental, useful, and evolving quickly. Commands and output shapes documented
> below are the supported surface; the underlying WorkIQ integration may change.

Your Microsoft 365 inbox and calendar, where `gh` belongs: keyboard-first when
you are at a terminal, deterministic when you need a script.

## Start here

```bash
gh extension install maxbeizer/gh-msft
gh msft accept-eula  # run once to accept the WorkIQ EULA
gh msft              # open the interactive inbox and calendar
```

The first command that reads Microsoft 365 may take a few seconds while WorkIQ
starts. Later commands reuse the local connection.

Here is a deliberately synthetic preview of the TUI--no real message or
calendar data is used:

```text
gh msft   [Mail]  Calendar
╭─ Inbox · 3 messages · Inbox ──────────────────────────╮
│ > NEW  Avery Chen          Project update  Aug 01 10:42 │
│        Contoso Newsletter  August news    Aug 01 09:15 │
│        Morgan Lee          Lunch next week? Jul 31 16:08 │
│                                                         │
│ Help: j/k move · enter open · tab switch · ? help · q quit │
╰─────────────────────────────────────────────────────────╯
```

## Why this instead of WorkIQ chat?

| WorkIQ chat | `gh-msft` |
| --- | --- |
| Ask natural-language questions about Microsoft 365 | Browse mail and upcoming events in a focused terminal UI |
| Conversational answers | Deterministic, scriptable commands for pipes and automation |
| Uses LLM-backed chat | Calls the Microsoft Graph proxy directly, without LLM latency |

## Setup and authentication

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

<details>
<summary>How the local WorkIQ broker works</summary>

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

</details>

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
gh msft mail view <id>            # message metadata and plain-text body
gh msft mail view <id> --json     # machine-readable message detail
gh msft mail archive <id>         # move a message to Archive
gh msft mail list --json | jq -r '.[].id' | gh msft mail archive --stdin
gh msft mail list --json | jq -r '.[0].id' | xargs gh msft mail view --json
gh msft mail list --json | jq -r '.[].id' | gh msft mail archive --stdin --json
gh msft cal                       # upcoming calendar events
gh msft cal --json                # machine-readable output
gh msft accept-eula               # accept the WorkIQ EULA (run once)
gh msft tui                       # interactive inbox and calendar
gh msft tui --cal                 # start in calendar mode
```

### Recipes

```bash
# Read the next three calendar events as JSON.
gh msft cal --top 3 --json | jq '.[] | {subject, start, organizer}'

# Find the most recent message ID, then inspect its plain-text body.
gh msft mail list --top 1 --json | jq -r '.[0].id' | xargs gh msft mail view

# Archive every message selected by an external filter.
gh msft mail list --json | jq -r '.[] | select(.from.email | endswith("@news.example")) | .id' \
  | gh msft mail archive --stdin
```

### Machine-readable output

Commands with `--json` write only the documented JSON value to stdout. Progress
and diagnostics are written to stderr; failures exit nonzero. The following
payload shapes are stable:

| Command | JSON payload |
| --- | --- |
| `mail list --json` | Array of message objects (backward compatible) |
| `cal --json` | Array of event objects (backward compatible) |
| `mail archive <id...> --json` | `{"archived":["<id>", ...]}` |

`mail archive --json` writes its success result only after every requested ID is
archived. If one ID fails after earlier IDs succeed, it exits nonzero without a
success JSON value and reports the completed and failed IDs in its stderr
diagnostic.

### Interactive inbox (`gh msft` or `gh msft tui`)

The TUI has two modes: mail (inbox) and calendar (upcoming events). Press
`tab` to switch, or start in calendar mode with `--cal`.

Its app chrome, panels, headers, metadata, status, help, loading, and error
states use one visual system. The active tab is bracketed, the current row has
an `>` marker, and unread messages include a `NEW` label, so state stays clear
without color. Colors adapt to light or dark terminals and Lip Gloss falls back
to the same readable text hierarchy on terminals without color support. Narrow
terminals use compact panels and two-line inbox rows to preserve message context.
On wider terminals, list panels use the available width so longer calendar and
message details remain easy to scan. Lists stay within the terminal height, and
`j` / `k` keeps the selected row visible while navigating.

| Key                    | Action                                      |
| ---------------------- | ------------------------------------------- |
| `j` / `k` or `↑` / `↓` | move down / up in mail and calendar lists   |
| `g` / `G` or home/end  | jump to top / bottom of a list or message   |
| `tab`                  | switch mail / calendar                      |
| `a`                    | archive selected message (mail only)        |
| `enter`                | open selected mail or calendar event; close detail |
| `esc` / `?`            | dismiss / toggle expanded help               |
| `r`                    | toggle read state (visual only; mail only)  |
| `R`                    | refresh current view or retry after an error |
| `q`                    | quit, or close an open message               |

Open messages use `j` / `k`, arrow keys, `g`, and `G` to scroll their full
contents. Open calendar events show their description, participants, location,
and available links. Press `j` to join through `onlineMeeting.joinUrl` (the
preferred meeting link) and `o` to open the Outlook event when its `webLink` is
available. Missing links and browser-launch failures remain visible in the event
detail view. Detail footers, list footers, and expanded help list only bindings
that apply to the active view.

Mail rows show selection, unread state, sender, subject, and received time. The
header identifies whether the TUI is showing Inbox or all mail, and list columns
compact intentionally on narrow terminals. Calendar events are grouped by start
day; all-day events, same-day meetings, and multi-day events use distinct time
labels so upcoming commitments remain easy to scan.

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
