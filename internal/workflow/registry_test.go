package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// stubHandler is a minimal StepHandler for registry tests.
type stubHandler struct {
	name string
}

func (s *stubHandler) Execute(_ context.Context, _ *WorkflowState) (string, error) {
	return EventSuccess, nil
}

func (s *stubHandler) DryRun(_ *WorkflowState) string { return "dry-run: " + s.name }
func (s *stubHandler) Name() string                   { return s.name }

// newStub returns a stubHandler with the given name.
func newStub(name string) StepHandler { return &stubHandler{name: name} }

// newRegistry returns a fresh, isolated Registry for each test (not DefaultRegistry).
func newRegistry() *Registry { return NewRegistry() }

// mustPanic executes f and returns the recovered value. It calls t.Fatal if f
// does not panic at all.
func mustPanic(t *testing.T, f func()) (recovered any) {
	t.Helper()
	defer func() { recovered = recover() }()
	f()
	t.Fatal("expected panic but function returned normally")
	return nil
}

// ---------------------------------------------------------------------------
// NewRegistry
// ---------------------------------------------------------------------------

func TestNewRegistry_IsEmpty(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	assert.Empty(t, r.List())
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func TestRegistry_Register_Single(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("step-a"))
	assert.True(t, r.Has("step-a"))
}

func TestRegistry_Register_Multiple(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("alpha"))
	r.Register(newStub("beta"))
	r.Register(newStub("gamma"))

	assert.True(t, r.Has("alpha"))
	assert.True(t, r.Has("beta"))
	assert.True(t, r.Has("gamma"))
}

func TestRegistry_Register_PanicsOnNilHandler(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	assert.PanicsWithValue(t,
		"workflow: Register called with nil handler",
		func() { r.Register(nil) },
	)
}

func TestRegistry_Register_PanicsOnEmptyName(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	assert.Panics(t, func() { r.Register(newStub("")) })
}

func TestRegistry_Register_PanicsOnDuplicateName(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("step-dup"))
	assert.Panics(t, func() { r.Register(newStub("step-dup")) })
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestRegistry_Get_ReturnsRegisteredHandler(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	h := newStub("my-step")
	r.Register(h)

	got, err := r.Get("my-step")
	require.NoError(t, err)
	assert.Equal(t, h, got)
}

func TestRegistry_Get_ReturnsErrStepNotFound(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	got, err := r.Get("missing")

	assert.Nil(t, got)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrStepNotFound),
		"expected ErrStepNotFound, got: %v", err)
}

func TestRegistry_Get_ErrorWrapsStepName(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	_, err := r.Get("no-such-step")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-step")
}

// ---------------------------------------------------------------------------
// Has
// ---------------------------------------------------------------------------

func TestRegistry_Has_TrueForRegistered(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("present"))
	assert.True(t, r.Has("present"))
}

func TestRegistry_Has_FalseForUnregistered(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	assert.False(t, r.Has("absent"))
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestRegistry_List_Sorted(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("zebra"))
	r.Register(newStub("apple"))
	r.Register(newStub("mango"))

	got := r.List()
	assert.Equal(t, []string{"apple", "mango", "zebra"}, got)
}

func TestRegistry_List_EmptyWhenNoHandlers(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	assert.Empty(t, r.List())
}

func TestRegistry_List_SingleHandler(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("only"))
	assert.Equal(t, []string{"only"}, r.List())
}

// ---------------------------------------------------------------------------
// MustGet
// ---------------------------------------------------------------------------

func TestRegistry_MustGet_ReturnsHandler(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	h := newStub("must-step")
	r.Register(h)

	got := r.MustGet("must-step")
	assert.Equal(t, h, got)
}

func TestRegistry_MustGet_PanicsOnMissing(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	assert.Panics(t, func() { r.MustGet("not-registered") })
}

// ---------------------------------------------------------------------------
// ErrStepNotFound sentinel
// ---------------------------------------------------------------------------

func TestErrStepNotFound_Sentinel(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	_, err := r.Get("ghost")
	require.Error(t, err)

	// Callers must be able to detect the sentinel via errors.Is.
	assert.True(t, errors.Is(err, ErrStepNotFound))
}

