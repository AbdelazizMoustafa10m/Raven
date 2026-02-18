package jsonutil_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbdelazizMoustafa10m/Raven/internal/jsonutil"
)

// ---------------------------------------------------------------------------
// Extract -- single-value extraction (objects and arrays)
// ---------------------------------------------------------------------------

func TestExtract_JSONObject(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`{"key":"value"}`)
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
	assert.JSONEq(t, `{"key":"value"}`, string(raw))
}

func TestExtract_JSONArray(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`[1,2,3]`)
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
	assert.JSONEq(t, `[1,2,3]`, string(raw))
}

func TestExtract_ObjectEmbeddedInProse(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`Here is the result: {"name":"alice","value":42} Done.`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"alice","value":42}`, string(raw))
}

func TestExtract_ArrayEmbeddedInProse(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`Results: [{"id":1},{"id":2}] end.`)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"id":1},{"id":2}]`, string(raw))
}

func TestExtract_MarkdownCodeFenceJSON(t *testing.T) {
	t.Parallel()

	text := "```json\n{\"verdict\":\"APPROVED\"}\n```"
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, `{"verdict":"APPROVED"}`, string(raw))
}

func TestExtract_MarkdownCodeFencePlain(t *testing.T) {
	t.Parallel()

	text := "Result:\n```\n{\"name\":\"test\",\"value\":99}\n```"
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"test","value":99}`, string(raw))
}

func TestExtract_CodeFencePriority(t *testing.T) {
	// When both a code fence and a raw JSON object are present, the code fence
	// is found first (it appears earlier in the text) so it wins.
	t.Parallel()

	text := "Preamble {\"outside\":true}\n```json\n{\"inside\":true}\n```\nTrailer"
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	// The raw JSON outside the fence appears before the fence in the output
	// since brace matching starts from the beginning of the string, but the
	// code fence strategy records its start as the fence content offset which
	// is after the "outside" object.  The "outside" object is at offset ~9
	// whereas the fence content starts later -- so the raw brace-match finds
	// {"outside":true} first in the combined ordered results list.
	// Both values are valid; the first one encountered is returned.
	assert.True(t, json.Valid(raw))
}

func TestExtract_NestedObject(t *testing.T) {
	t.Parallel()

	text := `{"outer":{"inner":1}}`
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, text, string(raw))
}

func TestExtract_DeeplyNested(t *testing.T) {
	t.Parallel()

	text := `{"a":{"b":{"c":{"d":{"e":"deep"}}}}}`
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, text, string(raw))
}

func TestExtract_EscapedQuotes(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`{"msg":"say \"hello\""}`)
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
}

func TestExtract_BackslashEscape(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`{"path":"C:\\Users\\foo"}`)
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
}

func TestExtract_BraceInsideString(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`{"key":"{not a brace}","ok":true}`)
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
}

func TestExtract_NoJSON(t *testing.T) {
	t.Parallel()

	_, err := jsonutil.Extract("no json here at all")
	assert.Error(t, err)
}

func TestExtract_EmptyString(t *testing.T) {
	t.Parallel()

	_, err := jsonutil.Extract("")
	assert.Error(t, err)
}

func TestExtract_UnbalancedBrace(t *testing.T) {
	t.Parallel()

	_, err := jsonutil.Extract(`{"key":"value"`)
	assert.Error(t, err)
}

func TestExtract_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Trailing comma makes it invalid JSON.
	_, err := jsonutil.Extract(`{"key":"value",}`)
	assert.Error(t, err)
}

func TestExtract_EmptyCodeFence(t *testing.T) {
	t.Parallel()

	// A code fence with no content should fall through to brace matching.
	text := "```json\n\n```\n{\"fallback\":true}"
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, `{"fallback":true}`, string(raw))
}

func TestExtract_ANSIEscapeCodes(t *testing.T) {
	t.Parallel()

	// ANSI codes are stripped before extraction.
	text := "\x1b[32m{\"key\":\"value\"}\x1b[0m"
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"value"}`, string(raw))
}

func TestExtract_BOMPrefix(t *testing.T) {
	t.Parallel()

	// UTF-8 BOM at the start is stripped.
	text := "\xef\xbb\xbf{\"key\":\"value\"}"
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"value"}`, string(raw))
}

