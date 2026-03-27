# Contributing to debrief

Thanks for your interest. Here's how to contribute.

## Workflow

1. Fork the repo
2. Create a branch: `git checkout -b fix/your-thing` or `feat/your-thing`
3. Make your changes
4. Run tests: `make test`
5. Open a PR against `main`

All PRs require one approving review from the maintainer before merge.

## What to work on

Check open issues. If you have an idea that isn't tracked, open an issue first so we can align before you invest time building it.

## Code style

- Go standard library first — avoid new dependencies unless clearly justified
- `fmt.Errorf("context: %w", err)` for error wrapping
- Table-driven tests
- Functions do one thing

## Commit messages

Concise, imperative mood. Explain why, not what. Example:

```
fix: handle missing git dir gracefully

Without this, debrief panics when run outside a git repo instead of
printing a useful error.
```
