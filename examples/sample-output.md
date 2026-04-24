# Sample outputs

Reference outputs the skill should produce given a representative day. Use these as regression anchors when iterating on SKILL.md wording.

---

## Example 1 — Full four-bucket day

**Ask:** `debrief today`

**Output:**

```
Fri, Apr 18 2026

Decided
  - Went with go-yaml over mapstructure — reflection cost wasn't worth the flexibility
  - Dropped the Slack webhook path for v1 — too much scope

Shipped
  - Handle PR-squash chore commits (#11)
  - Redesigned the cost table with per-model breakdown (#12)
  - Added --project filter to standup and cost commands

Investigated
  - goreleaser's default matrix skips arm64 Linux — had to add it explicitly

Watch
  - The classifier regex is too aggressive on test: prefixes and might drop real work
```

Note: **Decided** and **Investigated** bullets come from Claude session notes. **Shipped** bullets are a mix of commits and session notes about completed work. **Watch** captures a risk the user flagged during a session but didn't file as an issue.

---

## Example 2 — Quiet day (chore-only)

**Ask:** `debrief today`

**Commits:** only `chore: bump deps`, `chore: format`, `test: flaky retry`.

**Session notes:** none survived the filter (all were planning language).

**Output:**

```
Quiet day — just chores and lints. Nothing shipped worth writing up.
```

This exact line signals "the tool worked, your day was just quiet" — distinct from "the tool found nothing" (which means a broken scan).

---

## Example 3 — Nothing at all

**Ask:** `debrief yesterday` on a day the user didn't work.

**Output:**

```
No activity to report.
```

---

## Example 4 — Slack format

**Ask:** `debrief today as slack`

**Output:**

````
`Fri, Apr 18 2026`

- Went with go-yaml over mapstructure — reflection cost wasn't worth the flexibility
- Handle PR-squash chore commits (#11)
- goreleaser's default matrix skips arm64 Linux — had to add it explicitly
- The classifier regex is too aggressive on test: prefixes and might drop real work
````

- Date is wrapped in backticks (Slack renders it as inline code).
- No section headers — flat list.
- Bucket order is preserved in bullet order.

---

## Example 5 — Week range

**Ask:** `debrief this week`

**Output:**

```
Week of Apr 13 – Apr 19, 2026

Decided
  - Went with go-yaml over mapstructure
  - Dropped cost subcommand — claudecost owns pricing

Shipped
  - Handle PR-squash chore commits (#11)
  - Humanizer visibility banner (#9)
  - CWD-fallback git discovery (#10)
  - Cost table redesign with per-model breakdown (#12)

Investigated
  - goreleaser arm64 Linux matrix gap
  - GIT_* env vars override -C flag in subprocesses

Watch
  - Classifier aggressive on test: prefixes
```

For multi-day ranges, bullets are deduplicated and merged across days. Date header is a range, not a single day.

---

## What "good output" means — tuning rubric

When dogfooding, grade each day's output against these questions:

1. **Could I paste this into Slack verbatim?** If no — the wording is off.
2. **Does every bullet describe real work I did?** If no — the note filter let noise through.
3. **Is anything I actually did missing?** If yes — either the filter is too strict, or the commit was covered-by-notes incorrectly.
4. **Did Decided/Investigated/Watch bullets land in the right buckets?** If a "decided" showed up in Shipped — the keyword list missed it.
5. **On a chore-only day, does it say "Quiet day"?** If it says "No activity" — the noise-vs-emptiness distinction broke.

Fix the SKILL.md wording — don't add code.
