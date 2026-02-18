package workflow

import (
	"errors"
	"fmt"
	"sort"
)

// ErrStepNotFound is returned by Registry.Get when no handler is registered
// for the requested step name.
var ErrStepNotFound = errors.New("step handler not found")

// Registry maps step names to their StepHandler implementations. It is used
// by the workflow engine to resolve the concrete handler for each step defined
// in a WorkflowDefinition. Registration is expected to occur at program
// initialization time (single-threaded), so no mutex is needed.
type Registry struct {
	handlers map[string]StepHandler
}

// NewRegistry creates a new, empty Registry ready for handler registration.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]StepHandler),
	}
}

// Register adds handler to the registry, keyed by handler.Name(). It panics
// if handler is nil, if handler.Name() returns an empty string, or if a
// handler with the same name has already been registered. These are all
// programming errors that should be caught at startup.
func (r *Registry) Register(handler StepHandler) {
	if handler == nil {
		panic("workflow: Register called with nil handler")
	}
	name := handler.Name()
	if name == "" {
		panic("workflow: Register called with handler that returns empty name")
	}
	if _, exists := r.handlers[name]; exists {
		panic(fmt.Sprintf("workflow: handler %q is already registered", name))
	}
	r.handlers[name] = handler
}

// Get returns the StepHandler registered under name. It returns ErrStepNotFound
// (wrapped with the step name) if no handler has been registered for name.
func (r *Registry) Get(name string) (StepHandler, error) {
	h, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("step %q: %w", name, ErrStepNotFound)
	}
	return h, nil
}

// Has reports whether a handler is registered under name.
func (r *Registry) Has(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// List returns the names of all registered handlers in alphabetical order.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MustGet returns the handler registered under name, or panics if no handler
// is found. Intended for use in initialization code where a missing handler
// is an unrecoverable programming error.
func (r *Registry) MustGet(name string) StepHandler {
	h, err := r.Get(name)
	if err != nil {
		panic(fmt.Sprintf("workflow: MustGet: %v", err))
	}
	return h
}

// DefaultRegistry is the package-level singleton Registry. Handlers are
// registered into it via the package-level Register function, typically from
// init() functions in packages that provide step implementations.
var DefaultRegistry = NewRegistry()

// Register adds handler to DefaultRegistry. See Registry.Register for
// panic conditions.
func Register(handler StepHandler) { DefaultRegistry.Register(handler) }

// GetHandler returns the handler registered under name in DefaultRegistry.
// Returns ErrStepNotFound if no handler is registered for name.
func GetHandler(name string) (StepHandler, error) { return DefaultRegistry.Get(name) }

// HasHandler reports whether a handler is registered under name in
// DefaultRegistry.
func HasHandler(name string) bool { return DefaultRegistry.Has(name) }

// ListHandlers returns the names of all handlers registered in DefaultRegistry,
// sorted alphabetically.
func ListHandlers() []string { return DefaultRegistry.List() }
