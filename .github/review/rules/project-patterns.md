# Raven Project Patterns

## Constructor Pattern
```go
func New(opts Options) (*Thing, error) {
    if opts.Required == "" {
        return nil, fmt.Errorf("required field missing")
    }
    return &Thing{...}, nil
}
```

## Interface-Driven Design
Define interfaces where consumers need them. Keep them small (1-3 methods).

## Error Wrapping
```go
if err := doThing(); err != nil {
    return fmt.Errorf("doing thing for %s: %w", name, err)
}
```

## Subprocess Execution
```go
cmd := exec.CommandContext(ctx, "claude", args...)
cmd.Dir = workDir
cmd.Env = append(os.Environ(), extraEnv...)
```

## Bounded Parallelism
```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(concurrency)
for _, item := range items {
    g.Go(func() error { ... })
}
return g.Wait()
```

## TUI Event Pattern
```go
go func() {
    scanner := bufio.NewScanner(stdout)
    for scanner.Scan() {
        p.Send(AgentOutputMsg{Line: scanner.Text()})
    }
}()
```
