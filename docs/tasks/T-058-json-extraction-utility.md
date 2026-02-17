# T-058: JSON Extraction Utility for Agent Output

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Medium: 6-10hrs |
| Dependencies | None |
| Blocked By | None |
| Blocks | T-057, T-059 |

## Goal
Implement a robust JSON extraction utility that can reliably extract structured JSON from freeform agent output. This utility is shared between the review pipeline (which extracts review findings JSON) and the PRD decomposition pipeline (which extracts epic and task JSON). It handles markdown code fences, partial output, nested JSON, and multiple JSON candidates.

## Background
Per PRD Section 5.5, the review pipeline needs "JSON extraction from freeform agent output using a robust extractor (handles markdown fencing, partial output)." The PRD also notes this is "Go port of the existing json-extract.js logic (regex-based candidate extraction, encoding/json validation)." The same extraction logic is needed for PRD decomposition workers (Section 5.8) where agents produce JSON embedded in their natural language output.

Note: If `internal/review/extract.go` already exists from Phase 3 implementation, this task refactors it into a shared utility in `internal/jsonutil/` that both review and PRD subsystems import.

## Technical Specifications
### Implementation Approach
Create `internal/jsonutil/extract.go` with a `Extract` function that scans text for JSON candidates using multiple strategies: (1) markdown code fence extraction, (2) brace-matching for top-level objects/arrays, (3) progressive truncation for partial output. Each candidate is validated with `encoding/json` and optionally unmarshaled into a target type.

### Key Components
- **Extract**: Main function that returns the first valid JSON object/array from text
- **ExtractAll**: Returns all valid JSON objects/arrays found in text
- **ExtractInto**: Extracts JSON and unmarshals into a provided Go type
- **Candidate strategies**: Ordered list of extraction approaches tried in sequence

### API/Interface Contracts
```go
// internal/jsonutil/extract.go

// Extract returns the first valid JSON object or array found in the text.
// It tries multiple extraction strategies in order of reliability.
func Extract(text string) (json.RawMessage, error)

// ExtractAll returns all valid JSON objects/arrays found in the text.
func ExtractAll(text string) []json.RawMessage

// ExtractInto extracts JSON from text and unmarshals into target.
func ExtractInto(text string, target interface{}) error

// ExtractFromFile reads a file and extracts JSON from its contents.
func ExtractFromFile(path string, target interface{}) error

// Extraction strategies (internal, tried in order):
// 1. Markdown code fence: ```json\n{...}\n``` or ```\n{...}\n```
// 2. Raw JSON detection: find outermost { } or [ ] with brace matching
// 3. Line-by-line accumulation: build JSON by tracking brace depth
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| encoding/json | stdlib | JSON validation and unmarshaling |
| regexp | stdlib | Markdown code fence pattern matching |
| strings | stdlib | Text scanning and manipulation |
| stretchr/testify | v1.9+ | Test assertions |

## Acceptance Criteria
- [ ] Extracts JSON from markdown-fenced code blocks (```json and ```)
- [ ] Extracts top-level JSON objects and arrays from raw text using brace matching
- [ ] Handles nested JSON objects correctly (does not terminate early on inner closing braces)
- [ ] ExtractInto successfully unmarshals into provided Go struct types
- [ ] Returns clear error when no valid JSON is found
- [ ] Handles partial/truncated JSON gracefully (returns error, not panic)
- [ ] Works with both agent stdout text and file contents
- [ ] Unit tests achieve 95% coverage
- [ ] Benchmarks show extraction completes in <10ms for typical agent output (10KB)

## Testing Requirements
### Unit Tests
- Extract JSON from ```json code fence
- Extract JSON from plain ``` code fence
- Extract JSON object from text with surrounding prose
- Extract JSON array from text with surrounding prose
- Handle deeply nested JSON (5+ levels)
- Handle JSON with string values containing braces
- Handle multiple JSON objects in same text (ExtractAll returns all)
- Handle JSON with escaped quotes in strings
- Return error for text with no JSON
- Return error for truncated JSON (missing closing brace)
- ExtractInto with valid target struct succeeds
- ExtractInto with type mismatch returns unmarshal error
- ExtractFromFile with valid file succeeds
- ExtractFromFile with nonexistent file returns error

### Integration Tests
- Extract from realistic agent output samples (canned review findings, epic JSON)

### Edge Cases to Handle
- JSON embedded between multiple markdown fences (first valid wins)
- Agent output with ANSI escape codes mixed in
- Very large JSON (>1MB) -- ensure no excessive memory allocation
- JSON with trailing commas (technically invalid -- should reject)
- JSON with comments (not valid JSON -- should handle gracefully)
- Empty code fences
- Code fences with language tags other than json

## Implementation Notes
### Recommended Approach
1. Try markdown code fence extraction first (most reliable when present)
2. If no fences found, scan for top-level `{` or `[` characters
3. For each candidate start position, use a brace-depth counter to find the matching close
4. Skip braces inside quoted strings (track quote state with escape handling)
5. Validate each candidate with `json.Valid()` before returning
6. For `ExtractInto`, use `json.Unmarshal` with the extracted bytes

### Potential Pitfalls
- Brace matching must handle escaped quotes within JSON strings (e.g., `"value": "he said \"hello\""`)
- Do not use regex alone for JSON extraction -- brace matching is more reliable for nested structures
- Agent output may contain ANSI escape codes that need stripping before extraction
- Some agents prefix JSON with a BOM character -- strip leading whitespace/BOM

### Security Considerations
- Cap input text size to prevent memory exhaustion (configurable, default 10MB)
- Do not execute or eval extracted JSON -- only unmarshal into typed structs

## References
- [PRD Section 5.5 - JSON extraction from agent output](docs/prd/PRD-Raven.md)
- [PRD Section 5.8 - Worker JSON output](docs/prd/PRD-Raven.md)
- [Go encoding/json documentation](https://pkg.go.dev/encoding/json)
