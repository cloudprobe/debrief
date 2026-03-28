# debrief

[![CI](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/cloudprobe/debrief)](https://github.com/cloudprobe/debrief/releases/latest) [![License](https://img.shields.io/github/license/cloudprobe/debrief)](LICENSE) [![Go Version](https://img.shields.io/github/go-mod/go-version/cloudprobe/debrief)](go.mod) [![Coverage](https://coveralls.io/repos/github/cloudprobe/debrief/badge.svg?branch=main)](https://coveralls.io/github/cloudprobe/debrief)

Know what you actually did today — git commits, AI sessions, one command.

## Install

```sh
brew install cloudprobe/tap/debrief
```

Or build from source:

```sh
go install github.com/cloudprobe/debrief/cmd/debrief@latest
```

## Quick start

```sh
debrief init       # first-time setup (choose your Claude access type)
debrief standup    # today's standup summary
debrief cost       # today's estimated API cost
```

## Usage

### standup — copy-paste standup bullets

```sh
debrief standup            # today
debrief standup yesterday  # yesterday
debrief standup week       # this week (per-day breakdown)

debrief standup -d 2026-03-25              # specific date
debrief standup -f 2026-03-20 -t 2026-03-25  # date range
```

### cost — estimated API cost

```sh
debrief cost           # today
debrief cost yesterday # yesterday
debrief cost week      # this week with per-model breakdown
debrief cost month     # this month with per-model breakdown
```

> **Note:** `debrief cost` requires running `debrief init` first to set your access type.
> If you're on a Max/Pro subscription, per-token costs don't apply — `debrief standup` is what you want.

### Other commands

```sh
debrief init     # first-time setup: configure your Claude access type
debrief version  # print the current version
```

## What it does

Reads your local Claude Code session files and git history. No API calls, no network access — everything is local.

**Standup** — plain text bullets for your team:

```
Mar 26 2026:

cloudprobe/debrief
  Built out new collector and added message dedup
  • rename to debrief
  • add message dedup
  • fix aggregator edge case

cloudprobe/dotfiles
  Updated zshrc and gitconfig

Minor: cloudprobe/helm-charts
```

**Cost** — billing view with per-model breakdown:

```
  Cost — Wednesday, Mar 26 2026
  ──────────────────────────────────────────────────────

  cloudprobe/debrief                        $32.07
    opus 4.6                   $31.88
    haiku 4.5                  $0.19

  ──────────────────────────────────────────────────────
  Today: $32.07 · This week: $102.89 · This month: $102.90

  Week by model:
    opus 4.6                   $84.85
    sonnet 4.6                 $16.89
    haiku 4.5                  $1.14
```

## Global flags

These flags work with any command:

| Flag | Short | Description |
|------|-------|-------------|
| `--date` | `-d` | Specific date (YYYY-MM-DD) |
| `--from` | `-f` | Start date for range (YYYY-MM-DD) |
| `--to` | `-t` | End date for range (YYYY-MM-DD) |
| `--version` | | Print version and exit |

## Configuration

Optional config file at `~/.config/debrief/config.yaml` (respects `$XDG_CONFIG_HOME`).

Run `debrief init` to create the initial config interactively.

```yaml
# Directories to scan for git repositories (default: ~/work, ~/projects, ~/code).
# If a configured path doesn't exist, debrief prints a warning to stderr.
git_repo_paths:
  - ~/work
  - ~/projects

# How many directory levels deep to scan for git repos (default: 2).
git_discovery_depth: 2

# Override the default ~/.claude/projects/ path for Claude Code session files.
# claude_dir: /custom/path

# Pricing preset: "direct" (Anthropic API), "max" (Max/Pro subscription),
# "vertex" (Google Vertex AI), or "bedrock" (AWS Bedrock).
# Set interactively via `debrief init`.
pricing:
  preset: direct

  # Optional per-model rate overrides (USD per 1M tokens).
  # overrides:
  #   my-custom-model:
  #     input_per_million: 3.00
  #     output_per_million: 15.00
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