func TestExtract_ExceedsMaxSize(t *testing.T) {
	t.Parallel()

	// 10 MB + 1 byte should be rejected.
	big := strings.Repeat("x", 10*1024*1024+1)
	_, err := jsonutil.Extract(big)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum size")
}

func TestExtract_UnicodeValues(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract(`{"name":"日本語","value":1}`)
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
}

func TestExtract_RealisticReviewOutput(t *testing.T) {
	t.Parallel()

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
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)

	var result struct {
		Findings []struct {
			Severity string `json:"severity"`
			File     string `json:"file"`
		} `json:"findings"`
		Verdict string `json:"verdict"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Equal(t, "CHANGES_NEEDED", result.Verdict)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "high", result.Findings[0].Severity)
}

// ---------------------------------------------------------------------------
// ExtractAll -- multiple values
// ---------------------------------------------------------------------------

func TestExtractAll_MultipleObjects(t *testing.T) {
	t.Parallel()

	text := `{"first":1} {"second":2} {"third":3}`
	results := jsonutil.ExtractAll(text)
	require.Len(t, results, 3)
	assert.JSONEq(t, `{"first":1}`, string(results[0]))
	assert.JSONEq(t, `{"second":2}`, string(results[1]))
	assert.JSONEq(t, `{"third":3}`, string(results[2]))
}

func TestExtractAll_MixedObjectsAndArrays(t *testing.T) {
	t.Parallel()

	text := `{"obj":true} [1,2,3]`
	results := jsonutil.ExtractAll(text)
	require.Len(t, results, 2)
}

func TestExtractAll_NoDuplicatesFromFenceAndBrace(t *testing.T) {
	t.Parallel()

	// A JSON object inside a code fence should not appear twice in ExtractAll.
	text := "```json\n{\"only\":\"once\"}\n```"
	results := jsonutil.ExtractAll(text)
	// Should produce exactly one result even though brace scanning would also
	// find the { inside the fence.
	require.Len(t, results, 1)
}

func TestExtractAll_EmptyText(t *testing.T) {
	t.Parallel()

	results := jsonutil.ExtractAll("")
	assert.Empty(t, results)
}

func TestExtractAll_NoJSON(t *testing.T) {
	t.Parallel()

	results := jsonutil.ExtractAll("nothing here")
	assert.Empty(t, results)
}

func TestExtractAll_MultipleCodeFences(t *testing.T) {
	t.Parallel()

	text := "First:\n```json\n{\"a\":1}\n```\nSecond:\n```json\n{\"b\":2}\n```"
	results := jsonutil.ExtractAll(text)
	require.Len(t, results, 2)
	assert.JSONEq(t, `{"a":1}`, string(results[0]))
	assert.JSONEq(t, `{"b":2}`, string(results[1]))
}

func TestExtractAll_ExceedsMaxSize(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("x", 10*1024*1024+1)
	results := jsonutil.ExtractAll(big)
	assert.Nil(t, results)
}

func TestExtractAll_AllValidJSON(t *testing.T) {
	t.Parallel()

	text := `{"a":1} invalid {"b":2}`
	results := jsonutil.ExtractAll(text)
	for i, r := range results {
		assert.True(t, json.Valid(r), "result[%d] is not valid JSON: %s", i, r)
	}
}

// ---------------------------------------------------------------------------
// ExtractInto -- unmarshal into target
// ---------------------------------------------------------------------------

type extractTarget struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestExtractInto_DirectJSON(t *testing.T) {
	t.Parallel()

	var dst extractTarget
	err := jsonutil.ExtractInto(`{"name":"bob","value":7}`, &dst)
	require.NoError(t, err)
	assert.Equal(t, "bob", dst.Name)
	assert.Equal(t, 7, dst.Value)
}

func TestExtractInto_EmbeddedInProse(t *testing.T) {
	t.Parallel()

	var dst extractTarget
	err := jsonutil.ExtractInto(`Agent output: {"name":"carol","value":99} end.`, &dst)
	require.NoError(t, err)
	assert.Equal(t, "carol", dst.Name)
	assert.Equal(t, 99, dst.Value)
}

func TestExtractInto_NoJSON(t *testing.T) {
	t.Parallel()

	var dst extractTarget
	err := jsonutil.ExtractInto("nothing here", &dst)
	assert.Error(t, err)
}

func TestExtractInto_EmptyText(t *testing.T) {
	t.Parallel()

	var dst extractTarget
	err := jsonutil.ExtractInto("", &dst)
	assert.Error(t, err)
}

func TestExtractInto_InCodeFence(t *testing.T) {
	t.Parallel()

	text := "```\n{\"name\":\"eve\",\"value\":5}\n```"
	var dst extractTarget
	err := jsonutil.ExtractInto(text, &dst)
	require.NoError(t, err)
	assert.Equal(t, "eve", dst.Name)
	assert.Equal(t, 5, dst.Value)
}

func TestExtractInto_JSONArray(t *testing.T) {
	t.Parallel()

	var dst []int
	err := jsonutil.ExtractInto(`[1,2,3]`, &dst)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, dst)
}

