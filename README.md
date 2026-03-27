# debrief

[![CI](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudprobe/debrief/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/cloudprobe/debrief)](https://github.com/cloudprobe/debrief/releases/latest) [![License](https://img.shields.io/github/license/cloudprobe/debrief)](LICENSE) [![Go Version](https://img.shields.io/github/go-mod/go-version/cloudprobe/debrief)](go.mod) [![Coverage](https://codecov.io/gh/cloudprobe/debrief/branch/main/graph/badge.svg)](https://codecov.io/gh/cloudprobe/debrief)

Know what you actually did today — git commits, AI sessions, one command.

## Install

```sh
brew install cloudprobe/tap/debrief
```

Or build from source:

```sh
go install github.com/cloudprobe/debrief/cmd/debrief@latest
```

## Usage

```sh
debrief                    # today's activity
debrief yesterday          # yesterday
debrief week               # this week (per-day breakdown)
debrief month              # this month
debrief -d 2026-03-25      # specific date
debrief -f 2026-03-20 -t 2026-03-25  # date range

debrief standup            # copy-paste standup bullets
debrief standup week       # standup for the whole week

debrief -c                 # cost view (today)
debrief -c week            # weekly cost with per-model breakdown
debrief -c month           # monthly cost with per-model breakdown

debrief --detail           # show per-session detail
debrief --no-git           # skip git, only show AI sessions
debrief -o json            # JSON output
debrief -o markdown        # markdown for PRs/wikis
debrief -v                 # verbose/debug output
```

## What it does

Reads your local Claude Code session files and git history. No API calls, no network access — everything is local.

**Default view** — what you actually did:
```
  Your day — Wednesday, Mar 26 2026
  ──────────────────────────────────────────────────────

  cloudprobe/debrief
    Built out new code with Claude
    ~4h 37m active
    Created 10 files, updated 14 files — main.go, text.go, aggregator.go +13 more
    Committed: "rename to debrief", "add message dedup" +3 more
    +340 -89 lines

  cloudprobe/dotfiles
    Made updates with Claude
    ~20m active
    updated 2 files — .zshrc, .gitconfig

  ──────────────────────────────────────────────────────
  2 repos · 26 files changed · 5 commits · +340 -89 lines · 2 deep sessions
```

**Standup** — plain text bullets for your team:
```
Mar 26 2026:
• Built out cloudprobe/debrief — 10 new files (main.go, text.go, aggregator.go, model.go)
• Minor work on cloudprobe/dotfiles
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

## Configuration

Optional config file at `~/.config/debrief/config.yaml`:

```yaml
git_repo_paths:
  - ~/work
  - ~/projects
  - ~/code

default_format: text  # text, json, standup, markdown
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--date` | `-d` | Specific date (YYYY-MM-DD) |
| `--from` | `-f` | Start date for range |
| `--to` | `-t` | End date for range |
| `--cost` | `-c` | Show billing view |
| `--format` | `-o` | Output format: text, json, standup, markdown |
| `--verbose` | `-v` | Debug output on stderr |
| `--detail` | | Show per-session detail |
| `--no-git` | | Skip git collection |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
