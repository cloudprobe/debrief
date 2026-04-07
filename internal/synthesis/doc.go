// Package synthesis provides Claude-powered standup synthesis for debrief.
// It pipes collected activity data through `claude -p` to produce a terse,
// opinionated standup summary. Falls back to ErrNoClaude when the binary
// is not available.
package synthesis
