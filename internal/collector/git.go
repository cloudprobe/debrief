package collector

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudprobe/devrecap/internal/model"
)

// GitCollector discovers git repos and extracts commit activity.
type GitCollector struct {
	scanPaths []string
	author    string // filter by author email or name; empty = current user
}

// NewGitCollector creates a GitCollector that scans the given directories.
func NewGitCollector(scanPaths []string) *GitCollector {
	return &GitCollector{scanPaths: scanPaths}
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
		a, err := g.collectRepo(repo, dr, author)
		if err != nil {
			continue
		}
		if a != nil {
			all = append(all, *a)
		}
	}
	return all, nil
}

// discoverRepos finds git repositories under the configured scan paths.
// It looks one level deep (direct children that contain .git).
func (g *GitCollector) discoverRepos() []string {
	seen := make(map[string]bool)
	var repos []string

	for _, scanPath := range g.scanPaths {
		entries, err := os.ReadDir(scanPath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(scanPath, e.Name())
			gitDir := filepath.Join(dir, ".git")
			if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
				real, err := filepath.EvalSymlinks(dir)
				if err != nil {
					real = dir
				}
				if !seen[real] {
					seen[real] = true
					repos = append(repos, dir)
				}
			}
		}
	}
	return repos
}

// resolveAuthor returns the git user email for filtering commits.
func (g *GitCollector) resolveAuthor() string {
	if g.author != "" {
		return g.author
	}
	out, err := exec.Command("git", "config", "user.email").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// collectRepo extracts commit activity from a single git repo.
func (g *GitCollector) collectRepo(repoPath string, dr model.DateRange, author string) (*model.Activity, error) {
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

	cmd := exec.Command("git", args...)
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

	project := filepath.Base(repoPath)
	var commits int
	var messages []string
	var earliest, latest time.Time

	// Get current branch.
	branch := getCurrentBranch(repoPath)

	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		ts, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}
		t := time.Unix(ts, 0)

		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
		if latest.IsZero() || t.After(latest) {
			latest = t
		}

		commits++
		messages = append(messages, parts[3])
	}

	if commits == 0 {
		return nil, nil
	}

	return &model.Activity{
		Source:         "git",
		SessionID:      project,
		Timestamp:      earliest,
		EndTime:        latest,
		Duration:       latest.Sub(earliest),
		Project:        project,
		Branch:         branch,
		CommitCount:    commits,
		CommitMessages: messages,
	}, nil
}

func getCurrentBranch(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
