# devrecap

Know what you actually did today — including the hours you spent in AI coding tools.

```
$ devrecap

  devrecap — Wednesday, Mar 25 2026
  ──────────────────────────────────────────────────────

  devrecap
    71 interactions · 178.8K tokens · opus 4.6
    21 files created, 5 files modified — aggregator.go, claude.go, text.go, main.go +18 more

  claude
    45 interactions · 203.8K tokens · opus 4.6
    1 file created, 4 files modified — devrecap.rb, claude.go, text.go, Makefile

  dotfiles
    3 interactions · 659 tokens · opus 4.6
    2 commits

  ──────────────────────────────────────────────────────
  120 interactions · 384.5K tokens
```

```
$ devrecap standup

**Mar 25:**
• Built out **devrecap** — created 21 new files (aggregator.go, claude.go, text.go, main.go +17 more)
• Researched and planned in **claude** (45 AI interactions)
• Minor work on dotfiles, cloudprobe
```

## Install

```bash
brew install cloudprobe/tap/devrecap
```

Or with Go:

```bash
go install github.com/cloudprobe/devrecap/cmd/devrecap@latest
```

### Upgrade

```bash
brew upgrade devrecap
```

## Usage

```bash
devrecap                          # Today's activity (default)
devrecap yesterday                # Yesterday
devrecap week                     # This week
devrecap standup                  # Copy-paste standup (yesterday)
devrecap today --format json      # JSON output for scripting
devrecap today --cost             # Include estimated API costs
devrecap today --date 2026-03-20  # Specific date
```

## Data sources

| Source | What it tracks |
|--------|---------------|
| **Claude Code** | Sessions, tokens, models, files created/modified, interactions |
| **Git** | Commits by author across discovered repos |

Planned: Codex CLI (OpenAI), Gemini CLI (Google).

## Configuration

Config file at `~/.config/devrecap/config.yaml` (optional — works without it):

```yaml
# Directories to scan for git repos (one level deep).
git_repo_paths:
  - ~/work
  - ~/projects
  - ~/code

# Override default Claude Code session path.
claude_dir: ""

# Default output format: text, json, or standup.
default_format: text
```

## Privacy

All data stays on your machine. devrecap makes **zero network calls**.

It reads only metadata from session logs — token counts, model names, timestamps, file paths. It never reads prompt content, AI responses, or file contents.

## License

MIT
