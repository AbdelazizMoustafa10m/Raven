package buildinfo_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/buildinfo"
)

// TestDefaultValues verifies that buildinfo package-level variables have their
// expected default values when not overridden by ldflags at build time.
func TestDefaultValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "Version defaults to dev",
			got:  buildinfo.Version,
			want: "dev",
		},
		{
			name: "Commit defaults to unknown",
			got:  buildinfo.Commit,
			want: "unknown",
		},
		{
			name: "Date defaults to unknown",
			got:  buildinfo.Date,
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

// TestGetInfo_DefaultValues verifies that GetInfo returns an Info struct
// populated from the package-level variables with their default values.
func TestGetInfo_DefaultValues(t *testing.T) {
	t.Parallel()

	info := buildinfo.GetInfo()

	assert.Equal(t, "dev", info.Version)
	assert.Equal(t, "unknown", info.Commit)
	assert.Equal(t, "unknown", info.Date)
}

// TestInfoString_Success verifies that Info.String() produces the expected
// human-readable format for various input combinations.
func TestInfoString_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{
			name: "default values",
			info: buildinfo.Info{
				Version: "dev",
				Commit:  "unknown",
				Date:    "unknown",
			},
			want: "raven vdev (commit: unknown, built: unknown)",
		},
		{
			name: "custom release values",
			info: buildinfo.Info{
				Version: "2.0.0",
				Commit:  "a1b2c3d",
				Date:    "2026-02-17T10:00:00Z",
			},
			want: "raven v2.0.0 (commit: a1b2c3d, built: 2026-02-17T10:00:00Z)",
		},
		{
			name: "semver with pre-release suffix",
			info: buildinfo.Info{
				Version: "1.0.0-rc.1",
				Commit:  "deadbeef",
				Date:    "2025-12-25T00:00:00Z",
			},
			want: "raven v1.0.0-rc.1 (commit: deadbeef, built: 2025-12-25T00:00:00Z)",
		},
		{
			name: "git describe with dirty suffix",
			info: buildinfo.Info{
				Version: "2.0.0-14-gabcdef0-dirty",
				Commit:  "abcdef0",
				Date:    "2026-01-15T08:30:00Z",
			},
			want: "raven v2.0.0-14-gabcdef0-dirty (commit: abcdef0, built: 2026-01-15T08:30:00Z)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.info.String())
		})
	}
}

// TestInfoString_EdgeCases verifies that Info.String() handles edge cases
// without panicking or producing corrupted output.
func TestInfoString_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{
			name: "all empty strings",
			info: buildinfo.Info{
				Version: "",
				Commit:  "",
				Date:    "",
			},
			want: "raven v (commit: , built: )",
		},
		{
			name: "very long version string",
			info: buildinfo.Info{
				Version: strings.Repeat("a", 1000),
				Commit:  "abc1234",
				Date:    "2026-02-17T10:00:00Z",
			},
			want: "raven v" + strings.Repeat("a", 1000) + " (commit: abc1234, built: 2026-02-17T10:00:00Z)",
		},
		{
			name: "non-ASCII characters in version",
			info: buildinfo.Info{
				Version: "v1.0.0-\u00e9\u00e8\u00ea",
				Commit:  "caf\u00e9",
				Date:    "2026-02-17",
			},
			want: "raven vv1.0.0-\u00e9\u00e8\u00ea (commit: caf\u00e9, built: 2026-02-17)",
		},
		{
			name: "unicode emoji in fields",
			info: buildinfo.Info{
				Version: "1.0.0-\U0001f680",
				Commit:  "\U0001f4a5",
				Date:    "\U0001f4c5",
			},
			want: "raven v1.0.0-\U0001f680 (commit: \U0001f4a5, built: \U0001f4c5)",
		},
		{
			name: "spaces in values",
			info: buildinfo.Info{
				Version: "1 0 0",
				Commit:  "has spaces",
				Date:    "Feb 17 2026",
			},
			want: "raven v1 0 0 (commit: has spaces, built: Feb 17 2026)",
		},
		{
			name: "special characters",
			info: buildinfo.Info{
				Version: "1.0.0+build.123",
				Commit:  "abc/def",
				Date:    "2026-02-17T10:00:00+05:30",
			},
			want: "raven v1.0.0+build.123 (commit: abc/def, built: 2026-02-17T10:00:00+05:30)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.info.String())
		})
	}
}

