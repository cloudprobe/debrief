package collector

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudprobe/debrief/internal/config"
	"github.com/cloudprobe/debrief/internal/model"
)

const claudeBaseDir = ".claude/projects"

// claudeRecord is the raw JSONL record from Claude Code session files.
type claudeRecord struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Timestamp time.Time       `json:"timestamp"`
	CWD       string          `json:"cwd"`
	GitBranch string          `json:"gitBranch"`
	Message   json.RawMessage `json:"message"`
}

// claudeAssistantMsg represents the assistant message envelope.
type claudeAssistantMsg struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Usage   *claudeUsage         `json:"usage"`
	Content []claudeContentBlock `json:"content"`
}

// claudeUsage holds token usage from a Claude API response.
type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// claudeContentBlock represents a content block in an assistant message.
type claudeContentBlock struct {
	Type  string          `json:"type"` // "text", "tool_use", "thinking"
	Name  string          `json:"name"` // tool name (only for tool_use)
	Input json.RawMessage `json:"input"`
}

// claudeUserMsg represents user message content.
type claudeUserMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ClaudeCollector reads Claude Code JSONL session files.
type ClaudeCollector struct {
	homeDir    string
	showCost   bool
	pricingCfg config.PricingConfig
}

// NewClaudeCollector creates a new ClaudeCollector.
func NewClaudeCollector(homeDir string, showCost bool, pricingCfg config.PricingConfig) *ClaudeCollector {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	return &ClaudeCollector{homeDir: homeDir, showCost: showCost, pricingCfg: pricingCfg}
}

func (c *ClaudeCollector) Name() string { return "claude-code" }

