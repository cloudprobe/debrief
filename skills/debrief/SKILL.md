---
name: debrief
description: Generate a daily standup — what you decided, shipped, and investigated — from local git history and Claude Code session logs. 100% local, no network calls. Use when the user asks for a standup, daily summary, end-of-day writeup, "what did I do today", or similar.
allowed-tools: Bash(git *), Bash(ls *), Bash(date *), Bash(pbcopy), Read, Grep, Glob
---

# debrief

Synthesize a standup from local git commits + Claude Code session logs, grouped by intent into four buckets: **Decided, Shipped, Investigated, Watch**.

Most daily-summary tools just list commits. debrief also reads the user's Claude Code session history (`~/.claude/projects/*.jsonl`) to surface decisions, discoveries, and risks that never made it into a commit message.

Runs entirely against local files. No API calls. No GitHub API.

---

## When to invoke

Trigger on any of:

- "debrief" / "/debrief"
- "what did I (do|ship|work on|decide) (today|yesterday|this week|this month)"
- "give me a standup" / "standup summary" / "daily summary" / "end-of-day writeup"
- "what's worth writing up from today"

Do **not** invoke for: changelog generation (use a changelog skill), PR descriptions, retrospectives across many weeks.

---

## Inputs

All optional. Free-form — parse intelligently:

| Input | Examples | Default |
|---|---|---|
| Time range | `today`, `yesterday`, `this week`, `this month`, `2026-04-18`, `last 3 days` | `today` |
| Project filter | `--project debrief`, `only debrief`, `for cloudprobe/*` | all repos |
| Format | `slack`, `flat`, `bullets` | sectioned text |
| Copy | `copy it`, `put it on my clipboard` | off |

If inputs are ambiguous, ask one short clarifying question before running. Never fabricate a range.

---

## Procedure

Do these steps in order. Do not skip steps even if you think the next one is obvious — the rules below are tuned and skipping loses quality.

### 1. Resolve the time range

Convert the user's input into a concrete `[start, end]` pair in the local timezone.

- `today` → today 00:00 → now
- `yesterday` → yesterday 00:00 → yesterday 23:59:59
- `this week` → most recent Monday 00:00 → now
- `this month` → day 1 of current month → now
- `last N days` → now - N days → now
- explicit `YYYY-MM-DD` → that day 00:00 → that day 23:59:59

### 2. Discover git repos

Scan these locations, depth 2:
- `~/work`
- `~/projects`
- `~/code`
- `$PWD` (fallback if above yield nothing, or if user is inside a repo)

A directory is a repo if it contains `.git/` or `.git` file. If the CWD itself is a repo, include it explicitly — directory walks that only look at children miss the CWD.

If all configured paths are missing, fall back to `$PWD` and note what was scanned so the user knows.

### 3. Collect commits per repo

For each repo, run:

```bash
git -C <repo> log --all \
  --since="<start>" --until="<end>" \
  --author="$(git -C <repo> config user.email)" \
  --format="%H|%at|%s"
```

**Important: strip `GIT_*` env vars before invoking git.** If `GIT_DIR`, `GIT_WORK_TREE`, or `GIT_INDEX_FILE` are set in the environment, git ignores `-C` and queries the ambient repo instead. This is a real bug seen in production. Use `env -i PATH="$PATH" HOME="$HOME" git -C ...` or unset the vars explicitly.

Parse each line as `<hash>|<unix-ts>|<subject>`. Keep the subject line (first line of commit message) for classification.

### 4. Collect Claude session notes

Walk `~/.claude/projects/**/*.jsonl`. Skip `memory/` and `tool-results/` subdirectories — not user work product.

Each JSONL file is one Claude Code session. Lines are records like:

```json
{"type":"assistant","sessionId":"...","timestamp":"...","cwd":"/path","gitBranch":"main","message":{"id":"msg_...","content":[{"type":"text","text":"..."},{"type":"tool_use",...}]}}
```

For each assistant message within `[start, end]`:
- Dedup globally by `message.id` across all files (same message can appear in parent + subagent files).
- For each `content` block of type `"text"`, extract the text and pass it through the **note filter** (§5).

Attribute each surviving note to a project via `rec.cwd`:
- Run `git -C <cwd> remote get-url origin` and derive `org/repo` from the URL.
- Fall back to `basename(cwd)`.
- Cache this lookup per cwd — it's called often.

### 5. Note filter rules

A text block becomes a note only if **all** of these hold. Apply in order; reject early.

