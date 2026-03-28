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
