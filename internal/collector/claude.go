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
	Text  string          `json:"text"` // text content (only for text blocks)
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
	homeDir      string
	showCost     bool
	pricingTable map[string]ModelPricing
}

// NewClaudeCollector creates a new ClaudeCollector.
// cacheDir is the config directory used to cache LiteLLM pricing (e.g. ~/.config/debrief).
func NewClaudeCollector(homeDir string, showCost bool, pricingCfg config.PricingConfig, cacheDir string) *ClaudeCollector {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	return &ClaudeCollector{
		homeDir:      homeDir,
		showCost:     showCost,
		pricingTable: LoadPricing(cacheDir, pricingCfg),
	}
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

	// Global state shared across all files.
	// Key: "sessionID:project" → accumulator (interaction counts, file edits, timestamps).
	accums := make(map[string]*projectAccum)
	// Global assistant message dedup: message.id → last record seen in any file.
	// Using a single global map prevents double-counting when the same message
	// appears in both a parent session file and a subagent file.
	assistantMsgs := make(map[string]deferredMsg)

	for _, f := range jsonlFiles {
		if err := c.scanFile(f, dr, accums, assistantMsgs); err != nil {
			continue
		}
	}

	return c.buildActivities(accums, assistantMsgs), nil
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
	sessionNotes  []string               // completion statements extracted from assistant text
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

// scanFile reads one JSONL file, populating accums with user events and
// assistantMsgs with assistant records (global last-wins dedup by message.id).
func (c *ClaudeCollector) scanFile(path string, dr model.DateRange, accums map[string]*projectAccum, assistantMsgs map[string]deferredMsg) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

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
			var peek struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(rec.Message, &peek); err == nil && peek.ID != "" {
				// Last-wins: overwrite any earlier record with the same message ID.
				// The final streaming chunk has the complete output_token count.
				assistantMsgs[peek.ID] = deferredMsg{rec: rec, raw: rec.Message}
			}
		}
	}
	return nil
}

// buildActivities processes the globally deduped assistant messages and returns activities.
func (c *ClaudeCollector) buildActivities(accums map[string]*projectAccum, assistantMsgs map[string]deferredMsg) []model.Activity {
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

		var textBlocks []string
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				if block.Text != "" {
					textBlocks = append(textBlocks, block.Text)
				}
			case "tool_use":
				if block.Name == "" {
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
		// Try last text block first (most likely a completion statement).
		for i := len(textBlocks) - 1; i >= 0; i-- {
			if note := extractSessionNote(textBlocks[i]); note != "" {
				pa.sessionNotes = append(pa.sessionNotes, note)
				break
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
				c2, known := CalculateCost(c.pricingTable, m, mu.tokensIn, mu.tokensOut, mu.cacheRead, mu.cacheWrite)
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
			SessionNotes:  pa.sessionNotes,
		})
	}

	return activities
}

// extractSessionNote extracts a usable standup note from an assistant text block.
// Returns empty string if the text is planning language, code, or too long/short.
func extractSessionNote(text string) string {
	text = strings.TrimSpace(text)
	if len(text) == 0 || len(text) > 600 {
		return ""
	}
	// Skip anything that looks like code.
	if strings.Contains(text, "```") || strings.Contains(text, "\t") {
		return ""
	}
	// Use the best sentence from the text.
	sentence := firstSentence(text)
	if len(sentence) < 15 {
		return ""
	}
	lower := strings.ToLower(sentence)
	// Skip planning/hedging language — only keep completion statements.
	planningPrefixes := []string{
		"let me ", "i'll ", "i will ", "i'm going to ", "i need to ",
		"let's ", "now i'll ", "next ", "first ", "to ",
	}
	for _, p := range planningPrefixes {
		if strings.HasPrefix(lower, p) {
			return ""
		}
	}
	// Must start with an action-oriented word (including "done" variants).
	actionPrefixes := []string{
		"i've ", "i have ", "done", "fixed", "added", "updated", "removed",
		"built", "implemented", "created", "refactored", "changed", "moved",
		"cleaned", "dropped", "replaced", "simplified", "wired", "switched",
		"deleted", "renamed", "extracted", "merged", "resolved",
	}
	for _, p := range actionPrefixes {
		if strings.HasPrefix(lower, p) {
			note := cleanNote(sentence)
			if note == "" {
				return ""
			}
			// Skip list intros: ends with ":" or contains "- " after a colon.
			trimmed := strings.TrimRight(note, " ")
			if strings.HasSuffix(trimmed, ":") {
				return ""
			}
			if colonIdx := strings.Index(note, ":"); colonIdx > 0 && strings.Contains(note[colonIdx:], " - ") {
				return ""
			}
			// Skip numbered list fragments: ends with " N." (e.g. "items: 1.").
			if matched := numberedListSuffix(note); matched {
				return ""
			}
			// Skip conversational responses — work descriptions don't address the reader.
			if isConversational(note) {
				return ""
			}
			// Skip truncated notes (ends mid-abbreviation or with an open paren).
			if strings.HasSuffix(note, "(e.g.") || strings.HasSuffix(note, "(i.e.") ||
				strings.HasSuffix(note, "(") || strings.HasSuffix(note, "e.g.") {
				return ""
			}
			return note
		}
	}
	return ""
}