func (c *ClaudeCollector) Available() bool {
	dir := filepath.Join(c.homeDir, claudeBaseDir)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func (c *ClaudeCollector) Collect(dr model.DateRange) ([]model.Activity, error) {
	baseDir := filepath.Join(c.homeDir, claudeBaseDir)

	jsonlFiles, err := findJSONLFiles(baseDir)
	if err != nil {
		return nil, err
	}

	var allActivities []model.Activity
	for _, f := range jsonlFiles {
		activities, err := c.parseSessionFile(f, dr)
		if err != nil {
			continue
		}
		allActivities = append(allActivities, activities...)
	}

	return allActivities, nil
}

// findJSONLFiles discovers all .jsonl files under the Claude projects directory.
func findJSONLFiles(baseDir string) ([]string, error) {
	var files []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && (info.Name() == "memory" || info.Name() == "tool-results") {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// modelUsage tracks token counts for a single model within a session.
type modelUsage struct {
	calls      int
	tokensIn   int
	tokensOut  int
	cacheRead  int
	cacheWrite int
}

// projectAccum accumulates data for one project within a session.
type projectAccum struct {
	project       string
	cwd           string
	branch        string
	sessionID     string
	firstSeen     time.Time
	lastSeen      time.Time
	interactions  int
	byModel       map[string]*modelUsage // model → per-model token counts
	tools         map[string]int         // tool name → call count
	filesCreated  map[string]bool        // unique file paths
	filesModified map[string]bool        // unique file paths
}

func newProjectAccum(project, cwd, branch, sessionID string, ts time.Time) *projectAccum {
	return &projectAccum{
		project:       project,
		cwd:           cwd,
		branch:        branch,
		sessionID:     sessionID,
		firstSeen:     ts,
		lastSeen:      ts,
		byModel:       make(map[string]*modelUsage),
		tools:         make(map[string]int),
		filesCreated:  make(map[string]bool),
		filesModified: make(map[string]bool),
	}
}

func (pa *projectAccum) touch(ts time.Time) {
	if ts.Before(pa.firstSeen) {
		pa.firstSeen = ts
	}
	if ts.After(pa.lastSeen) {
		pa.lastSeen = ts
	}
}

// addFile records a file path, making it relative to the project CWD
// and filtering out Claude internal files.
func (pa *projectAccum) addFile(absPath string, created bool) {
	if isClaudeInternal(absPath) {
		return
	}
	rel := makeRelative(absPath, pa.cwd)
	if created {
		pa.filesCreated[rel] = true
	} else {
		pa.filesModified[rel] = true
	}
}

// isClaudeInternal returns true for paths that are Claude Code metadata,
// not user work product.
func isClaudeInternal(path string) bool {
	return strings.Contains(path, "/.claude/")
}

// makeRelative converts an absolute path to a path relative to cwd.
// Falls back to the base name if the file isn't under cwd.
func makeRelative(absPath, cwd string) string {
	if cwd != "" {
		rel, err := filepath.Rel(cwd, absPath)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return filepath.Base(absPath)
}

// deferredMsg holds a raw assistant message with its record metadata for dedup.
type deferredMsg struct {
	rec claudeRecord
	raw json.RawMessage
}

// parseSessionFile reads a single JSONL file and extracts activities split by project (CWD).
func (c *ClaudeCollector) parseSessionFile(path string, dr model.DateRange) ([]model.Activity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	pricingTable := EffectivePricing(c.pricingCfg)

	// Key: "sessionID:project" → accumulator
	accums := make(map[string]*projectAccum)

	// Dedup assistant messages by message.id — keep last (most complete) chunk.
	assistantMsgs := make(map[string]deferredMsg) // message.id → last record

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var rec claudeRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}

		if rec.Timestamp.Before(dr.Start) || rec.Timestamp.After(dr.End) {
			continue
		}

		project := projectFromCWD(rec.CWD)

		key := rec.SessionID + ":" + project
		pa, ok := accums[key]
		if !ok {
			pa = newProjectAccum(project, rec.CWD, rec.GitBranch, rec.SessionID, rec.Timestamp)
			accums[key] = pa
		}
		pa.touch(rec.Timestamp)

		switch rec.Type {
		case "user":
			if isUserMessage(rec.Message) {
				pa.interactions++
			}

		case "assistant":
			// Extract message ID for dedup.
			var peek struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(rec.Message, &peek); err == nil && peek.ID != "" {
				assistantMsgs[peek.ID] = deferredMsg{rec: rec, raw: rec.Message}
			}

		}
	}

	// Process deduplicated assistant messages.
	for _, dm := range assistantMsgs {
		rec := dm.rec
		project := projectFromCWD(rec.CWD)
		key := rec.SessionID + ":" + project
		pa, ok := accums[key]
		if !ok {
			continue
		}

		var msg claudeAssistantMsg
		if err := json.Unmarshal(dm.raw, &msg); err != nil {
			continue
		}

		if msg.Usage != nil && msg.Model != "" {
			mu := pa.byModel[msg.Model]
			if mu == nil {
				mu = &modelUsage{}
				pa.byModel[msg.Model] = mu
			}
			mu.calls++
			mu.tokensIn += msg.Usage.InputTokens
			mu.tokensOut += msg.Usage.OutputTokens
			mu.cacheRead += msg.Usage.CacheReadInputTokens
			mu.cacheWrite += msg.Usage.CacheCreationInputTokens
		}

		for _, block := range msg.Content {
			if block.Type != "tool_use" || block.Name == "" {
				continue
			}
			pa.tools[block.Name]++

			switch block.Name {
			case "Write":
				if fp := extractFilePath(block.Input); fp != "" {
					pa.addFile(fp, true)
				}
			case "Edit":
				if fp := extractFilePath(block.Input); fp != "" {
					pa.addFile(fp, false)
				}
			}
		}
	}

	var activities []model.Activity
	for _, pa := range accums {
		// Sum tokens across all models for display fields.
		var totalIn, totalOut, totalCacheRead, totalCacheWrite int
		modelCalls := make(map[string]int, len(pa.byModel))
		for m, mu := range pa.byModel {
			totalIn += mu.tokensIn
			totalOut += mu.tokensOut
			totalCacheRead += mu.cacheRead
			totalCacheWrite += mu.cacheWrite
			modelCalls[m] = mu.calls
		}

		if pa.interactions == 0 && totalIn == 0 {
			continue
		}

		// Calculate cost per model and sum — avoids applying one model's rate to
		// another model's tokens when a session mixes e.g. opus and sonnet.
		var cost float64
		if c.showCost {
			var anyCostKnown bool
			for m, mu := range pa.byModel {
				c2, known := CalculateCost(pricingTable, m, mu.tokensIn, mu.tokensOut, mu.cacheRead, mu.cacheWrite)
				if known {
					cost += c2
					anyCostKnown = true
				}
			}
			if !anyCostKnown {
				cost = -1.0 // sentinel: unknown price
			}
		}

		activities = append(activities, model.Activity{
			Source:        "claude-code",
			Timestamp:     pa.firstSeen,
			EndTime:       pa.lastSeen,
			Project:       pa.project,
			Branch:        pa.branch,
			Model:         topKey(modelCalls),
			TokensIn:      totalIn,
			TokensOut:     totalOut,
			CacheRead:     totalCacheRead,
			CacheWrite:    totalCacheWrite,
			CostUSD:       cost,
			Interactions:  pa.interactions,
			FilesCreated:  setToSlice(pa.filesCreated),
			FilesModified: setToSlice(pa.filesModified),
			ToolBreakdown: copyMap(pa.tools),
		})
	}

	return activities, nil
}

// isUserMessage checks if a user record is a real user message (not a tool result).
func isUserMessage(raw json.RawMessage) bool {
	var msg claudeUserMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return false
	}
	if msg.Role != "user" {
		return false
	}
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return true
	}
	var blocks []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" {
				return true
			}
		}
	}
	return false
}

// extractFilePath pulls file_path from a tool_use input.
func extractFilePath(input json.RawMessage) string {
	var params struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ""
	}
	return params.FilePath
}

var slugCache = make(map[string]string)

// projectFromCWD extracts an "org/repo" project name from a working directory.
// Falls back to the directory base name if no git remote is found.
func projectFromCWD(cwd string) string {
	if cwd == "" {
		return "unknown"
	}
	if cached, ok := slugCache[cwd]; ok {
		return cached
	}
	result := filepath.Base(cwd)
	out, err := exec.Command("git", "-C", cwd, "remote", "get-url", "origin").Output()
	if err == nil {
		if slug := parseRepoSlug(strings.TrimSpace(string(out))); slug != "" {
			result = slug
		}
	}
	slugCache[cwd] = result
	return result
}

// topKey returns the key with the highest count in a map.
func topKey(m map[string]int) string {
	var best string
	var max int
	for k, v := range m {
		if v > max {
			max = v
			best = k
		}
	}
	return best
}

func setToSlice(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	var s []string
	for k := range m {
		s = append(s, k)
	}
	return s
}

func copyMap(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