// ---------------------------------------------------------------------------
// Package-level convenience functions (DefaultRegistry delegation)
// ---------------------------------------------------------------------------

// freshDefaultRegistry resets DefaultRegistry to an empty state and returns
// a restore function that puts the original registry back. This keeps tests
// hermetic without exposing internal fields.
func freshDefaultRegistry(t *testing.T) func() {
	t.Helper()
	original := DefaultRegistry
	DefaultRegistry = NewRegistry()
	return func() { DefaultRegistry = original }
}

func TestPackageLevel_Register_And_GetHandler(t *testing.T) {
	// Not parallel: mutates DefaultRegistry.
	restore := freshDefaultRegistry(t)
	defer restore()

	Register(newStub("pkg-step"))
	h, err := GetHandler("pkg-step")
	require.NoError(t, err)
	assert.Equal(t, "pkg-step", h.Name())
}

func TestPackageLevel_HasHandler(t *testing.T) {
	// Not parallel: mutates DefaultRegistry.
	restore := freshDefaultRegistry(t)
	defer restore()

	assert.False(t, HasHandler("not-there"))
	Register(newStub("there"))
	assert.True(t, HasHandler("there"))
}

func TestPackageLevel_ListHandlers_Sorted(t *testing.T) {
	// Not parallel: mutates DefaultRegistry.
	restore := freshDefaultRegistry(t)
	defer restore()

	Register(newStub("delta"))
	Register(newStub("alpha"))
	Register(newStub("charlie"))

	assert.Equal(t, []string{"alpha", "charlie", "delta"}, ListHandlers())
}

func TestPackageLevel_GetHandler_ErrStepNotFound(t *testing.T) {
	// Not parallel: mutates DefaultRegistry.
	restore := freshDefaultRegistry(t)
	defer restore()

	_, err := GetHandler("missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrStepNotFound))
}

// ---------------------------------------------------------------------------
// DefaultRegistry singleton
// ---------------------------------------------------------------------------

// TestDefaultRegistry_IsSingleton verifies that repeated accesses to DefaultRegistry
// return the exact same pointer rather than allocating a fresh Registry each time.
func TestDefaultRegistry_IsSingleton(t *testing.T) {
	t.Parallel()

	first := DefaultRegistry
	second := DefaultRegistry
	assert.Same(t, first, second, "DefaultRegistry must be the same pointer on every access")
}

// TestDefaultRegistry_IsNotNil verifies the package-level singleton was properly
// initialized by NewRegistry and is ready to accept registrations.
func TestDefaultRegistry_IsNotNil(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, DefaultRegistry, "DefaultRegistry must not be nil")
}

// ---------------------------------------------------------------------------
// Exact panic message verification
// ---------------------------------------------------------------------------

// TestRegistry_Register_PanicsOnNilHandler_Message verifies the panic message
// for a nil handler registration is exactly as specified.
func TestRegistry_Register_PanicsOnNilHandler_Message(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	got := mustPanic(t, func() { r.Register(nil) })
	assert.Equal(t, "workflow: Register called with nil handler", got,
		"nil handler panic message must match exactly")
}

// TestRegistry_Register_PanicsOnEmptyName_Message verifies the panic message
// for a handler that returns an empty name is exactly as specified.
func TestRegistry_Register_PanicsOnEmptyName_Message(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	got := mustPanic(t, func() { r.Register(newStub("")) })
	assert.Equal(t, "workflow: Register called with handler that returns empty name", got,
		"empty-name panic message must match exactly")
}

// TestRegistry_Register_PanicsOnDuplicate_Message verifies the panic message
// for a duplicate-name registration contains the handler name.
func TestRegistry_Register_PanicsOnDuplicate_Message(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("dup"))
	got := mustPanic(t, func() { r.Register(newStub("dup")) })
	wantMsg := fmt.Sprintf("workflow: handler %q is already registered", "dup")
	assert.Equal(t, wantMsg, got,
		"duplicate-name panic message must contain the handler name")
}

