// Package buildinfo exposes version, commit, and build date set via ldflags.
package buildinfo

// These variables are set at build time via -ldflags -X.
var (
	// Version is the semantic version or git describe output.
	Version = "dev"

	// Commit is the short git commit SHA.
	Commit = "unknown"

	// Date is the UTC build timestamp in RFC3339 format.
	Date = "unknown"
)
