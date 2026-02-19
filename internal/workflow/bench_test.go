package workflow

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// BenchmarkStateStoreSave measures the throughput of writing a minimal
// WorkflowState (no step history) to disk via an atomic write+rename.
func BenchmarkStateStoreSave(b *testing.B) {
	store, err := NewStateStore(b.TempDir())
	if err != nil {
		b.Fatalf("NewStateStore: %v", err)
	}
	state := NewWorkflowState("bench-save-001", "implement", "init")

	b.ResetTimer()
	for b.Loop() {
		if err := store.Save(state); err != nil {
			b.Fatalf("Save: %v", err)
		}
	}
}

// BenchmarkStateStoreSaveWithHistory measures Save throughput when the state
// carries 10 completed step records, which increases JSON payload size.
func BenchmarkStateStoreSaveWithHistory(b *testing.B) {
	store, err := NewStateStore(b.TempDir())
	if err != nil {
		b.Fatalf("NewStateStore: %v", err)
	}

	state := NewWorkflowState("bench-save-hist-001", "implement", "review")
	now := time.Now()
	for i := range 10 {
		state.AddStepRecord(StepRecord{
			Step:      fmt.Sprintf("step-%d", i),
			Event:     EventSuccess,
			StartedAt: now.Add(time.Duration(i) * time.Second),
			Duration:  500 * time.Millisecond,
		})
	}

	b.ResetTimer()
	for b.Loop() {
		if err := store.Save(state); err != nil {
			b.Fatalf("Save: %v", err)
		}
	}
}

// BenchmarkStateStoreLoad measures the throughput of reading and unmarshalling
// a persisted WorkflowState from disk. A state is saved once in setup and then
// loaded in every iteration.
func BenchmarkStateStoreLoad(b *testing.B) {
	store, err := NewStateStore(b.TempDir())
	if err != nil {
		b.Fatalf("NewStateStore: %v", err)
	}

	state := NewWorkflowState("bench-load-001", "implement", StepDone)
	if err := store.Save(state); err != nil {
		b.Fatalf("Save (setup): %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		if _, err := store.Load("bench-load-001"); err != nil {
			b.Fatalf("Load: %v", err)
		}
	}
}

// BenchmarkStateStoreList measures the throughput of listing all persisted runs.
// Sub-benchmarks vary the number of pre-seeded states (5, 10, 20).
func BenchmarkStateStoreList(b *testing.B) {
	counts := []int{5, 10, 20}
	for _, n := range counts {
		n := n
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			store, err := NewStateStore(b.TempDir())
			if err != nil {
				b.Fatalf("NewStateStore: %v", err)
			}

			for i := range n {
				s := NewWorkflowState(fmt.Sprintf("run-%03d", i), "implement", StepDone)
				if err := store.Save(s); err != nil {
					b.Fatalf("Save (setup, i=%d): %v", i, err)
				}
			}

			b.ResetTimer()
			for b.Loop() {
				if _, err := store.List(); err != nil {
					b.Fatalf("List: %v", err)
				}
			}
		})
	}
}

// BenchmarkWorkflowStateMarshal measures pure JSON marshalling of a WorkflowState
// without any file I/O, isolating serialisation cost from disk latency.
func BenchmarkWorkflowStateMarshal(b *testing.B) {
	state := NewWorkflowState("bench-marshal-001", "pipeline", "review")
	now := time.Now()
	for i := range 10 {
		state.AddStepRecord(StepRecord{
			Step:      fmt.Sprintf("step-%d", i),
			Event:     EventSuccess,
			StartedAt: now.Add(time.Duration(i) * time.Second),
			Duration:  250 * time.Millisecond,
		})
	}
	state.Metadata["phase"] = "implementation"
	state.Metadata["task"] = "T-001"

	b.ResetTimer()
	for b.Loop() {
		if _, err := json.Marshal(state); err != nil {
			b.Fatalf("json.Marshal: %v", err)
		}
	}
}