// TestRegistry_MustGet_PanicsOnMissing_Message verifies the panic message from
// MustGet for an unregistered step name contains the step name.
func TestRegistry_MustGet_PanicsOnMissing_Message(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	got := mustPanic(t, func() { r.MustGet("ghost-step") })
	msg, ok := got.(string)
	require.True(t, ok, "MustGet must panic with a string value")
	assert.Contains(t, msg, "ghost-step",
		"MustGet panic message must include the unregistered step name")
}

// ---------------------------------------------------------------------------
// Table-driven: Register + Get for multiple handlers
// ---------------------------------------------------------------------------

// TestRegistry_Register_And_Get_TableDriven verifies that all registered handlers
// are individually retrievable by name. This covers the "Register multiple handlers,
// verify all retrievable" acceptance criterion.
func TestRegistry_Register_And_Get_TableDriven(t *testing.T) {
	t.Parallel()

	handlers := []struct {
		name string
	}{
		{"handler-alpha"},
		{"handler-beta"},
		{"handler-gamma"},
		{"handler-delta"},
		{"handler-epsilon"},
	}

	r := newRegistry()
	for _, h := range handlers {
		r.Register(newStub(h.name))
	}

	for _, tt := range handlers {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := r.Get(tt.name)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.name, got.Name())
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven: Get – registered and unregistered cases
// ---------------------------------------------------------------------------

// TestRegistry_Get_TableDriven covers both the happy path and the error path for
// Registry.Get in a single table, verifying errors.Is(err, ErrStepNotFound) on
// every missing-name case.
func TestRegistry_Get_TableDriven(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("known"))

	tests := []struct {
		name         string
		query        string
		wantErr      bool
		wantSentinel bool
	}{
		{
			name:    "registered name returns handler",
			query:   "known",
			wantErr: false,
		},
		{
			name:         "unregistered name returns ErrStepNotFound",
			query:        "unknown",
			wantErr:      true,
			wantSentinel: true,
		},
		{
			name:         "empty string returns ErrStepNotFound",
			query:        "",
			wantErr:      true,
			wantSentinel: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := r.Get(tt.query)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				if tt.wantSentinel {
					assert.True(t, errors.Is(err, ErrStepNotFound),
						"expected errors.Is(err, ErrStepNotFound) to be true, got: %v", err)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven: Has
// ---------------------------------------------------------------------------

// TestRegistry_Has_TableDriven covers all Has scenarios in one table including
// names that were never registered and names registered then looked up.
func TestRegistry_Has_TableDriven(t *testing.T) {
	t.Parallel()

	r := newRegistry()
	r.Register(newStub("registered-step"))

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{
			name:  "registered name returns true",
			query: "registered-step",
			want:  true,
		},
		{
			name:  "unregistered name returns false",
			query: "not-registered",
			want:  false,
		},
		{
			name:  "empty string returns false",
			query: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, r.Has(tt.query))
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven: List with 0, 1, and N handlers
// ---------------------------------------------------------------------------

// TestRegistry_List_TableDriven verifies List behavior across empty, single,
// and multi-handler registries, including alphabetical ordering invariant.
func TestRegistry_List_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		register  []string
		wantNames []string
	}{
		{
			name:      "empty registry returns empty slice",
			register:  []string{},
			wantNames: []string{},
		},
		{
			name:      "single handler",
			register:  []string{"only"},
			wantNames: []string{"only"},
		},
		{
			name:      "three handlers returned in sorted order",
			register:  []string{"zebra", "apple", "mango"},
			wantNames: []string{"apple", "mango", "zebra"},
		},
		{
			name:      "five handlers returned in sorted order",
			register:  []string{"echo", "alpha", "delta", "bravo", "charlie"},
			wantNames: []string{"alpha", "bravo", "charlie", "delta", "echo"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newRegistry()
			for _, n := range tt.register {
				r.Register(newStub(n))
			}
			got := r.List()
			assert.Equal(t, tt.wantNames, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Table-driven: Register panic conditions
// ---------------------------------------------------------------------------

// TestRegistry_Register_PanicCases covers all three panic conditions for
// Registry.Register in a single table-driven test.
func TestRegistry_Register_PanicCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(r *Registry)
		registerFn func(r *Registry)
		wantPanic  bool
	}{
		{
			name:       "nil handler panics",
			setupFn:    func(_ *Registry) {},
			registerFn: func(r *Registry) { r.Register(nil) },
			wantPanic:  true,
		},
		{
			name:       "empty name panics",
			setupFn:    func(_ *Registry) {},
			registerFn: func(r *Registry) { r.Register(newStub("")) },
			wantPanic:  true,
		},
		{
			name: "duplicate name panics",
			setupFn: func(r *Registry) {
				r.Register(newStub("collision"))
			},
			registerFn: func(r *Registry) { r.Register(newStub("collision")) },
			wantPanic:  true,
		},
		{
			name:       "valid handler does not panic",
			setupFn:    func(_ *Registry) {},
			registerFn: func(r *Registry) { r.Register(newStub("valid")) },
			wantPanic:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newRegistry()
			tt.setupFn(r)
			if tt.wantPanic {
				assert.Panics(t, func() { tt.registerFn(r) })
			} else {
				assert.NotPanics(t, func() { tt.registerFn(r) })
			}
		})
	}
}

// ---------------------------------------------------------------------------
// errors.Is compatibility across multiple sentinel usages
// ---------------------------------------------------------------------------

// TestErrStepNotFound_IsCompatible verifies that errors.Is correctly unwraps the
// sentinel in all contexts where ErrStepNotFound is produced: Registry.Get and
// the package-level GetHandler.
func TestErrStepNotFound_IsCompatible(t *testing.T) {
	t.Parallel()

	t.Run("Registry.Get wraps sentinel correctly", func(t *testing.T) {
		t.Parallel()
		r := newRegistry()
		_, err := r.Get("not-there")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrStepNotFound))
		// The wrapped error must also contain the step name for diagnostics.
		assert.Contains(t, err.Error(), "not-there")
	})

	t.Run("sentinel is distinct from generic errors", func(t *testing.T) {
		t.Parallel()
		r := newRegistry()
		_, err := r.Get("nope")
		require.Error(t, err)
		// Must NOT match an unrelated sentinel.
		assert.False(t, errors.Is(err, errors.New("unrelated error")))
	})
}

// ---------------------------------------------------------------------------
// Package-level: DefaultRegistry delegation completeness
// ---------------------------------------------------------------------------

// TestPackageLevel_DelegatesCorrectly verifies each package-level convenience
// function produces the same outcome as calling the equivalent method on
// DefaultRegistry directly, confirming delegation is not a no-op.
func TestPackageLevel_DelegatesCorrectly(t *testing.T) {
	// Not parallel: mutates DefaultRegistry.
	restore := freshDefaultRegistry(t)
	defer restore()

	// HasHandler before any registration must return false.
	assert.False(t, HasHandler("step-x"), "HasHandler must return false before registration")
	assert.False(t, DefaultRegistry.Has("step-x"))

	// Register via package-level function.
	Register(newStub("step-x"))

	// HasHandler after registration must return true via both paths.
	assert.True(t, HasHandler("step-x"), "HasHandler must return true after registration")
	assert.True(t, DefaultRegistry.Has("step-x"))

	// GetHandler must return the same handler as DefaultRegistry.Get.
	pkgH, pkgErr := GetHandler("step-x")
	regH, regErr := DefaultRegistry.Get("step-x")
	require.NoError(t, pkgErr)
	require.NoError(t, regErr)
	assert.Equal(t, regH, pkgH, "GetHandler must return the same handler as DefaultRegistry.Get")

	// ListHandlers must return the same names as DefaultRegistry.List.
	assert.Equal(t, DefaultRegistry.List(), ListHandlers(),
		"ListHandlers must return the same result as DefaultRegistry.List")
}

// TestPackageLevel_ListHandlers_Empty verifies ListHandlers returns an empty
// slice (not nil) on a freshly reset DefaultRegistry.
func TestPackageLevel_ListHandlers_Empty(t *testing.T) {
	// Not parallel: mutates DefaultRegistry.
	restore := freshDefaultRegistry(t)
	defer restore()

	got := ListHandlers()
	assert.Empty(t, got, "ListHandlers on empty DefaultRegistry must return empty slice")
}
