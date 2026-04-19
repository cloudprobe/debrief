package collector

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

// GitCollector discovers git repos and extracts commit activity.
type GitCollector struct {
	scanPaths []string
	maxDepth  int
	author    string // filter by author email or name; empty = current user
}

// gitCommand returns an *exec.Cmd for `git` with GIT_* env stripped.
//
// This matters because debrief may be invoked from contexts that export
// GIT_DIR / GIT_WORK_TREE / GIT_INDEX_FILE — git hooks, other test harnesses,
// tooling that shells out to debrief from inside a repo operation. Git
// respects those env vars over `-C <path>`, so without stripping them the
// subprocess would query the ambient repo instead of the one we asked for.
func gitCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		out = append(out, kv)
	}
	cmd.Env = out
	return cmd
}

// NewGitCollector creates a GitCollector that scans the given directories.
func NewGitCollector(scanPaths []string, maxDepth int) *GitCollector {
	if maxDepth <= 0 {
		maxDepth = 2
	}
	return &GitCollector{scanPaths: scanPaths, maxDepth: maxDepth}
}

func (g *GitCollector) Name() string { return "git" }

func (g *GitCollector) Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func (g *GitCollector) Collect(dr model.DateRange) ([]model.Activity, error) {
	repos := g.discoverRepos()
	author := g.resolveAuthor()

	var all []model.Activity
	for _, repo := range repos {
		activities, err := g.collectRepo(repo, dr, author)
		if err != nil {
			continue
		}
		all = append(all, activities...)
	}
	return all, nil
}

// discoverRepos finds git repositories under the configured scan paths.
// It scans up to g.maxDepth directory levels deep.
func (g *GitCollector) discoverRepos() []string {
	seen := make(map[string]bool)
	var repos []string

	for _, scanPath := range g.scanPaths {
		if _, err := os.Stat(scanPath); os.IsNotExist(err) {
			continue
		}
		g.scanDir(scanPath, 0, seen, &repos)
	}
	return repos
}

// scanDir recursively walks dir up to g.maxDepth levels, collecting git repos.
func (g *GitCollector) scanDir(dir string, depth int, seen map[string]bool, repos *[]string) {
	if depth > g.maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subdir := filepath.Join(dir, e.Name())
		gitDir := filepath.Join(subdir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			real, err := filepath.EvalSymlinks(subdir)
			if err != nil {
				real = subdir
			}
			if !seen[real] {
				seen[real] = true
				*repos = append(*repos, subdir)
			}
			// Keep recursing — nested repos (e.g. a workspace containing multiple
			// project repos) must still be discovered.
		}
		g.scanDir(subdir, depth+1, seen, repos)
	}
}

// resolveAuthor returns the git user email for filtering commits.
func (g *GitCollector) resolveAuthor() string {
	if g.author != "" {
		return g.author
	}
	out, err := gitCommand("config", "user.email").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// dayGroup accumulates commits for a single calendar day in a repo.
type dayGroup struct {
	earliest   time.Time
	latest     time.Time
	messages   []string
	insertions int
	deletions  int
}

// collectRepo extracts commit activity from a single git repo, returning one
// Activity per calendar day so that SplitByDay can assign commits to their
// actual day without cross-day leakage.
func (g *GitCollector) collectRepo(repoPath string, dr model.DateRange, author string) ([]model.Activity, error) {
	// Format: hash|author_email|timestamp|subject
	args := []string{
		"-C", repoPath,
		"log", "--all",
		"--format=%H|%ae|%at|%s",
		"--since=" + dr.Start.Format(time.RFC3339),
		"--until=" + dr.End.Format(time.RFC3339),
	}
	if author != "" {
		args = append(args, "--author="+author)
	}

	cmd := gitCommand(args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil, nil
	}

	project := repoSlug(repoPath)
	branch := getCurrentBranch(repoPath)

	// Group commits by local calendar day so each day gets its own Activity.
	byDay := make(map[string]*dayGroup)
	var dayOrder []string // preserve insertion order for stable output

	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		hash := parts[0]
		ts, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}
		t := time.Unix(ts, 0)
		key := t.Local().Format("2006-01-02")

		dg, ok := byDay[key]
		if !ok {
			dg = &dayGroup{earliest: t, latest: t}
			byDay[key] = dg
			dayOrder = append(dayOrder, key)
		}
		if t.Before(dg.earliest) {
			dg.earliest = t
		}
		if t.After(dg.latest) {
			dg.latest = t
		}
		dg.messages = append(dg.messages, parts[3])

		ins, del := commitDiffStats(repoPath, hash)
		dg.insertions += ins
		dg.deletions += del
	}

	if len(byDay) == 0 {
		return nil, nil
	}

	activities := make([]model.Activity, 0, len(dayOrder))
	for _, key := range dayOrder {
		dg := byDay[key]
		activities = append(activities, model.Activity{
			Source:         "git",
			Timestamp:      dg.earliest,
			EndTime:        dg.latest,
			Project:        project,
			Branch:         branch,
			CommitCount:    len(dg.messages),
			CommitMessages: dg.messages,
			Insertions:     dg.insertions,
			Deletions:      dg.deletions,
		})
	}
	return activities, nil
}

// commitDiffStats returns insertions and deletions for a single commit.
func commitDiffStats(repoPath, hash string) (int, int) {
	cmd := gitCommand("-C", repoPath, "diff-tree", "--numstat", "--no-commit-id", hash)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, 0
	}

	var ins, del int
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// Binary files show "-" instead of numbers.
		if fields[0] != "-" {
			if n, err := strconv.Atoi(fields[0]); err == nil {
				ins += n
			}
		}
		if fields[1] != "-" {
			if n, err := strconv.Atoi(fields[1]); err == nil {
				del += n
			}
		}
	}
	return ins, del
}

// repoSlug returns "org/repo" from the git remote URL, falling back to the directory name.
func repoSlug(repoPath string) string {
	out, err := gitCommand("-C", repoPath, "remote", "get-url", "origin").Output()
	if err == nil {
		if slug := parseRepoSlug(strings.TrimSpace(string(out))); slug != "" {
			return slug
		}
	}
	return filepath.Base(repoPath)
}

// parseRepoSlug extracts "org/repo" from a git remote URL.
// Handles HTTPS (https://github.com/org/repo.git) and SSH (git@github.com:org/repo.git).
func parseRepoSlug(url string) string {
	url = strings.TrimSuffix(url, ".git")

	// SSH: git@github.com:org/repo
	if i := strings.Index(url, ":"); i > 0 && !strings.Contains(url[:i], "/") {
		parts := strings.Split(url[i+1:], "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
	}

	// HTTPS: https://github.com/org/repo
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}

	return ""
}

func getCurrentBranch(repoPath string) string {
	out, err := gitCommand("-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
