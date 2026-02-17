package buildinfo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AbdelazizMoustafa10m/Raven/internal/buildinfo"
)

// TestDefaultValues verifies that buildinfo variables have their expected
// default values when not overridden by ldflags at build time.
func TestDefaultValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		got      string
		wantDflt string
	}{
		{
			name:     "Version defaults to dev",
			got:      buildinfo.Version,
			wantDflt: "dev",
		},
		{
			name:     "Commit defaults to unknown",
			got:      buildinfo.Commit,
			wantDflt: "unknown",
		},
		{
			name:     "Date defaults to unknown",
			got:      buildinfo.Date,
			wantDflt: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantDflt, tt.got)
		})
	}
}