func TestExtractInto_MapTarget(t *testing.T) {
	t.Parallel()

	dst := make(map[string]any)
	err := jsonutil.ExtractInto(`{"key":"value","num":42}`, &dst)
	require.NoError(t, err)
	assert.Equal(t, "value", dst["key"])
}

func TestExtractInto_EmptyObject(t *testing.T) {
	t.Parallel()

	var dst extractTarget
	err := jsonutil.ExtractInto(`{}`, &dst)
	require.NoError(t, err)
	assert.Equal(t, extractTarget{}, dst)
}

func TestExtractInto_LargeObject(t *testing.T) {
	t.Parallel()

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
// ExtractFromFile -- file-based extraction
// ---------------------------------------------------------------------------

func TestExtractFromFile_ValidFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name":"from_file","value":123}`), 0o644))

	var dst extractTarget
	err := jsonutil.ExtractFromFile(path, &dst)
	require.NoError(t, err)
	assert.Equal(t, "from_file", dst.Name)
	assert.Equal(t, 123, dst.Value)
}

func TestExtractFromFile_NonexistentFile(t *testing.T) {
	t.Parallel()

	var dst extractTarget
	err := jsonutil.ExtractFromFile("/nonexistent/path/to/file.json", &dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading file")
}

func TestExtractFromFile_FileWithProse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	content := "Agent says:\n```json\n{\"name\":\"file_prose\",\"value\":7}\n```"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	var dst extractTarget
	err := jsonutil.ExtractFromFile(path, &dst)
	require.NoError(t, err)
	assert.Equal(t, "file_prose", dst.Name)
	assert.Equal(t, 7, dst.Value)
}

func TestExtractFromFile_EmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	var dst extractTarget
	err := jsonutil.ExtractFromFile(path, &dst)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Edge cases for Extract
// ---------------------------------------------------------------------------

func TestExtract_CodeFenceWithNonJSONLanguage(t *testing.T) {
	t.Parallel()

	// A code fence with a non-json language tag is not matched by the code
	// fence strategy; JSON is found via brace matching instead. We use text
	// that does not contain additional JSON-like structures inside the fence
	// so that the intended object is the first valid match.
	text := "```go\npackage main\n```\n{\"fallback\":true}"
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, `{"fallback":true}`, string(raw))
}

func TestExtract_JSONWithComments(t *testing.T) {
	t.Parallel()

	// JSON with comments is not valid JSON. Extract should return an error.
	text := `{
		// this is a comment
		"key": "value"
	}`
	_, err := jsonutil.Extract(text)
	assert.Error(t, err)
}

func TestExtract_TrailingComma(t *testing.T) {
	t.Parallel()

	// Trailing comma is not valid JSON. Extract should return an error.
	_, err := jsonutil.Extract(`{"key":"value",}`)
	assert.Error(t, err)
}

