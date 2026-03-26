# devrecap

Your standup doesn't know about the 3 hours you spent in Claude Code. **devrecap** does.

A CLI tool that parses AI coding session data locally to show what you actually did today. No cloud, no API calls, no telemetry. Just reads the logs already on your machine.

```
$ devrecap today

  devrecap — Wednesday, Mar 25 2026
  ──────────────────────────────────────────────────────

  devrecap
    55 interactions · 138.0K tokens · opus 4.6
    12 files created, 1 file modified — claude.go, model.go, aggregator.go, text.go +9 more

  claude
    42 interactions · 184.8K tokens · opus 4.6

  dotfiles
    3 interactions · 659 tokens · opus 4.6

  ──────────────────────────────────────────────────────
  100 interactions · 323.5K tokens
```

```
$ devrecap standup

**Mar 25:**
• Built out **devrecap** — created 12 new files (claude.go, model.go, aggregator.go, text.go +8 more)
• Researched and planned in **claude** (42 AI interactions)
• Minor work on dotfiles, cloudprobe
```

## Why

Developers spend hours daily in AI coding tools — Claude Code, Codex CLI, Gemini CLI — but none of that shows up in traditional activity trackers. `git log` only captures commits. WakaTime only sees editor keystrokes. Neither knows you spent three hours debugging with Claude.

devrecap reads the session logs these tools already write to disk and produces a complete picture of your day.

## Install

```bash
# Homebrew
brew install cloudprobe/tap/devrecap

# From source
go install github.com/cloudprobe/devrecap/cmd/devrecap@latest

# Or clone and build
git clone https://github.com/cloudprobe/devrecap.git
cd devrecap
make install
```

## Usage

```bash
devrecap today                    # Today's activity
devrecap yesterday                # Yesterday's activity
devrecap week                     # This week's rollup
devrecap standup                  # Copy-paste standup (yesterday)
devrecap today --format standup   # Standup for today
devrecap today --format json      # JSON output for scripting
devrecap today --cost             # Include estimated API costs
devrecap today --date 2026-03-20  # Specific date
```

## What it tracks

**Currently supported:**
- **Claude Code** — sessions, tokens, models used, files created/modified, tool usage

**Planned:**
- Codex CLI (OpenAI)
- Gemini CLI (Google)
- Git commit activity

## How it works

1. Reads JSONL session files from `~/.claude/projects/`
2. Parses assistant messages for token usage, model, and tool calls
3. Groups activity by project (based on working directory)
4. Aggregates and renders a summary

All data stays on your machine. devrecap makes zero network calls.

### What gets tracked

| Data | Source | Purpose |
|------|--------|---------|
| Token counts | Assistant message `usage` field | Volume of AI work |
| Model name | Assistant message `model` field | Which models you used |
| Files created/modified | `Write`/`Edit` tool inputs | What you actually built |
| Interactions | User messages (excluding tool results) | How many turns |
| Timestamps | Record timestamps | Time range of work |
| Project | Working directory (`cwd`) | Group by project |

### What is NOT tracked

- Prompt content (what you said to the AI)
- Response content (what the AI said back)
- File contents
- Any personally identifiable information

## Cost estimation

By default, cost display is off — most developers use subscription plans (Pro/Max) where per-token costs don't apply. Use `--cost` to enable it for pay-per-token users.

```bash
devrecap today --cost
```

Pricing is based on current published rates for Anthropic, OpenAI, and Google models.

## Output formats

**text** (default) — clean terminal output with project breakdown, files, and models

**standup** — conversational markdown ready to paste into Slack:
- "Built out **project**" when you created 5+ files
- "Iterated on **project**" when you mostly edited existing code
- "Researched and planned in **project**" for heavy AI interaction without file output

**json** — structured output for piping to other tools:
```bash
devrecap today --format json | jq '.by_project | keys'
```

## Requirements

- Go 1.26+
- Claude Code installed (creates session files at `~/.claude/projects/`)

## License

MIT