// TestInfoJSON_Marshal verifies that Info marshals to JSON with the expected
// field names from the struct tags.
func TestInfoJSON_Marshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{
			name: "default values",
			info: buildinfo.Info{
				Version: "dev",
				Commit:  "unknown",
				Date:    "unknown",
			},
			want: `{"version":"dev","commit":"unknown","date":"unknown"}`,
		},
		{
			name: "custom release values",
			info: buildinfo.Info{
				Version: "2.0.0",
				Commit:  "a1b2c3d",
				Date:    "2026-02-17T10:00:00Z",
			},
			want: `{"version":"2.0.0","commit":"a1b2c3d","date":"2026-02-17T10:00:00Z"}`,
		},
		{
			name: "empty strings",
			info: buildinfo.Info{
				Version: "",
				Commit:  "",
				Date:    "",
			},
			want: `{"version":"","commit":"","date":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.info)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

// TestInfoJSON_RoundTrip verifies that marshaling and then unmarshaling an Info
// struct preserves all field values.
func TestInfoJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info buildinfo.Info
	}{
		{
			name: "default values",
			info: buildinfo.Info{
				Version: "dev",
				Commit:  "unknown",
				Date:    "unknown",
			},
		},
		{
			name: "release values",
			info: buildinfo.Info{
				Version: "2.0.0",
				Commit:  "a1b2c3d",
				Date:    "2026-02-17T10:00:00Z",
			},
		},
		{
			name: "empty strings survive round-trip",
			info: buildinfo.Info{
				Version: "",
				Commit:  "",
				Date:    "",
			},
		},
		{
			name: "non-ASCII characters survive round-trip",
			info: buildinfo.Info{
				Version: "v1.0.0-\u00e9",
				Commit:  "caf\u00e9",
				Date:    "2026-02-17",
			},
		},
		{
			name: "long values survive round-trip",
			info: buildinfo.Info{
				Version: strings.Repeat("x", 500),
				Commit:  strings.Repeat("f", 40),
				Date:    "2026-02-17T10:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.info)
			require.NoError(t, err)

			var got buildinfo.Info
			err = json.Unmarshal(data, &got)
			require.NoError(t, err)

			assert.Equal(t, tt.info, got)
		})
	}
}

// TestInfoJSON_Unmarshal verifies that Info can be correctly unmarshaled from
// JSON with the expected field names.
func TestInfoJSON_Unmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    buildinfo.Info
		wantErr bool
	}{
		{
			name:  "valid JSON with all fields",
			input: `{"version":"1.0.0","commit":"abc1234","date":"2026-02-17T10:00:00Z"}`,
			want: buildinfo.Info{
				Version: "1.0.0",
				Commit:  "abc1234",
				Date:    "2026-02-17T10:00:00Z",
			},
		},
		{
			name:  "missing fields default to zero values",
			input: `{}`,
			want: buildinfo.Info{
				Version: "",
				Commit:  "",
				Date:    "",
			},
		},
		{
			name:  "extra fields are ignored",
			input: `{"version":"1.0.0","commit":"abc","date":"today","extra":"ignored"}`,
			want: buildinfo.Info{
				Version: "1.0.0",
				Commit:  "abc",
				Date:    "today",
			},
		},
		{
			name:    "invalid JSON",
			input:   `{not valid json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got buildinfo.Info
			err := json.Unmarshal([]byte(tt.input), &got)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGetInfo_ReturnsPopulatedStruct verifies that GetInfo populates the Info
// struct from the current package-level variable values.
func TestGetInfo_ReturnsPopulatedStruct(t *testing.T) {
	t.Parallel()

	info := buildinfo.GetInfo()

	// The struct fields should match the package-level variables.
	assert.Equal(t, buildinfo.Version, info.Version)
	assert.Equal(t, buildinfo.Commit, info.Commit)
	assert.Equal(t, buildinfo.Date, info.Date)
}

// TestInfoString_MatchesGetInfo verifies that the String() output from
// GetInfo() is consistent with the default values.
func TestInfoString_MatchesGetInfo(t *testing.T) {
	t.Parallel()

	info := buildinfo.GetInfo()
	str := info.String()

	assert.Contains(t, str, "raven v")
	assert.Contains(t, str, info.Version)
	assert.Contains(t, str, info.Commit)
	assert.Contains(t, str, info.Date)
}

// TestInfoZeroValue verifies that a zero-value Info struct behaves correctly
// and does not panic.
func TestInfoZeroValue(t *testing.T) {
	t.Parallel()

	var info buildinfo.Info

	// Zero-value String should not panic.
	str := info.String()
	assert.Equal(t, "raven v (commit: , built: )", str)

	// Zero-value JSON marshaling should work.
	data, err := json.Marshal(info)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":"","commit":"","date":""}`, string(data))
}

// TestInfoJSON_StructTags verifies that the JSON struct tags produce lowercase
// field names matching the API contract.
func TestInfoJSON_StructTags(t *testing.T) {
	t.Parallel()

	info := buildinfo.Info{
		Version: "1.0.0",
		Commit:  "abc",
		Date:    "today",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	// Verify the JSON keys are lowercase as specified by struct tags.
	var raw map[string]string
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	assert.Contains(t, raw, "version")
	assert.Contains(t, raw, "commit")
	assert.Contains(t, raw, "date")

	// Verify no uppercase keys leaked through.
	assert.NotContains(t, raw, "Version")
	assert.NotContains(t, raw, "Commit")
	assert.NotContains(t, raw, "Date")

	// Verify exactly 3 fields.
	assert.Len(t, raw, 3)
}

// BenchmarkInfoString benchmarks the String() method to ensure it does not have
// unexpected performance characteristics.
func BenchmarkInfoString(b *testing.B) {
	info := buildinfo.Info{
		Version: "2.0.0",
		Commit:  "a1b2c3d",
		Date:    "2026-02-17T10:00:00Z",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = info.String()
	}
}

// BenchmarkGetInfo benchmarks the GetInfo() function.
func BenchmarkGetInfo(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildinfo.GetInfo()
	}
}

// BenchmarkInfoJSONMarshal benchmarks JSON marshaling of Info.
func BenchmarkInfoJSONMarshal(b *testing.B) {
	info := buildinfo.Info{
		Version: "2.0.0",
		Commit:  "a1b2c3d",
		Date:    "2026-02-17T10:00:00Z",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(info)
	}
}
