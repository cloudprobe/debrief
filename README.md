# debrief

[![CI](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml)
[![Coverage](https://coveralls.io/repos/github/cloudprobe/debrief/badge.svg?branch=main)](https://coveralls.io/github/cloudprobe/debrief?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/cloudprobe/debrief)](https://goreportcard.com/report/github.com/cloudprobe/debrief)
[![Latest Release](https://img.shields.io/github/v/release/cloudprobe/debrief)](https://github.com/cloudprobe/debrief/releases/latest)
[![License: MIT](https://img.shields.io/github/license/cloudprobe/debrief)](LICENSE)

Know what you actually did today -- git commits, AI sessions, one command.

## Install

```sh
brew install cloudprobe/tap/debrief
```

Or with Go:

```sh
go install github.com/cloudprobe/debrief/cmd/debrief@latest
```

## Usage

```sh
debrief standup            # today's standup
debrief standup yesterday  # yesterday
debrief standup week       # this week (per-day breakdown)
debrief standup -d 2026-03-25  # specific date

debrief cost               # today's estimated API cost
debrief cost week          # this week with per-model breakdown

debrief standup --project myapp  # filter to one project
debrief cost --project myapp     # same filter on cost
```

## Setup

```sh
debrief init
```

Choose your Claude Code access type (direct API, Max/Pro, Vertex, Bedrock). Config is saved to `~/.config/debrief/config.yaml`.

## Example output

```
Week of Mar 31 -- Apr 6, 2026
──────────────────────────────

Mon, Mar 31 2026:

cloudprobe/debrief
  * Redesigned cost table with per-model breakdown and subtotals
  * Added --project filter to standup and cost commands
  PRs: https://github.com/cloudprobe/debrief/pull/12

cloudprobe/dotfiles
  * Updated zshrc aliases for new debrief commands

Wed, Apr 2 2026:

cloudprobe/debrief
  * Fixed empty day suppression in week output
  * Removed deprecated --from/--to flags

────────────────────────────────────────
2 projects * 8 commits * active 2 of 7 days
```