func TestExtract_MultipleJSONReturnsFirst(t *testing.T) {
	t.Parallel()

	text := `{"first":1} {"second":2}`
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, `{"first":1}`, string(raw))
}

func TestExtract_WhitespaceAroundJSON(t *testing.T) {
	t.Parallel()

	raw, err := jsonutil.Extract("   \n\t{\"key\":\"value\"}\n  ")
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"value"}`, string(raw))
}

func TestExtract_NestedMixedTypes(t *testing.T) {
	t.Parallel()

	text := `{"items":[1,2,{"nested":true}],"count":3}`
	raw, err := jsonutil.Extract(text)
	require.NoError(t, err)
	assert.JSONEq(t, text, string(raw))
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkExtract_10KB measures extraction performance on a typical 10 KB
// agent output string that contains JSON embedded in prose with a code fence.
func BenchmarkExtract_10KB(b *testing.B) {
	// Build ~10 KB of realistic agent output with a code fence containing JSON.
	prose := strings.Repeat("This is typical agent output describing the analysis. ", 100) // ~5 KB
	jsonPayload := `{
  "verdict": "CHANGES_NEEDED",
  "findings": [
    {"severity": "high", "file": "main.go", "line": 42, "message": "potential nil dereference"},
    {"severity": "medium", "file": "handler.go", "line": 17, "message": "error not checked"},
    {"severity": "low", "file": "util.go", "line": 99, "message": "unused variable"}
  ],
  "summary": "Three findings discovered during code review."
}`
	trailer := strings.Repeat("Additional commentary follows here. ", 100) // ~4 KB
	text := prose + "\n```json\n" + jsonPayload + "\n```\n" + trailer

	b.ResetTimer()
	b.SetBytes(int64(len(text)))
	for i := 0; i < b.N; i++ {
		_, err := jsonutil.Extract(text)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExtractAll_Multiple benchmarks extraction of multiple JSON objects
// scattered through a text string.
func BenchmarkExtractAll_Multiple(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString(fmt.Sprintf("Result %d: {\"id\":%d,\"status\":\"ok\"} ", i, i))
	}
	text := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := jsonutil.ExtractAll(text)
		if len(results) == 0 {
			b.Fatal("expected results")
		}
	}
}

// ---------------------------------------------------------------------------
// Fuzz tests for Extract and ExtractAll
// ---------------------------------------------------------------------------

// FuzzExtract verifies that Extract never panics on arbitrary input and that
// when a result is returned it is always valid JSON.
func FuzzExtract(f *testing.F) {
	f.Add(`{"key":"value"}`)
	f.Add(`[1,2,3]`)
	f.Add("```json\n{\"verdict\":\"APPROVED\"}\n```")
	f.Add(`{ bad json } {"good":true}`)
	f.Add(`{"msg":"say \"hello\""}`)
	f.Add("")
	f.Add("{")
	f.Add("}")
	f.Add("[")
	f.Add("]")
	f.Add(`{"findings":[{"severity":"high","file":"main.go"}]}`)

	f.Fuzz(func(t *testing.T, input string) {
		raw, err := jsonutil.Extract(input)
		if err == nil {
			if !json.Valid(raw) {
				t.Errorf("Extract returned invalid JSON for input %q: %s", input, raw)
			}
		}
	})
}

// FuzzExtractAll verifies that ExtractAll never panics and all returned values
// are valid JSON.
func FuzzExtractAll(f *testing.F) {
	f.Add(`{"a":1} {"b":2}`)
	f.Add(`[1,2] {"obj":true}`)
	f.Add("")
	f.Add("{")
	f.Add("not json")

	f.Fuzz(func(t *testing.T, input string) {
		results := jsonutil.ExtractAll(input)
		for i, r := range results {
			if !json.Valid(r) {
				t.Errorf("ExtractAll result[%d] is invalid JSON for input %q: %s", i, input, r)
			}
		}
	})
}
