package collector

import "github.com/cloudprobe/devrecap/internal/model"

// Collector is the interface that all data sources implement.
type Collector interface {
	// Name returns the source identifier (e.g., "claude-code").
	Name() string

	// Collect gathers activities within the given date range.
	Collect(dr model.DateRange) ([]model.Activity, error)

	// Available reports whether this collector's data source exists.
	Available() bool
}
