# debrief

[![CI](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml)
[![Coverage](https://coveralls.io/repos/github/cloudprobe/debrief/badge.svg?branch=main&t=1)](https://coveralls.io/github/cloudprobe/debrief?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/cloudprobe/debrief)](https://goreportcard.com/report/github.com/cloudprobe/debrief)
[![Latest Release](https://img.shields.io/github/v/release/cloudprobe/debrief)](https://github.com/cloudprobe/debrief/releases/latest)
[![License: MIT](https://img.shields.io/github/license/cloudprobe/debrief)](LICENSE)

Know what you actually shipped today.

debrief reads your local git history and Claude Code sessions — no API calls, no logins, no cloud — and turns them into a standup summary or cost report in seconds.

```
$ debrief standup week

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
2 projects · 8 commits · active 2 of 7 days
```

- **No API calls** — reads `~/.claude/projects/` and git log locally, nothing leaves your machine
- **Smart synthesis** — surfaces Claude's own session notes alongside signal commits, filters out chore/lint noise
- **PR-aware** — automatically extracts GitHub PR links and GitLab MR references from commits
- **Multi-source pricing** — supports direct API, Max/Pro, Vertex, and Bedrock with per-model cost breakdown

## Install

```sh
brew install cloudprobe/tap/debrief
```

Or with Go:

```sh
go install github.com/cloudprobe/debrief/cmd/debrief@latest
```

## Setup

```sh
debrief init
```

Choose your Claude Code access type (direct API, Max/Pro, Vertex, Bedrock). Config is saved to `~/.config/debrief/config.yaml`.

## Commands

| Command | Args | Description |
|---------|------|-------------|
| `debrief init` | — | Interactive setup wizard |
| `debrief standup` | `today` `yesterday` `week` `month` `-d YYYY-MM-DD` | Standup summary from commits + AI sessions |
| `debrief cost` | `today` `yesterday` `week` `month` `-d YYYY-MM-DD` | Estimated API cost with per-model breakdown |
| `debrief version` | — | Print version |

**Flags:**

| Flag | Commands | Description |
|------|----------|-------------|
| `--project`, `-p` | `standup`, `cost` | Filter to repos matching substring |
| `--by-project` | `standup` | Group bullets under project name headers |
| `--date`, `-d` | all | Override date (YYYY-MM-DD) |

## Configuration

`~/.config/debrief/config.yaml` (respects `$XDG_CONFIG_HOME`):

```yaml
git_repo_paths:
  - ~/work
  - ~/projects
  - ~/code
git_discovery_depth: 2        # how deep to scan for git repos

pricing:
  preset: direct              # direct | max | vertex | bedrock
  overrides:
    claude-opus-4-5:
      input_per_million: 15.0
      output_per_million: 75.0

# optional overrides for AI session paths
claude_dir: ~/.claude/projects
codex_dir: ~/.codex/sessions
gemini_dir: ~/.gemini/tmp
```
