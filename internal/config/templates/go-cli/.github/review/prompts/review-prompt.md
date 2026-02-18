# Code Review Prompt

Review the following code changes for this project.

## Review Criteria

1. Correctness: Does the code do what it claims?
2. Safety: Are there security vulnerabilities or data races?
3. Style: Does it follow project conventions?
4. Tests: Are there adequate tests?
5. Performance: Any obvious bottlenecks?

## Output Format

Respond with a JSON object:
```json
{
  "summary": "Overall assessment",
  "issues": [
    {"severity": "high|medium|low", "file": "path/to/file.go", "line": 42, "description": "Issue description"}
  ],
  "approved": true
}
```