**Length and shape:**
- `len(trimmed) > 0` and `len(trimmed) <= 600`
- Does not contain triple-backtick fenced code (```` ``` ````) or a tab character — both signal code, not a note.

**First sentence must be ≥ 15 chars**, where "first sentence" means: split on `". "` / `"! "` / `"? "` but ignore breaks after known abbreviations (`e.g.`, `i.e.`, `vs.`, `etc.`, `Mr.`, `Dr.`, `St.`, month abbreviations `Jan.` through `Dec.`). Use only the first paragraph (break at `\n\n`). Strip markdown: `**bold**` → `bold`, `*italic*` → `italic`, `` `code` `` → `code`, leading `- ` or `• `.

**Reject planning / hedging language.** Drop if the lowercased first sentence starts with any of:
```
"let me ", "i'll ", "i will ", "i'm going to ", "i need to ",
"let's ", "now i'll ", "next ", "first ", "to "
```

**Require action-completion prefix.** Keep only if the lowercased first sentence starts with one of:
```
"i've ", "i have ", "done", "fixed", "added", "updated", "removed",
"built", "implemented", "created", "refactored", "changed", "moved",
"cleaned", "dropped", "replaced", "simplified", "wired", "switched",
"deleted", "renamed", "extracted", "merged", "resolved",
"pushed", "committed", "tagged", "released", "deployed", "shipped",
"wrote", "rewrote", "generated", "configured", "installed", "upgraded",
"migrated", "patched", "reverted", "exposed", "enabled", "disabled",
"all tests", "tests pass", "build passes"
```

**Reject list intros and truncations:**
- Ends with `:` → list intro, drop.
- Contains `:` followed later by ` - ` → list intro, drop.
- Ends with ` N.` where N is digits (e.g. `captured: 1.`) → numbered-list fragment, drop.
- Ends with `(e.g.`, `(i.e.`, `(`, or `e.g.` → truncated, drop.

**Reject conversational responses:**
- Contains `http://` or `https://` → not a standup bullet.
- Contains a standalone `you` / `your` / `you're` / `you'll` / `yourself` word → addresses the reader, drop.
- Starts with `"go ahead"`, `"feel free"`, `"keep the"`, `"keep in"`, `"paste "`, `"try the"`, `"run the"`, `"check the"`, `"note that"`, `"just "`, `"sure,"`, `"of course"` → conversational, drop.

**Clean the surviving note:**
- Strip leading `"Done. "`, `"Done — "`, `"Done: "`, `"Done, "`, `"Done! "` — recapitalize the remainder.
- Strip leading `"I've "` or `"I have "` — recapitalize the remainder.
- If what's left is bare `"Done."` / `"Done"` / `"Done!"` — drop.

**Final quality gates (post-clean):**
- `len(note) >= 40`.
- Does not match a bare `[0-9a-f]{7,}` hex hash (naked commit SHA — noise).
- Does not start with `"pushed"`, `"committed"`, `"fixed."` — these add no info commits don't.
- Not literally `"All the content."`.

### 6. Classify

**Commits → bucket:**

| Commit shape | Bucket |
|---|---|
| Starts with `"Merge pull request"` or `"Merge branch"` | **Shipped** |
| No `:` in subject (no conventional prefix) | **Shipped** |
| Prefix `feat` / `fix` / `perf` / `refactor` / `build` / `ci` | **Shipped** |
| Prefix `docs` AND body-after-prefix > 20 chars | **Shipped** |
| Prefix `docs` AND body ≤ 20 chars | **Drop (noise)** |
| Prefix `chore` / `test` AND has `(#N)` suffix AND body-after-prefix > 20 chars | **Shipped** (merged via review) |
| Prefix `chore` / `test` without both conditions | **Drop (noise)** |
| Any other prefix | **Shipped** |

"Prefix" matching: take the text before the first `:`, lowercase it, strip anything in parentheses (so `feat(cli):` matches `feat`).

"Body-after-prefix": text after the first `:`, trimmed, with the first char capitalized.

**Notes → bucket:** match the lowercased note against these keyword sets in order. First match wins.

| If lowercased note contains | Bucket |
|---|---|
| `"decided"`, `"went with"`, `"chose"`, `"switched to"`, `"picked"` | **Decided** |
| `"found"`, `"discovered"`, `"ruled out"`, `"investigated"`, `"turns out"` | **Investigated** |
| `"risk"`, `"concern"`, `"watch out"` | **Watch** |
| Anything else | **Shipped** |

### 7. Deduplicate commits covered by notes

For each commit heading for the Shipped bucket, check if a surviving note covers it. Coverage rule:
- Extract "significant words" from the commit and each note: lowercase, strip surrounding punctuation, keep words > 4 chars that are not in `{with, from, that, this, into, have, been, when, also, then}`.
- If at least half the commit's significant words appear in any single note, drop the commit (the note already describes it).

This is the reason notes are richer than commit logs — don't skip this step.

### 8. Render

Emit bullets in this exact bucket order: **Decided → Shipped → Investigated → Watch**.

Skip empty buckets. If all four are empty and at least one commit was filtered as noise, emit:
```
Quiet day — just chores and lints. Nothing shipped worth writing up.
```
If all four are empty and zero commits existed, emit:
```
No activity to report.
```

**Default (sectioned text) format:**

```
<Date header: "Mon, Apr 18 2026" for single day; free-form for ranges>

Decided
  - <bullet>
  - <bullet>

Shipped
  - <bullet>
  - <bullet>

Investigated
  - <bullet>

Watch
  - <bullet>
```

- Two-space indent before `-` for bullets.
- Blank line between sections; **no** blank line between section header and its first bullet.
- Date header uses `Mon, Jan 2 2006` format for single-day output. For ranges, use something like `Week of Apr 13 – Apr 19, 2026`.

**Slack format (`slack` / `flat` request):** flat bullets, no section headers, wrap the date in backticks:

```
`Fri, Apr 18 2026`

- <decided bullet>
- <decided bullet>
- <shipped bullet>
- <investigated bullet>
```

Buckets still dictate the *order* of bullets in the flat list. No section headers.

**Copy request:** after rendering, run `printf '%s' "<output>" | pbcopy` (macOS) or `xclip -selection clipboard` (Linux). Confirm with a one-line status on stderr-style second message: `"Copied to clipboard."`

### 9. Display

Print the rendered standup. Do not wrap it in code fences unless the user asked for Slack format — the output is meant to be copy-pasted.

If the scan fell back to `$PWD` (configured paths had no repos), append a single italic line at the end: `*Scanned $PWD only — no repos found under ~/work, ~/projects, ~/code.*`

---

## Style and tone

- **No AI filler.** Do not use "utilize," "leverage," "delve," "pivotal," "navigate the landscape," "the intersection of," "comprehensive," "robust." Write like a tired engineer at 5pm.
- **Contractions.** "Didn't," "don't," "wasn't" — not "did not."
- **No rule-of-three.** Prefer two examples or one. Never three parallel clauses for emphasis.
- **Preserve identifiers verbatim.** File names, function names, PR numbers (`#42`), commit short-SHAs — do not rewrite or translate.
- **One-line bullets.** Each bullet is a single sentence. Fragments are fine.
- **Past tense, active voice.** "Redesigned the cost table." Not "The cost table was redesigned."

---

## Worked example

**Input:** `debrief today` on 2026-04-18.

**Collected:**
- Commit `feat(classifier): handle PR-squash chore commits (#11)` in `cloudprobe/debrief`.
- Commit `chore: bump goreleaser to 2.4.1` in `cloudprobe/debrief`.
- Session note `"Decided to go with go-yaml over mapstructure — reflection cost wasn't worth the flexibility."`
- Session note `"Found that goreleaser's default matrix skips arm64 Linux, had to add it explicitly."`
- Session note `"Risk: the classifier regex is too aggressive on test: prefixes and might drop real work."`

**Classification:**
- `feat` commit → Shipped.
- `chore` commit without `(#N)` → noise, drop.
- "Decided to go with..." → Decided.
- "Found that..." → Investigated.
- "Risk:..." → Watch.

**Output:**

```
Fri, Apr 18 2026

Decided
  - Went with go-yaml over mapstructure — reflection cost wasn't worth the flexibility

Shipped
  - Handle PR-squash chore commits (#11)

Investigated
  - goreleaser's default matrix skips arm64 Linux — had to add it explicitly

Watch
  - The classifier regex is too aggressive on test: prefixes and might drop real work
```

---

## Non-goals

- **No network calls.** This skill reads local files. Do not call the GitHub API, Linear, Jira, or any web service even if it would make the output "better."
- **No LLM-generated content beyond what's derived from the inputs.** If a bucket is empty, leave it empty — don't invent bullets.
- **No multi-person aggregation.** This is a personal standup. Team rollups are out of scope.
- **No scheduled/cron use.** Skills run inside a Claude session. For unattended standups, the user can ask daily inside a running session.

---

## Acknowledgements

Rules ported from the Go CLI at [cloudprobe/debrief](https://github.com/cloudprobe/debrief) (now archived). The 4-bucket classifier, session-note filter, and PR-squash handling are tuned to months of real standup output — preserve them literally.
