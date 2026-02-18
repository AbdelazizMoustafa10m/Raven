// Package jsonutil provides utilities for extracting and decoding JSON from
// freeform text output produced by AI agent CLIs.
//
// The primary API is in extract.go: Extract, ExtractAll, ExtractInto, and
// ExtractFromFile. This file retains ExtractFirst for backward compatibility
// with callers that need the original object-only (no arrays) extraction
// behaviour.
package jsonutil

import "encoding/json"

// ExtractFirst extracts the first valid JSON object string from text. It only
// matches JSON objects (starting with '{') and not arrays. Returns ("", false)
// if no valid JSON object is found.
//
// Deprecated: prefer Extract which also handles JSON arrays and applies
// multi-strategy extraction (markdown code fences, ANSI stripping, BOM
// removal). ExtractFirst is retained for backward compatibility.
func ExtractFirst(text string) (string, bool) {
	candidates := collectCandidates(text)
	for _, c := range candidates {
		var probe any
		if err := json.Unmarshal([]byte(c), &probe); err == nil {
			return c, true
		}
	}
	return "", false
}

// collectCandidates scans text for '{' characters and for each opening brace
// finds the matching closing brace using brace counting (respecting quoted
// strings and escape sequences). It returns all substrings that start with '{'
// and end with their matching '}', in order of appearance.
func collectCandidates(text string) []string {
	var results []string
	n := len(text)

	for i := 0; i < n; i++ {
		if text[i] != '{' {
			continue
		}

		end := matchingBrace(text, i)
		if end < 0 {
			continue
		}

		results = append(results, text[i:end+1])
	}

	return results
}

// matchingBrace returns the index of the '}' that closes the '{' at position
// start in text. It returns -1 if no matching brace is found before end of
// text. The function handles nested braces, double-quoted strings, and escape
// sequences (including \" inside strings).
func matchingBrace(text string, start int) int {
	depth := 0
	inString := false
	n := len(text)

	for i := start; i < n; i++ {
		ch := text[i]

		if inString {
			switch ch {
			case '\\':
				// Escape sequence: skip the next character unconditionally.
				i++
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1 // unbalanced — no matching closing brace found
}
