package jsonutil_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/jsonutil"
)

// simpleObj is a helper struct used in ExtractInto tests.
type simpleObj struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestExtractFirst(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantFound bool
		wantJSON  string
	}{
		{
			name:      "plain JSON object",
			text:      `{"key":"value"}`,
			wantFound: true,
			wantJSON:  `{"key":"value"}`,
		},
		{
			name:      "JSON embedded in prose",
			text:      `Here is the result: {"name":"alice","value":42} Done.`,
			wantFound: true,
			wantJSON:  `{"name":"alice","value":42}`,
		},
		{
			name:      "JSON in markdown code fence",
			text:      "```json\n{\"verdict\":\"APPROVED\"}\n```",
			wantFound: true,
			wantJSON:  `{"verdict":"APPROVED"}`,
		},
		{
			name:      "nested JSON object returns outer first",
			text:      `{"outer":{"inner":1}}`,
			wantFound: true,
			wantJSON:  `{"outer":{"inner":1}}`,
		},
		{
			name:      "escaped quote inside string value",
			text:      `{"msg":"say \"hello\""}`,
			wantFound: true,
			wantJSON:  `{"msg":"say \"hello\""}`,
		},
		{
			name:      "no JSON in text",
			text:      "no json here at all",
			wantFound: false,
		},
		{
			name:      "empty string",
			text:      "",
			wantFound: false,
		},
		{
			name:      "unbalanced brace",
			text:      `{"key":"value"`,
			wantFound: false,
		},
		{
			name:      "brace inside string is not counted",
			text:      `{"key":"{not a brace}","ok":true}`,
			wantFound: true,
			wantJSON:  `{"key":"{not a brace}","ok":true}`,
		},
		{
			name:      "multiple JSON objects returns first",
			text:      `{"first":1} {"second":2}`,
			wantFound: true,
			wantJSON:  `{"first":1}`,
		},
		{
			name:      "JSON array is not a JSON object",
			text:      `[1,2,3]`,
			wantFound: false,
		},
		{
			name:      "only invalid JSON before valid JSON",
			text:      `{ bad json } {"good":true}`,
			wantFound: true,
			wantJSON:  `{"good":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := jsonutil.ExtractFirst(tt.text)
			if tt.wantFound {
				assert.True(t, ok, "expected JSON to be found")
				assert.Equal(t, tt.wantJSON, got)
			} else {
				assert.False(t, ok, "expected no JSON to be found")
				assert.Empty(t, got)
			}
		})
	}
}

func TestExtractInto(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantObj simpleObj
		wantErr bool
	}{
		{
			name:    "direct JSON",
			text:    `{"name":"bob","value":7}`,
			wantObj: simpleObj{Name: "bob", Value: 7},
		},
		{
			name:    "JSON embedded in prose",
			text:    `Agent output: {"name":"carol","value":99} end.`,
			wantObj: simpleObj{Name: "carol", Value: 99},
		},
		{
			// The outer object {"wrapper":...} decodes successfully into simpleObj
			// (Go's json ignores unknown fields), giving zero values. The inner
			// object is tried second but since the first unmarshal already
			// succeeded, ExtractInto returns the zero-value result.
			name:    "outer object wraps target object — outer matched first",
			text:    `{"wrapper":{"name":"dave","value":3}}`,
			wantObj: simpleObj{}, // outer JSON has no "name"/"value" fields
		},
		{
			name:    "nested: outer object matches first",
			text:    `{"name":"outer","value":1,"child":{"name":"inner","value":2}}`,
			wantObj: simpleObj{Name: "outer", Value: 1},
		},
		{
			name:    "no JSON",
			text:    "nothing here",
			wantErr: true,
		},
		{
			name:    "JSON that does not match target type",
			text:    `{"unrelated":true}`,
			wantObj: simpleObj{}, // fields are zero-valued but no error — json allows unknown fields
		},
		{
			name:    "empty text",
			text:    "",
			wantErr: true,
		},
		{
			name:    "JSON in code block",
			text:    "```\n{\"name\":\"eve\",\"value\":5}\n```",
			wantObj: simpleObj{Name: "eve", Value: 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got simpleObj
			err := jsonutil.ExtractInto(tt.text, &got)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantObj, got)
		})
	}
}

func TestExtractInto_ReviewResult(t *testing.T) {
	// Verify the primary use-case: extracting a structured review result from
	// agent output that contains surrounding prose.
	type ReviewResult struct {
		Findings []struct {
			Severity string `json:"severity"`
			File     string `json:"file"`
		} `json:"findings"`
		Verdict string `json:"verdict"`
	}

	text := `
I reviewed the code. Here are my findings:

` + "```json" + `
{
  "findings": [
    {"severity": "high", "file": "main.go"}
  ],
  "verdict": "CHANGES_NEEDED"
}
` + "```" + `

Let me know if you need more detail.
`

	var result ReviewResult
	err := jsonutil.ExtractInto(text, &result)
	require.NoError(t, err)
	assert.Equal(t, "CHANGES_NEEDED", result.Verdict)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "high", result.Findings[0].Severity)
	assert.Equal(t, "main.go", result.Findings[0].File)
}

// ---------------------------------------------------------------------------
// Additional edge-case tests for ExtractFirst
// ---------------------------------------------------------------------------

func TestExtractFirst_WhitespaceAroundJSON(t *testing.T) {
	t.Parallel()

	got, ok := jsonutil.ExtractFirst("   \n\t{\"key\":\"value\"}\n  ")
	assert.True(t, ok)
	assert.Equal(t, `{"key":"value"}`, got)
}

