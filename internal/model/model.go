package model

import "time"

// Activity represents a unified activity record from any source.
type Activity struct {
	Source         string         `json:"source"` // "claude-code", "git"
	SessionID      string         `json:"session_id"`
	SessionTitle   string         `json:"session_title,omitempty"` // human-readable session name
	Timestamp      time.Time      `json:"timestamp"`
	EndTime        time.Time      `json:"end_time"`
	Duration       time.Duration  `json:"duration"`
	Project        string         `json:"project"` // derived from cwd or repo name
	Branch         string         `json:"branch"`
	Model          string         `json:"model"` // primary AI model used (empty for git)
	TokensIn       int            `json:"tokens_in"`
	TokensOut      int            `json:"tokens_out"`
	CacheRead      int            `json:"cache_read"`
	CacheWrite     int            `json:"cache_write"`
	CostUSD        float64        `json:"cost_usd"`
	Interactions   int            `json:"interactions"`    // user messages / turns
	FilesCreated   []string       `json:"files_created"`   // files written via Write tool
	FilesModified  []string       `json:"files_modified"`  // files changed via Edit tool
	ToolBreakdown  map[string]int `json:"tool_breakdown"`  // tool name → call count
	CommitCount    int            `json:"commit_count"`    // git only
	CommitMessages []string       `json:"commit_messages"` // git only
	Insertions     int            `json:"insertions"`      // git: lines added
	Deletions      int            `json:"deletions"`       // git: lines removed
	Summary        string         `json:"summary"`
}

// ProjectSummary aggregates activities for a single project.
type ProjectSummary struct {
	Name           string         `json:"name"`
	TotalCost      float64        `json:"total_cost"`
	TotalTokens    int            `json:"total_tokens"`
	Interactions   int            `json:"interactions"`
	FilesCreated   []string       `json:"files_created"`
	FilesModified  []string       `json:"files_modified"`
	ToolBreakdown  map[string]int `json:"tool_breakdown"`
	CommitCount    int            `json:"commit_count"`
	CommitMessages []string       `json:"commit_messages"`
	Insertions     int            `json:"insertions"`
	Deletions      int            `json:"deletions"`
	SessionCount   int            `json:"session_count"`
	Sessions       []Activity     `json:"sessions,omitempty"` // per-session detail
	Models         []string       `json:"models"`
	Sources        []string       `json:"sources"`
	Duration       time.Duration  `json:"duration"`
}

// ModelSummary aggregates token usage and cost by AI model.
type ModelSummary struct {
	Name      string  `json:"name"`
	TokensIn  int     `json:"tokens_in"`
	TokensOut int     `json:"tokens_out"`
	TotalCost float64 `json:"total_cost"`
	CallCount int     `json:"call_count"`
}

// DaySummary is the aggregated summary for a single day.
type DaySummary struct {
	Date         time.Time                 `json:"date"`
	Activities   []Activity                `json:"activities"`
	TotalCost    float64                   `json:"total_cost"`
	TotalTokens  int                       `json:"total_tokens"`
	Interactions int                       `json:"interactions"`
	DeepSessions int                       `json:"deep_sessions"` // sustained work blocks (>30 min)
	ByProject    map[string]ProjectSummary `json:"by_project"`
	ByModel      map[string]ModelSummary   `json:"by_model"`
}

// DateRange represents a time range for querying activities.
type DateRange struct {
	Start time.Time
	End   time.Time
}
