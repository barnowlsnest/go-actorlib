# go-actorlib

A lightweight, type-safe actor library for Go implementing the Actor Model pattern. Built with Go's goroutines and channels for high-performance concurrent applications.

## Features

- **Type-Safe**: Generic interfaces ensure compile-time type safety for entities and commands
- **Lightweight**: Minimal overhead using Go's native concurrency primitives
- **Lifecycle Management**: Complete actor lifecycle with hooks for monitoring and supervision
- **Command Pattern**: Built-in command implementation with result channels and error handling
- **Context Support**: Full context.Context integration for cancellation and timeouts
- **Panic Recovery**: Automatic panic recovery with configurable error handling
- **Thread-Safe**: All operations are safe for concurrent use

## Why Actor Model in Go?

Go already provides goroutines, channels, and `sync.Mutex` — so why add an actor abstraction on top?

### Pros

- **No locks, no data races by design.** Each actor owns its state exclusively. There is no shared mutable state to protect, so entire classes of concurrency bugs (deadlocks, forgotten mutexes, lock ordering issues) are eliminated at the structural level rather than by developer discipline.
- **Predictable sequential execution.** Commands sent to an actor are processed one at a time in order. Complex state mutations become simple single-threaded logic — no need to reason about interleaving.
- **Clear ownership boundaries.** The actor is the single source of truth for its entity. This makes it easy to reason about who can read or write a piece of state, even in large codebases.
- **Structured lifecycle.** Built-in start/stop/hooks give you a consistent way to manage resource setup and teardown across many concurrent components, avoiding leaked goroutines and orphaned resources.
- **Natural backpressure.** Bounded input channels signal when a component is overloaded, letting the system push back rather than silently growing unbounded queues.
- **Fault isolation.** A panic in one actor is recovered and contained — it doesn't bring down the entire application or corrupt unrelated state.

### Cons

- **Overhead for trivial concurrency.** If you just need a goroutine-safe counter or a simple fan-out, a `sync.Mutex` or a bare channel is lighter and more idiomatic.
- **Latency from message passing.** Every interaction goes through a channel send and sequential processing. For hot paths that need sub-microsecond shared reads, a `sync.RWMutex` or `sync/atomic` will be faster.
- **Debugging indirection.** Stack traces stop at channel operations. Tracing a request across multiple actors requires correlation IDs or structured logging — the call chain is no longer visible in a single stack.
- **Learning curve.** Developers familiar with Go's stdlib concurrency need to shift from "protect shared state with a lock" to "send a message and wait for a result."

### When to use

- Long-lived stateful components (connection managers, session stores, caches, worker coordinators)
- State that multiple goroutines need to read and mutate, where getting the locking right is error-prone
- Systems where you need structured lifecycle management (graceful startup ordering, coordinated shutdown)
- Domains that naturally decompose into independent entities (game objects, device controllers, per-user/per-tenant state)

### When not to use

- Simple request-scoped concurrency — a `sync.WaitGroup` or `errgroup` is sufficient
- Read-heavy, write-rare shared state — `sync.RWMutex` or `atomic.Value` avoids unnecessary serialization
- Fire-and-forget work — a plain goroutine with a channel is simpler
- CPU-bound parallelism (data processing pipelines) — use worker pools and fan-out/fan-in instead

## Quick Start By Examples

### 1. Define Your Entity

```go
type Counter struct {
    value int
}

func (c *Counter) IsProvidable() bool {
    return true // Entity is ready for use
}
```

### 2. Create a Provider

```go
type CounterProvider struct {
    counter *Counter
}

func (p *CounterProvider) Provide() *Counter {
    return p.counter
}
```

### 3. Create and Start an Actor

```go
// Create the actor with options
myActor, err := actor.New(
    actor.WithProvider(&CounterProvider{&Counter{}}),
    actor.WithInputBufferSize(10),
    actor.WithReceiveTimeout(5*time.Second),
)
if err != nil {
    log.Fatal(err)
}

// Start the actor
ctx := context.Background()
if err := myActor.Start(ctx); err != nil {
    log.Fatal(err)
}

// Wait for the actor to be ready
if err := myActor.WaitReady(ctx, 5*time.Second); err != nil {
    log.Fatal(err)
}
```

### 4. Send Commands

```go
// Create a command that increments the counter
cmd := command.New(func(counter *Counter) (int, error) {
    counter.value++
    return counter.value, nil
})

// Send command to actor
err := actor.Receive(ctx, cmd)
if err != nil {
    log.Fatal(err)
}

// Wait for result
select {
case result := <-cmd.Done():
    fmt.Printf("Counter value: %d\n", result)
case <-time.After(5 * time.Second):
    fmt.Println("Command timeout")
}

// Check for errors
if err := cmd.Error(); err != nil {
    log.Printf("Command failed: %v", err)
}
```

## Architecture

PlantUML diagrams documenting the architecture are available in [`docs/`](./docs/README.md).

## Performance Considerations

- **Buffer Sizes**: Tune input buffer sizes based on message volume
- **Timeouts**: Configure appropriate timeouts for your use case
- **Resource Cleanup**: Always stop actors to prevent goroutine leaks

## Development Commands

This project uses [Task](https://taskfile.dev/) for build automation:

```bash
# Install Task
go install github.com/go-task/task/v3/cmd/task@latest

# Essential commands
task go:build     # Build all packages
task go:test      # Run tests with coverage and benchmarks  
task go:fmt       # Format all Go files
task go:vet       # Run static analysis
task check        # Run complete check suite
```

## Requirements

- Go 1.24.5 or later
- Dependencies managed with Go modules

## License

This project is licensed under the terms specified in the LICENSE file.

## References

- [Actor Model](https://en.wikipedia.org/wiki/Actor_model) - Learn about the Actor Model pattern
- [Go Concurrency Patterns](https://go.dev/blog/pipelines) - Go concurrency best practices
