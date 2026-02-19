package agent

import (
	"context"
	"fmt"
	"testing"

	"golang.org/x/sync/errgroup"
)

// BenchmarkMockAgentRun measures the throughput of a single MockAgent.Run call.
// This establishes a baseline for pure mock overhead with no I/O.
func BenchmarkMockAgentRun(b *testing.B) {
	a := NewMockAgent("claude")
	ctx := context.Background()
	opts := RunOpts{Prompt: "implement the feature described in the task spec"}

	b.ResetTimer()
	for b.Loop() {
		_, _ = a.Run(ctx, opts)
	}
}

// benchmarkConcurrentAgents is the shared helper that launches n goroutines
// each calling agent.Run once per benchmark iteration, waits for all to finish,
// then moves to the next iteration. It uses errgroup for bounded parallelism.
func benchmarkConcurrentAgents(b *testing.B, n int) {
	b.Helper()

	agents := make([]*MockAgent, n)
	for i := range agents {
		agents[i] = NewMockAgent(fmt.Sprintf("agent-%d", i))
	}

	ctx := context.Background()
	opts := RunOpts{Prompt: "implement the feature described in the task spec"}

	b.ResetTimer()
	for b.Loop() {
		g, gctx := errgroup.WithContext(ctx)
		for _, a := range agents {
			a := a
			g.Go(func() error {
				_, err := a.Run(gctx, opts)
				return err
			})
		}
		_ = g.Wait()
	}
}

// BenchmarkConcurrentAgents1 measures coordination overhead with 1 agent.
// This isolates errgroup setup cost from actual concurrency.
func BenchmarkConcurrentAgents1(b *testing.B) {
	benchmarkConcurrentAgents(b, 1)
}

// BenchmarkConcurrentAgents3 measures coordination overhead with 3 agents
// running concurrently — a typical review pipeline scenario.
func BenchmarkConcurrentAgents3(b *testing.B) {
	benchmarkConcurrentAgents(b, 3)
}

// BenchmarkConcurrentAgents5 measures coordination overhead with 5 agents
// running concurrently.
func BenchmarkConcurrentAgents5(b *testing.B) {
	benchmarkConcurrentAgents(b, 5)
}

// BenchmarkConcurrentAgents10 measures coordination overhead with 10 agents
// running concurrently — a stress scenario for the errgroup scheduler.
func BenchmarkConcurrentAgents10(b *testing.B) {
	benchmarkConcurrentAgents(b, 10)
}

// BenchmarkAgentRunParallel measures MockAgent.Run throughput under Go's
// built-in b.RunParallel scheduler, where GOMAXPROCS goroutines call Run
// simultaneously without coordination overhead between iterations.
func BenchmarkAgentRunParallel(b *testing.B) {
	a := NewMockAgent("claude")
	ctx := context.Background()
	opts := RunOpts{Prompt: "implement the feature described in the task spec"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = a.Run(ctx, opts)
		}
	})
}
