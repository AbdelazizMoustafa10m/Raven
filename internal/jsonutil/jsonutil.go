// Package jsonutil provides utilities for extracting and decoding JSON from
// freeform text output produced by AI agent CLIs.
package jsonutil

import (
	"encoding/json"
	"fmt"
)

// ExtractInto extracts the first valid JSON object from text and decodes it
// into dst. It scans for '{' delimiters, finds balanced JSON objects using
// brace counting, and tries json.Unmarshal on each candidate in order of
// appearance. Returns an error if no valid JSON object that decodes into dst
// is found.
func ExtractInto(text string, dst any) error {
	candidates := collectCandidates(text)
	for _, c := range candidates {
		if err := json.Unmarshal([]byte(c), dst); err == nil {
			return nil
		}
	}
	return fmt.Errorf("jsonutil: no valid JSON object found in text")
}

// ExtractFirst extracts the first valid JSON object string from text.
// Returns ("", false) if no valid JSON object is found.
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
// and end with their matching '}', in order of appearance. Substrings for
// nested objects are also included as independent candidates.
func collectCandidates(text string) []string {
	var results []string
	n := len(text)

	for i := 0; i < n; i++ {
		if text[i] != '{' {
			continue
		}

		// Found an opening brace. Walk forward counting braces, respecting
		// strings and escape sequences, to find the matching closing brace.
		end := matchingBrace(text, i)
		if end < 0 {
			// Unbalanced brace — skip this position.
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
