# T-044: Step Handler Registry

## Metadata
| Field | Value |
|-------|-------|
| Priority | Must Have |
| Estimated Effort | Small: 2-4hrs |
| Dependencies | T-043 |
| Blocked By | T-043 |
| Blocks | T-045, T-049, T-050, T-051 |

## Goal
Implement a global step handler registry that maps step names to their handler implementations. The registry allows built-in handlers to register at init time and is queried by the workflow engine at runtime to resolve step names to executable handlers. This is the extension point that makes the workflow engine generic.

## Background
Per PRD Section 5.1, "Step handlers are registered in a global registry; built-in handlers are registered at init, custom handlers loaded from plugins (v2.1)." The registry pattern decouples workflow definitions (which reference steps by name) from step implementations (which are Go types implementing the `StepHandler` interface). The workflow engine uses the registry to look up the handler for each step before execution.

## Technical Specifications
### Implementation Approach
Create `internal/workflow/registry.go` containing a `Registry` struct with a `map[string]StepHandler`. Provide `Register`, `Get`, `Has`, and `List` methods. Use a package-level default registry instance for convenience, but allow creating isolated registries for testing. Registration is not goroutine-safe at init time (single-threaded), but `Get` must be safe for concurrent reads after initialization.

### Key Components
- **Registry**: Holds the mapping of step names to StepHandler implementations
- **DefaultRegistry**: Package-level instance used by the engine unless overridden
- **Register function**: Registers a handler by name, panics on duplicate registration (programming error)
- **Get function**: Returns a handler by name, returns error if not found

### API/Interface Contracts
```go
// internal/workflow/registry.go

// Registry maps step names to their StepHandler implementations.
type Registry struct {
    handlers map[string]StepHandler
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry

// Register adds a step handler to the registry.
// Panics if a handler with the same name is already registered (programming error).
func (r *Registry) Register(handler StepHandler)

// Get returns the handler for the given step name.
// Returns ErrStepNotFound if no handler is registered for the name.
func (r *Registry) Get(name string) (StepHandler, error)

// Has returns true if a handler is registered for the given step name.
func (r *Registry) Has(name string) bool

// List returns the names of all registered handlers, sorted alphabetically.
func (r *Registry) List() []string

// MustGet returns the handler or panics. Used in initialization code.
func (r *Registry) MustGet(name string) StepHandler

// Package-level convenience functions using DefaultRegistry.
var DefaultRegistry = NewRegistry()

func Register(handler StepHandler) { DefaultRegistry.Register(handler) }
func GetHandler(name string) (StepHandler, error) { return DefaultRegistry.Get(name) }
func HasHandler(name string) bool { return DefaultRegistry.Has(name) }
func ListHandlers() []string { return DefaultRegistry.List() }

// ErrStepNotFound is returned when a step name is not in the registry.
var ErrStepNotFound = errors.New("step handler not found")
```

### Dependencies & Versions
| Package/Library | Version | Purpose |
|-----------------|---------|---------|
| internal/workflow (T-043) | - | StepHandler interface |
| sort | stdlib | Alphabetical listing |
| errors | stdlib | Sentinel error |
| sync | stdlib | (future) read-write mutex for concurrent access |

## Acceptance Criteria
- [ ] Registry stores handlers keyed by StepHandler.Name()
- [ ] Register panics on duplicate name registration
- [ ] Get returns handler and nil error for registered names
- [ ] Get returns nil and ErrStepNotFound for unregistered names
- [ ] Has correctly reports registered/unregistered status
- [ ] List returns sorted handler names
- [ ] DefaultRegistry is available as a package-level singleton
- [ ] Package-level convenience functions delegate to DefaultRegistry
- [ ] Unit tests achieve 95% coverage
- [ ] Tests use isolated registries (not DefaultRegistry) to avoid cross-test pollution

## Testing Requirements
### Unit Tests
- Register and Get a handler: succeeds
- Register duplicate name: panics
- Get unregistered name: returns ErrStepNotFound
- Has for registered name: returns true
- Has for unregistered name: returns false
- List with 0 handlers: returns empty slice
- List with 3 handlers: returns sorted names
- MustGet with registered name: returns handler
- MustGet with unregistered name: panics
- Register multiple handlers, verify all retrievable

### Integration Tests
- None (integration tested via T-045 engine tests)

### Edge Cases to Handle
- Empty string as handler name (should be rejected or handled gracefully)
- Handler whose Name() returns different values on different calls (should use name at registration time)
- Nil handler registration (should panic with clear message)

## Implementation Notes
### Recommended Approach
1. Create `internal/workflow/registry.go`
2. Use a simple `map[string]StepHandler` -- no need for sync.RWMutex in v2.0 since registration happens at init and reads happen at runtime (single-writer/multi-reader after init)
3. Registration uses `handler.Name()` as the key
4. Add validation: reject empty names, nil handlers
5. Use `recover()` in test helpers to verify panic behavior

### Potential Pitfalls
- Do not use sync.RWMutex unless actually needed -- registration at init time is single-threaded in Go
- Do not export the internal map -- consumers must use Get/Has/List
- Test with fresh registries per test case to avoid cross-test contamination from DefaultRegistry

### Security Considerations
- None specific to this task (internal registry, not exposed to user input)

## References
- [PRD Section 5.1 - Step handler registry](docs/prd/PRD-Raven.md)
- [Go patterns: service registry](https://refactoring.guru/design-patterns/registry)
- [Go init() function documentation](https://go.dev/doc/effective_go#init)