# go-actorlib

A lightweight, type-safe actor library for Go implementing the Actor Model pattern. Built with Go's goroutines and channels for high-performance concurrent applications.

## Features

- **Type-Safe**: Generic interfaces ensure compile-time type safety for entities and commands
- **Lightweight**: Minimal overhead using Go's native concurrency primitives
- **Lifecycle Management**: Complete actor lifecycle with hooks for monitoring and supervision
- **Command Pattern**: Built-in command implementation with result channels and error handling
- **Registry System**: Centralized actor management with concurrent startup/shutdown
- **Context Support**: Full context.Context integration for cancellation and timeouts
- **Panic Recovery**: Automatic panic recovery with configurable error handling
- **Thread-Safe**: All operations are safe for concurrent use

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

### 5. Registry Management

```go
// Create a registry for managing multiple actors
r := registry.New("my-app",
    registry.WithStartTimeout(10*time.Second),
    registry.WithStopTimeout(5*time.Second),
)

// Register actors
id1, err := r.Register("worker-1", actor1)
id2, err := r.Register("worker-2", actor2)

// Start all actors concurrently
if err := r.StartAll(ctx); err != nil {
    log.Fatal(err)
}

// Dispatch commands by ID
cmd := command.New(func(entity *MyEntity) (string, error) {
    return "processed", nil
})
err = r.Dispatch(ctx, id1, cmd)

// Graceful shutdown
if err := r.StopAll(); err != nil {
    log.Printf("Shutdown errors: %v", err)
}
```
## Architecture

1. Please take a look into [library Architecture](./ARCHITECTURE.md)

## Performance Considerations

- **Buffer Sizes**: Tune input buffer sizes based on message volume
- **Timeouts**: Configure appropriate timeouts for your use case
- **Resource Cleanup**: Always stop actors to prevent goroutine leaks
- **Batch Operations**: Use registry StartAll/StopAll for efficient bulk operations

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
