package collector

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

// testRepoName is reused across discovery tests to satisfy goconst; the
// specific value doesn't matter, only that it's deterministic.
const testRepoName = "myrepo"

// sanitizedEnv returns os.Environ() with all GIT_* variables stripped. This
// isolates subprocess git invocations from leaked ambient state (e.g. when
// the test runs under a pre-push hook or inside a worktree, GIT_DIR/
// GIT_WORK_TREE/GIT_INDEX_FILE are inherited and break `git init` in temp dirs).
func sanitizedEnv() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func TestGitCollector_CollectFromTempRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a temp repo with commits.
	dir := t.TempDir()
	repo := filepath.Join(dir, "test-project")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(sanitizedEnv(),
			"GIT_AUTHOR_NAME=Test User",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test User",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	run("init")
	run("checkout", "-b", "main")
	// Belt-and-suspenders: also set identity via repo-local config so commits
	// resolve to test@example.com even if GIT_AUTHOR_* env doesn't propagate.
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")

	// Create two commits.
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")

	if err := os.WriteFile(filepath.Join(repo, "main_test.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "add tests")

	// Collect.
	g := &GitCollector{
		scanPaths: []string{dir},
		maxDepth:  2,
		author:    "test@example.com",
	}

	dr := model.DateRange{
		Start: time.Now().Add(-1 * time.Hour),
		End:   time.Now().Add(1 * time.Hour),
	}

	activities, err := g.Collect(dr)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}

	a := activities[0]
	if a.Source != "git" {
		t.Errorf("source: got %q, want %q", a.Source, "git")
	}
	if a.Project != "test-project" {
		t.Errorf("project: got %q, want %q", a.Project, "test-project")
	}
	if a.CommitCount != 2 {
		t.Errorf("commit_count: got %d, want 2", a.CommitCount)
	}
	if len(a.CommitMessages) != 2 {
		t.Errorf("commit_messages: got %d, want 2", len(a.CommitMessages))
	}
	if a.Branch != "main" {
		t.Errorf("branch: got %q, want %q", a.Branch, "main")
	}
}

func TestGitCollector_DiscoverReposDepth2(t *testing.T) {
	tmpdir := t.TempDir()

	// Create a repo nested 2 levels deep: tmpdir/level1/level2/myrepo/.git/
	repoPath := filepath.Join(tmpdir, "level1", "level2", testRepoName)
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	g := NewGitCollector([]string{tmpdir}, 2)
	repos := g.discoverRepos()

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
	if filepath.Base(repos[0]) != testRepoName {
		t.Errorf("expected repo named %q, got %s", testRepoName, repos[0])
	}
}

func TestGitCollector_DiscoverReposEmpty(t *testing.T) {
	tmpdir := t.TempDir()

	// Create an empty subdir with no .git repos.
	if err := os.MkdirAll(filepath.Join(tmpdir, "empty-subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	g := NewGitCollector([]string{tmpdir}, 2)
	repos := g.discoverRepos()

	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d: %v", len(repos), repos)
	}
}

// TestGitCollector_MultiDayCommitsSplitByDay verifies that commits on different
// calendar days produce separate Activities — one per day — so that SplitByDay
// can correctly assign each commit to its actual day without leakage.
func TestGitCollector_MultiDayCommitsSplitByDay(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	repo := filepath.Join(dir, "multi-day-project")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	day1 := time.Date(2026, time.March, 30, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, time.March, 31, 11, 0, 0, 0, time.UTC)

	run := func(env []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	baseEnv := append(sanitizedEnv(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	run(baseEnv, "init")
	run(baseEnv, "checkout", "-b", "main")
	run(baseEnv, "config", "user.email", "test@example.com")
	run(baseEnv, "config", "user.name", "Test User")

	// Commit on day1.
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(baseEnv, "add", ".")
	day1Env := make([]string, len(baseEnv), len(baseEnv)+2)
	copy(day1Env, baseEnv)
	day1Env = append(day1Env,
		"GIT_AUTHOR_DATE="+day1.Format(time.RFC3339),
		"GIT_COMMITTER_DATE="+day1.Format(time.RFC3339),
	)
	run(day1Env, "commit", "-m", "feat: day one work")

	// Commit on day2.
	if err := os.WriteFile(filepath.Join(repo, "b.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(baseEnv, "add", ".")
	day2Env := make([]string, len(baseEnv), len(baseEnv)+2)
	copy(day2Env, baseEnv)
	day2Env = append(day2Env,
		"GIT_AUTHOR_DATE="+day2.Format(time.RFC3339),
		"GIT_COMMITTER_DATE="+day2.Format(time.RFC3339),
	)
	run(day2Env, "commit", "-m", "feat: day two work")

	g := &GitCollector{
		scanPaths: []string{dir},
		maxDepth:  2,
		author:    "test@example.com",
	}

	dr := model.DateRange{
		Start: time.Date(2026, time.March, 30, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
	}

	activities, err := g.Collect(dr)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if len(activities) != 2 {
		t.Fatalf("expected 2 activities (one per day), got %d", len(activities))
	}

	// Each activity must have exactly one commit.
	for _, a := range activities {
		if a.CommitCount != 1 {
			t.Errorf("activity %q: expected 1 commit, got %d", a.Timestamp.Format("2006-01-02"), a.CommitCount)
		}
	}

	// Activities must be on different calendar days.
	day1Key := activities[0].Timestamp.UTC().Format("2006-01-02")
	day2Key := activities[1].Timestamp.UTC().Format("2006-01-02")
	if day1Key == day2Key {
		t.Errorf("both activities have the same day %q — commits are not split by day", day1Key)
	}
}

// TestGitCollector_DiscoverReposWithFallback_UsesCWDWhenConfiguredEmpty covers
// the first-run-dead-end case: user has no repos under the default scan paths
// (or points them at empty directories), so discovery falls back to CWD.
func TestGitCollector_DiscoverReposWithFallback_UsesCWDWhenConfiguredEmpty(t *testing.T) {
	// Build a workspace whose CWD contains one subdir repo.
	workspace := t.TempDir()
	repoSub := filepath.Join(workspace, testRepoName)
	if err := os.MkdirAll(filepath.Join(repoSub, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Configured scan paths point at a completely empty temp dir — zero repos.
	emptyScan := t.TempDir()

	// chdir to the workspace so os.Getwd inside the collector returns it.
	restoreCwd := chdir(t, workspace)
	defer restoreCwd()

	g := &GitCollector{scanPaths: []string{emptyScan}, maxDepth: 2}
	repos := g.discoverReposWithFallback()

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo via CWD fallback, got %d: %v", len(repos), repos)
	}
	if filepath.Base(repos[0]) != testRepoName {
		t.Errorf("expected repo named %q, got %s", testRepoName, repos[0])
	}
	if !g.UsedCWDFallback() {
		t.Error("expected UsedCWDFallback = true after empty-configured fallback")
	}
	if len(g.ScannedPaths()) == 0 {
		t.Error("expected ScannedPaths to be populated")
	}
}

// TestGitCollector_DiscoverReposWithFallback_CWDItselfIsRepo covers the case
// where the user runs debrief from inside a git repo — scanDir can't find the
// repo on its own (it only looks at subdirectories), so the fallback must
// explicitly include CWD when it has a .git.
func TestGitCollector_DiscoverReposWithFallback_CWDItselfIsRepo(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	emptyScan := t.TempDir()
	restoreCwd := chdir(t, repo)
	defer restoreCwd()

	g := &GitCollector{scanPaths: []string{emptyScan}, maxDepth: 2}
	repos := g.discoverReposWithFallback()

	if len(repos) != 1 {
		t.Fatalf("expected CWD itself as the single repo, got %d: %v", len(repos), repos)
	}
	if !g.UsedCWDFallback() {
		t.Error("expected UsedCWDFallback = true")
	}
}

// TestGitCollector_DiscoverReposWithFallback_NoFallbackWhenConfiguredFinds
// verifies we do NOT trigger the fallback (and do NOT mark usedFallback) when
// the primary configured path yielded at least one repo.
func TestGitCollector_DiscoverReposWithFallback_NoFallbackWhenConfiguredFinds(t *testing.T) {
	scanDir := t.TempDir()
	repoSub := filepath.Join(scanDir, "realrepo")
	if err := os.MkdirAll(filepath.Join(repoSub, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// CWD is a wholly unrelated temp dir — fallback should NOT scan it.
	unrelated := t.TempDir()
	restoreCwd := chdir(t, unrelated)
	defer restoreCwd()

	g := &GitCollector{scanPaths: []string{scanDir}, maxDepth: 2}
	repos := g.discoverReposWithFallback()

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo from configured path, got %d: %v", len(repos), repos)
	}
	if g.UsedCWDFallback() {
		t.Error("expected UsedCWDFallback = false when configured path produced repos")
	}
	if len(g.ScannedPaths()) != 1 || g.ScannedPaths()[0] != scanDir {
		t.Errorf("ScannedPaths should contain only the configured path, got %v", g.ScannedPaths())
	}
}

// chdir changes to dir for the duration of the test and returns a cleanup fn.
// Fails the test if chdir can't happen; resolves symlinks so macOS /var vs
// /private/var comparisons don't trip callers.
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}
	if err := os.Chdir(resolved); err != nil {
		t.Fatalf("Chdir(%q): %v", resolved, err)
	}
	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restoring cwd: %v", err)
		}
	}
}

func TestGitCollector_DiscoverRepos(t *testing.T) {
	dir := t.TempDir()

	// Create a fake repo (dir with .git subdir).
	repo := filepath.Join(dir, testRepoName)
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a non-repo dir.
	if err := os.MkdirAll(filepath.Join(dir, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}

	g := &GitCollector{scanPaths: []string{dir}, maxDepth: 2}
	repos := g.discoverRepos()

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
	if filepath.Base(repos[0]) != testRepoName {
		t.Errorf("expected %q, got %s", testRepoName, repos[0])
	}
}
