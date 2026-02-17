package buildinfo

import "fmt"

// Info holds structured build information suitable for JSON serialization.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// GetInfo returns the current build information as a structured type.
func GetInfo() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}

// String returns a human-readable version string.
// Example: "raven v2.0.0 (commit: a1b2c3d, built: 2026-02-17T10:00:00Z)"
func (i Info) String() string {
	return fmt.Sprintf("raven v%s (commit: %s, built: %s)", i.Version, i.Commit, i.Date)
}
