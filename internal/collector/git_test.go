package collector

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudprobe/debrief/internal/model"
)

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
		cmd.Env = append(os.Environ(),
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
	repoPath := filepath.Join(tmpdir, "level1", "level2", "myrepo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	g := NewGitCollector([]string{tmpdir}, 2)
	repos := g.discoverRepos()

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d: %v", len(repos), repos)
	}
	if filepath.Base(repos[0]) != "myrepo" {
		t.Errorf("expected repo named myrepo, got %s", repos[0])
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

	baseEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	run(baseEnv, "init")
	run(baseEnv, "checkout", "-b", "main")

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

func TestGitCollector_DiscoverRepos(t *testing.T) {
	dir := t.TempDir()

	// Create a fake repo (dir with .git subdir).
	repo := filepath.Join(dir, "myrepo")
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
	if filepath.Base(repos[0]) != "myrepo" {
		t.Errorf("expected myrepo, got %s", repos[0])
	}
}
