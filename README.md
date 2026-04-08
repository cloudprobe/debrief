<p align="center">
  <h1 align="center">debrief</h1>
  <p align="center">Know what you actually shipped today.</p>
</p>

<p align="center">
  <a href="https://github.com/cloudprobe/debrief/actions/workflows/ci.yml"><img src="https://github.com/cloudprobe/debrief/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://coveralls.io/github/cloudprobe/debrief?branch=main"><img src="https://coveralls.io/repos/github/cloudprobe/debrief/badge.svg?branch=main&t=1" alt="Coverage"></a>
  <a href="https://goreportcard.com/report/github.com/cloudprobe/debrief"><img src="https://goreportcard.com/badge/github.com/cloudprobe/debrief" alt="Go Report Card"></a>
  <a href="https://github.com/cloudprobe/debrief/releases/latest"><img src="https://img.shields.io/github/v/release/cloudprobe/debrief" alt="Latest Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/cloudprobe/debrief" alt="License: MIT"></a>
</p>

---

> [!IMPORTANT]
> **100% local. Zero network calls.** debrief reads your git history and Claude Code session files directly from disk. Nothing leaves your machine — no API calls, no logins, no cloud.

---

## 💡 Why debrief?

- Writing standup notes from memory wastes time and misses half of what you actually did.
- Claude Code session logs contain valuable context — decisions made, discoveries, blockers — buried in `.jsonl` files you never read again.
- Cost visibility across multiple Claude access types (direct API, Max/Pro, Vertex, Bedrock) requires manual calculation.

debrief solves all three in seconds.

---

## 🚀 Install

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

Choose your Claude Code access type. Config is written to `~/.config/debrief/config.yaml`.

---

## 📋 Output samples

**`debrief standup`** — flat bullets, sorted by signal type:

```
Wed, Apr 8 2026

  - Decided to use gRPC instead of REST for the internal service calls
  - Added smart local classifier — Decided/Shipped/Investigated/Risk buckets
  - Fixed UTF-8 rune-boundary truncation using utf8.RuneStart
  - Found that squash commits bypass conventional prefix filter — fixed commitBucket
```

**`debrief standup --format slack`** — ready to paste into Slack:

```
`Wed, Apr 8 2026`

- Decided to use gRPC instead of REST for the internal service calls
- Added smart local classifier — Decided/Shipped/Investigated/Risk buckets
- Fixed UTF-8 rune-boundary truncation using utf8.RuneStart
```

**`debrief cost week`** — per-model cost breakdown:

```
┌────────────┬──────────────────────────┬────────────┐
│ Date       │ Model                    │ Cost (USD) │
├────────────┼──────────────────────────┼────────────┤
│ 2026-04-06 │ opus 4.6                 │     $3.21  │
│            │ sonnet 4.6               │     $0.84  │
│            │ subtotal                 │     $4.05  │
├────────────┼──────────────────────────┼────────────┤
│ 2026-04-07 │ sonnet 4.6               │     $1.12  │
├────────────┼──────────────────────────┼────────────┤
│ grand total│                          │     $5.17  │
└────────────┴──────────────────────────┴────────────┘
```

---

## 🛠 Commands

| Command | Args | Description |
|---------|------|-------------|
| `debrief init` | — | Interactive setup wizard |
| `debrief standup` | `today` `yesterday` `week` `month` `-d YYYY-MM-DD` | Standup summary from commits + AI sessions |
| `debrief cost` | `today` `yesterday` `week` `month` `-d YYYY-MM-DD` | Estimated API cost with per-model breakdown |
| `debrief log` | `"message"` / `--list` | Record or list journal entries |
| `debrief version` | — | Print version |

<details>
<summary>All flags</summary>

| Flag | Commands | Description |
|------|----------|-------------|
| `--format` | `standup` | Output format: `text` (default) or `slack` |
| `--copy` | `standup`, `cost` | Copy output to clipboard |
| `--project`, `-p` | `standup`, `cost` | Filter to repos matching substring |
| `--date`, `-d` | all | Override date (YYYY-MM-DD) |

</details>

---

## ⚙️ Configuration

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

---

## 🔒 How it works

```mermaid
flowchart LR
    A[git log] --> C[classifier]
    B["~/.claude/*.jsonl"] --> C
    D["debrief log entries"] --> C
    C --> E[Decided]
    C --> F[Shipped]
    C --> G[Investigated]
    C --> H[Risk]
    E & F & G & H --> I[output]
```

The local classifier runs entirely in-process — no model calls, no network.

**Commit signal rules:**

| Prefix | Bucket |
|--------|--------|
| `feat`, `fix`, `perf`, `refactor` | Shipped |
| `chore`, `test`, `ci`, `docs` | Skipped |
| anything else | Shipped (fallback) |

**Session note signal rules:**

| Keyword pattern | Bucket |
|-----------------|--------|
| "decided", "went with", "chose" | Decided |
| "found", "discovered", "turns out" | Investigated |
| "risk", "concern", "blocked" | Risk |

Output order is always: Decided → Shipped → Investigated → Risk.

---

## 📖 debrief log

`debrief log` lets you record short journal entries during the day that feed directly into the classifier.

```sh
# record an entry
debrief log "decided to use gRPC instead of REST"
debrief log "found that squash commits bypass prefix filter"
debrief log "concern: migration could break existing configs"

# review today's entries
debrief log --list
```

Entries are stored locally and appear in your next standup summary alongside commits and session notes. Writing entries in natural language ("decided to...", "found that...", "concern: ...") ensures they land in the right classifier bucket.

---

<details>
<summary>FAQ</summary>

**Does debrief send any data to the internet?**

No. It reads only from your local filesystem: `git log` output, `~/.claude/projects/*.jsonl` session files, and `debrief log` entries. No network calls are made at any point.

**What Claude access types are supported for cost tracking?**

Direct API, Max/Pro subscription, Vertex AI, and Amazon Bedrock. Set your type with `debrief init` or manually in `config.yaml` under `pricing.preset`.

**Why are some commits not showing up in standup?**

Commits with `chore`, `test`, `ci`, or `docs` prefixes are filtered out by default — they add noise to standup output. Use `debrief log` to surface anything filtered that actually matters.

**The cost table shows $0.00 for some models — is that correct?**

Zero-cost entries are filtered from the table automatically. If a model shows up with no cost (e.g. Max/Pro subscription models), it will not appear in the cost output.

**`debrief standup dsad` returned an error — is that expected?**

Yes. Unknown time-range arguments are rejected with an error listing the allowed values (`today`, `yesterday`, `week`, `month`, `-d YYYY-MM-DD`).

</details>

---

## License

MIT — see [LICENSE](LICENSE).
