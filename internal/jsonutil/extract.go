package jsonutil

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// maxInputBytes is the maximum number of bytes we will process. Inputs larger
// than this limit are rejected with an error to prevent memory exhaustion.
const maxInputBytes = 10 * 1024 * 1024 // 10 MB

// reANSI matches ANSI escape codes (CSI sequences) that AI CLIs may embed in
// their output. We strip these before attempting JSON extraction.
var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHF]`)

// reCodeFence matches a markdown code fence block that optionally carries a
// "json" language tag (or no tag). The content between the opening and closing
// fences is captured in subgroup 1.  The (?s) flag enables dot-all mode so
// that .*? matches newlines; the non-greedy quantifier stops at the first
// closing fence, allowing multiple fences in the same text.
var reCodeFence = regexp.MustCompile("(?s)```(?:json)?[ \\t]*\n(.*?)\n```")

// sanitize strips ANSI escape codes and a leading UTF-8 BOM from text, then
// enforces the 10 MB cap. The cleaned string is returned together with any
// error.
func sanitize(text string) (string, error) {
	if len(text) > maxInputBytes {
		return "", fmt.Errorf("jsonutil: input exceeds maximum size of %d bytes", maxInputBytes)
	}
	// Strip leading UTF-8 BOM (U+FEFF encoded as EF BB BF).
	text = strings.TrimPrefix(text, "\xef\xbb\xbf")
	// Strip ANSI escape codes.
	text = reANSI.ReplaceAllString(text, "")
	return text, nil
}

// Extract returns the first valid JSON object or array found in text.
// Multiple extraction strategies are tried in order of reliability:
//  1. Markdown code fence (```json or ```)
//  2. Brace/bracket matching for top-level { } and [ ] structures
//
// An error is returned when no valid JSON is found or the input exceeds 10 MB.
func Extract(text string) (json.RawMessage, error) {
	text, err := sanitize(text)
	if err != nil {
		return nil, err
	}

	all := extractAllFrom(text)
	if len(all) == 0 {
		return nil, fmt.Errorf("jsonutil: no valid JSON found in text")
	}
	return all[0], nil
}

// ExtractAll returns all valid JSON objects and arrays found in text, in order
// of appearance. Strategies are applied and results deduplicated by byte
// offset so that the same JSON span is not returned twice.
func ExtractAll(text string) []json.RawMessage {
	cleaned, err := sanitize(text)
	if err != nil {
		return nil
	}
	return extractAllFrom(cleaned)
}

// ExtractInto extracts JSON from text and unmarshals it into target. It
// delegates to Extract using the multi-strategy approach (code fences, then
// brace matching) and is backward compatible with callers that previously used
// the brace-only implementation.
func ExtractInto(text string, target interface{}) error {
	raw, err := Extract(text)
	if err != nil {
		return fmt.Errorf("jsonutil: no valid JSON object found in text")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("jsonutil: unmarshal failed: %w", err)
	}
	return nil
}

// ExtractFromFile reads the file at path and calls ExtractInto to unmarshal
// the first valid JSON found in its contents into target.
func ExtractFromFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("jsonutil: reading file %q: %w", path, err)
	}
	return ExtractInto(string(data), target)
}

// fenceSpan records the byte range [start, end) of a code fence match in the
// original text. Any brace-matched candidate that starts within this span is
// considered a duplicate of the fence content and is suppressed.
type fenceSpan struct{ start, end int }

// extractAllFrom applies all extraction strategies to the pre-sanitized text
// and returns all unique valid JSON values in order of appearance.
func extractAllFrom(text string) []json.RawMessage {
	var results []json.RawMessage
	var fences []fenceSpan

	// Strategy 1: markdown code fences -- highest reliability when present.
	for _, loc := range reCodeFence.FindAllStringSubmatchIndex(text, -1) {
		if len(loc) < 4 {
			continue
		}
		inner := strings.TrimSpace(text[loc[2]:loc[3]])
		if inner == "" {
			continue
		}
		if !json.Valid([]byte(inner)) {
			continue
		}
		// Record the byte range of the entire fence match so that the
		// brace-matching strategy can skip any position that falls inside a
		// fence we already processed.
		fences = append(fences, fenceSpan{loc[0], loc[1]})
		results = append(results, json.RawMessage(inner))
	}

	// Strategy 2: brace/bracket matching for top-level { } and [ ] structures.
	n := len(text)
	for i := 0; i < n; i++ {
		ch := text[i]
		if ch != '{' && ch != '[' {
			continue
		}
		// Skip positions that fall within a code fence already processed.
		if inAnyFence(i, fences) {
			continue
		}
		end := matchingDelimiter(text, i)
		if end < 0 {
			continue
		}
		candidate := text[i : end+1]
		if !json.Valid([]byte(candidate)) {
			continue
		}
		results = append(results, json.RawMessage(candidate))
	}

	return results
}

// inAnyFence reports whether position pos falls within the byte range of any
// recorded fence span.
func inAnyFence(pos int, fences []fenceSpan) bool {
	for _, f := range fences {
		if pos >= f.start && pos < f.end {
			return true
		}
	}
	return false
}

// matchingDelimiter returns the index of the closing delimiter that closes the
// opening delimiter ('{' → '}', '[' → ']') at position start in text. It
// returns -1 when no matching closer is found. The function handles:
//   - nested delimiters of the same type
//   - double-quoted strings (so braces/brackets inside strings are ignored)
//   - escape sequences inside strings (\" and \\)
func matchingDelimiter(text string, start int) int {
	openCh := text[start]
	var closeCh byte
	switch openCh {
	case '{':
		closeCh = '}'
	case '[':
		closeCh = ']'
	default:
		return -1
	}

	depth := 0
	inString := false
	n := len(text)

	for i := start; i < n; i++ {
		ch := text[i]

		if inString {
			switch ch {
			case '\\':
				i++ // skip the escaped character
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case openCh:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}