func TestExtractFirst_OnlyWhitespace(t *testing.T) {
	t.Parallel()

	_, ok := jsonutil.ExtractFirst("   \n\t   ")
	assert.False(t, ok)
}

func TestExtractFirst_JSONWithUnicodeValues(t *testing.T) {
	t.Parallel()

	got, ok := jsonutil.ExtractFirst(`{"name":"日本語","value":1}`)
	assert.True(t, ok)
	assert.Equal(t, `{"name":"日本語","value":1}`, got)
}

func TestExtractFirst_DeeplyNestedJSON(t *testing.T) {
	t.Parallel()

	text := `{"a":{"b":{"c":{"d":{"e":"deep"}}}}}`
	got, ok := jsonutil.ExtractFirst(text)
	assert.True(t, ok)
	assert.Equal(t, text, got)
}

func TestExtractFirst_JSONAfterMarkdownProse(t *testing.T) {
	t.Parallel()

	text := "## Review Results\n\nHere is my analysis:\n\n" + `{"verdict":"APPROVED","findings":[]}` + "\n\nEnd of review."
	got, ok := jsonutil.ExtractFirst(text)
	assert.True(t, ok)
	assert.Equal(t, `{"verdict":"APPROVED","findings":[]}`, got)
}

func TestExtractFirst_BackslashEscapeInString(t *testing.T) {
	t.Parallel()

	// Backslash before a non-quote character — must still be handled.
	got, ok := jsonutil.ExtractFirst(`{"path":"C:\\Users\\foo"}`)
	assert.True(t, ok)
	assert.Equal(t, `{"path":"C:\\Users\\foo"}`, got)
}

// ---------------------------------------------------------------------------
// Additional edge-case tests for ExtractInto
// ---------------------------------------------------------------------------

func TestExtractInto_MapDestination(t *testing.T) {
	t.Parallel()

	// Passing a map as dst exercises the non-struct unmarshalling path.
	dst := make(map[string]any)
	err := jsonutil.ExtractInto(`{"key":"value","num":42}`, &dst)
	require.NoError(t, err)
	assert.Equal(t, "value", dst["key"])
}

func TestExtractInto_EmptyJSONObject(t *testing.T) {
	t.Parallel()

	var dst simpleObj
	err := jsonutil.ExtractInto(`{}`, &dst)
	require.NoError(t, err)
	// Fields are zero-valued.
	assert.Equal(t, simpleObj{}, dst)
}

func TestExtractInto_JSONInBacktickCodeFence(t *testing.T) {
	t.Parallel()

	text := "Result:\n```\n{\"name\":\"test\",\"value\":99}\n```"
	var dst simpleObj
	err := jsonutil.ExtractInto(text, &dst)
	require.NoError(t, err)
	assert.Equal(t, "test", dst.Name)
	assert.Equal(t, 99, dst.Value)
}

func TestExtractInto_LargeJSONObject(t *testing.T) {
	t.Parallel()

	// Build a JSON object with many fields to test performance and correctness.
	type largeObj struct {
		Fields [100]string `json:"fields"`
		Count  int         `json:"count"`
	}
	var src largeObj
	src.Count = 42
	for i := range src.Fields {
		src.Fields[i] = fmt.Sprintf("value-%d", i)
	}

	encoded, err := json.Marshal(src)
	require.NoError(t, err)

	var dst largeObj
	err = jsonutil.ExtractInto(string(encoded), &dst)
	require.NoError(t, err)
	assert.Equal(t, 42, dst.Count)
}

// ---------------------------------------------------------------------------
// Fuzz test — ExtractFirst must never panic on arbitrary input
// ---------------------------------------------------------------------------

// FuzzExtractFirst verifies that the JSON extractor never panics on arbitrary
// input and that if a result is returned it is always valid JSON.
func FuzzExtractFirst(f *testing.F) {
	// Seed with known interesting inputs.
	f.Add(`{"key":"value"}`)
	f.Add(`{"nested":{"inner":true}}`)
	f.Add("```json\n{\"verdict\":\"APPROVED\"}\n```")
	f.Add(`{ bad json } {"good":true}`)
	f.Add(`{"msg":"say \"hello\""}`)
	f.Add(`{"path":"C:\\Users\\foo"}`)
	f.Add("")
	f.Add("{")
	f.Add("}")
	f.Add("{{{")
	f.Add(`{"findings":[{"severity":"high","file":"main.go"}],"verdict":"BLOCKING"}`)
	f.Add(string([]byte{0x7b, 0x22, 0x61, 0x22, 0x3a, 0x31, 0x7d})) // {"a":1}

	f.Fuzz(func(t *testing.T, input string) {
		// Must never panic.
		got, ok := jsonutil.ExtractFirst(input)

		if ok {
			// If a result is returned, it must be valid JSON.
			var probe any
			if err := json.Unmarshal([]byte(got), &probe); err != nil {
				t.Errorf("ExtractFirst returned non-JSON string %q for input %q: %v", got, input, err)
			}
		}
	})
}

// FuzzExtractInto verifies that ExtractInto never panics on arbitrary input.
func FuzzExtractInto(f *testing.F) {
	f.Add(`{"name":"alice","value":1}`)
	f.Add("prose before {\"name\":\"bob\",\"value\":2} prose after")
	f.Add("")
	f.Add("{")
	f.Add("not json at all")

	f.Fuzz(func(t *testing.T, input string) {
		var dst simpleObj
		// Must never panic; errors are acceptable.
		_ = jsonutil.ExtractInto(input, &dst)
	})
}