// abbreviations are words ending in "." that should not break sentences.
var abbreviations = map[string]bool{
	"e.g": true, "i.e": true, "vs": true, "etc": true, "mr": true,
	"dr": true, "st": true, "jan": true, "feb": true, "mar": true,
	"apr": true, "jun": true, "jul": true, "aug": true, "sep": true,
	"oct": true, "nov": true, "dec": true,
}

// firstSentence returns the first sentence of text.
// Breaks on ". " but skips abbreviations (e.g., i.e., vs.) to avoid false cuts.
func firstSentence(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// Use first paragraph only.
	if idx := strings.Index(text, "\n\n"); idx > 0 {
		text = text[:idx]
	}
	text = strings.ReplaceAll(text, "\n", " ")
	text = stripMarkdown(text)

	words := strings.Fields(text)
	var buf []string
	for _, w := range words {
		buf = append(buf, w)
		// Check for sentence-ending punctuation followed by space (implicit since we split).
		last := w[len(w)-1]
		if last == '.' || last == '!' || last == '?' {
			// Check it's not an abbreviation.
			bare := strings.ToLower(strings.TrimRight(w, ".!?"))
			if abbreviations[bare] {
				continue
			}
			sentence := strings.Join(buf, " ")
			if len(sentence) >= 15 {
				return sentence
			}
			// Too short (e.g. "Done.") — keep going to find more content.
		}
	}
	s := strings.TrimSpace(text)
	if len(s) > 250 {
		if idx := strings.LastIndex(s[:250], " "); idx > 0 {
			return s[:idx]
		}
		return s[:250]
	}
	return s
}

// numberedListSuffix returns true if the string ends with a numbered list marker like " 1."
func numberedListSuffix(s string) bool {
	// Match trailing pattern: space + digits + period, e.g. "captured: 1."
	s = strings.TrimSpace(s)
	if len(s) < 3 {
		return false
	}
	if s[len(s)-1] != '.' {
		return false
	}
	// Walk back past digits.
	i := len(s) - 2
	if i < 0 || s[i] < '0' || s[i] > '9' {
		return false
	}
	for i > 0 && s[i] >= '0' && s[i] <= '9' {
		i--
	}
	return s[i] == ' ' || s[i] == ':'
}

// isConversational returns true if the note is a chat response rather than a
// description of work done — these should not appear in standup output.
func isConversational(s string) bool {
	lower := strings.ToLower(s)

	// Contains a URL — not a standup bullet.
	if strings.Contains(s, "http://") || strings.Contains(s, "https://") {
		return true
	}

	// Addresses the reader directly ("you", "your").
	for _, w := range strings.Fields(lower) {
		w = strings.Trim(w, ".,;:!?\"'()")
		if w == "you" || w == "your" || w == "you're" || w == "you'll" || w == "yourself" {
			return true
		}
	}

	// Imperative phrases directed at the user rather than describing completed work.
	imperativePrefixes := []string{
		"go ahead", "feel free", "keep the", "keep in", "paste ", "try the",
		"run the", "check the", "note that", "just ", "sure,", "of course",
	}
	for _, p := range imperativePrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}

	return false
}

// stripMarkdown removes common markdown formatting from a string.
func stripMarkdown(s string) string {
	// Remove bold/italic: **text** → text, *text* → text, _text_ → text.
	for _, pair := range []string{"**", "__", "*", "_"} {
		for strings.Contains(s, pair) {
			open := strings.Index(s, pair)
			if open < 0 {
				break
			}
			close := strings.Index(s[open+len(pair):], pair)
			if close < 0 {
				// Unterminated — just strip the marker.
				s = s[:open] + s[open+len(pair):]
				break
			}
			inner := s[open+len(pair) : open+len(pair)+close]
			s = s[:open] + inner + s[open+len(pair)+close+len(pair):]
		}
	}
	// Remove inline code: `text` → text.
	for strings.Contains(s, "`") {
		open := strings.Index(s, "`")
		close := strings.Index(s[open+1:], "`")
		if close < 0 {
			s = s[:open] + s[open+1:]
			break
		}
		inner := s[open+1 : open+1+close]
		s = s[:open] + inner + s[open+1+close+1:]
	}
	// Remove list markers at start.
	s = strings.TrimPrefix(s, "- ")
	s = strings.TrimPrefix(s, "• ")
	return strings.TrimSpace(s)
}

// cleanNote strips robotic prefixes and humanizes a session note.
// "Done. README now shows X" → "README now shows X"
// "Done — removed from tracking" → "Removed from tracking"
// "I've added X" → "Added X"
func cleanNote(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)

	// Strip "Done." / "Done —" / "Done:" / "Done," variants first,
	// then capitalize and return the remainder.
	donePrefixes := []string{
		"done. ", "done — ", "done — ", "done: ", "done, ", "done!\n", "done! ",
	}
	for _, p := range donePrefixes {
		if strings.HasPrefix(lower, p) {
			rest := strings.TrimSpace(s[len(p):])
			if rest == "" {
				return ""
			}
			return strings.ToUpper(rest[:1]) + rest[1:]
		}
	}
	// Bare "Done." / "Done" with nothing after — not useful.
	if lower == "done." || lower == "done" || lower == "done!" {
		return ""
	}

	// Strip "I've " / "I have " — convert to past tense third-person.
	if strings.HasPrefix(lower, "i've ") {
		s = strings.ToUpper(s[5:6]) + s[6:]
	} else if strings.HasPrefix(lower, "i have ") {
		s = strings.ToUpper(s[7:8]) + s[8:]
	}

	return strings.TrimSpace(s)
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
