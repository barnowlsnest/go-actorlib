# go-actorlib

A lightweight, type-safe actor library for Go implementing the Actor Model pattern. Built with Go's goroutines and channels for high-performance concurrent applications.

## Features

- **Type-Safe**: Generic interfaces ensure compile-time type safety for entities and commands
- **Lightweight**: Minimal overhead using Go's native concurrency primitives
- **ActorRef**: Immutable, lightweight proxy that decouples senders from the concrete actor type
- **Command Pattern**: Built-in command implementation with result channels and error handling
- **Ask Pattern**: Single-call request/response with timeout
- **Actor System**: Name-based actor registry with Spawn convenience and event bus
- **Supervision**: Supervisor with OneForOne/AllForOne restart strategies, death watch, and max restart frequency
- **Behavior Change**: Dynamic handler switching via Become/Unbecome with BehaviorStack
- **Actor Context**: Access actor identity and behavior operations during message processing
- **Middleware**: Composable middleware chain for cross-cutting concerns (logging, metrics, recovery)
- **Dead Letters**: Queue for capturing undeliverable messages
- **Priority Mailbox**: Priority-based message ordering with configurable levels
- **Signal Handling**: OS signal integration (SIGTERM/SIGINT) for graceful shutdown
- **Context Support**: Full context.Context integration for cancellation and timeouts
- **Panic Recovery**: Automatic panic recovery with configurable error handling
- **Thread-Safe**: All operations are safe for concurrent use; tests run with `-race` detector

## Why Actor Model in Go?

Go already provides goroutines, channels, and `sync.Mutex` — so why add an actor abstraction on top?

### Pros

- **No locks, no data races by design.** Each actor owns its state exclusively. There is no shared mutable state to protect, so entire classes of concurrency bugs (deadlocks, forgotten mutexes, lock ordering issues) are eliminated at the structural level rather than by developer discipline.
- **Predictable sequential execution.** Commands sent to an actor are processed one at a time in order. Complex state mutations become simple single-threaded logic — no need to reason about interleaving.
- **Clear ownership boundaries.** The actor is the single source of truth for its entity. This makes it easy to reason about who can read or write a piece of state, even in large codebases.
- **Structured lifecycle.** Built-in start/stop/hooks give you a consistent way to manage resource setup and teardown across many concurrent components, avoiding leaked goroutines and orphaned resources.
- **Natural backpressure.** Bounded input channels signal when a component is overloaded, letting the system push back rather than silently growing unbounded queues.
- **Fault isolation.** A panic in one actor is recovered and contained — it doesn't bring down the entire application or corrupt unrelated state. Supervisors can automatically restart failed actors.

### Cons

- **Overhead for trivial concurrency.** If you just need a goroutine-safe counter or a simple fan-out, a `sync.Mutex` or a bare channel is lighter and more idiomatic.
- **Latency from message passing.** Every interaction goes through a channel send and sequential processing. For hot paths that need sub-microsecond shared reads, a `sync.RWMutex` or `sync/atomic` will be faster.
- **Debugging indirection.** Stack traces stop at channel operations. Tracing a request across multiple actors requires correlation IDs or structured logging — the call chain is no longer visible in a single stack.

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
    Value int
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

The simplest way to create, start, and obtain a reference:

```go
import (
    "github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
    "github.com/barnowlsnest/go-actorlib/v4/pkg/actorref"
)

ctx := context.Background()

// StartNew combines New + Start + WaitReady
myActor, err := actor.StartNew(ctx, 5*time.Second,
    actor.WithProvider(&CounterProvider{&Counter{}}),
    actor.WithInputBufferSize[*Counter](10),
    actor.WithReceiveTimeout[*Counter](5*time.Second),
    actor.WithName[*Counter]("counter"),
)
if err != nil {
    log.Fatal(err)
}

ref, err := actorref.New(myActor)
if err != nil {
    log.Fatal(err)
}
```

Or use the step-by-step approach for more control:

```go
myActor, err := actor.New(
    actor.WithProvider(&CounterProvider{&Counter{}}),
    actor.WithInputBufferSize[*Counter](10),
    actor.WithReceiveTimeout[*Counter](5*time.Second),
)
if err != nil {
    log.Fatal(err)
}

if err := myActor.Start(ctx); err != nil {
    log.Fatal(err)
}
if err := myActor.WaitReady(ctx, 5*time.Second); err != nil {
    log.Fatal(err)
}
```

### 4. Create an ActorRef and Hand It Out

`ActorRef` decouples the **lifecycle owner** (who creates and starts the actor) from **senders** (who only need to send messages). The ref exposes only `Send`, `Stop`, `State`, and `Done` — callers cannot call `Start`, `WaitReady`, or access internals.

```go
import "github.com/barnowlsnest/go-actorlib/v4/pkg/actorref"

ref, err := actorref.New(myActor)
if err != nil {
    log.Fatal(err)
}

// Pass ref to other components — they can send messages but cannot
// start, restart, or access lifecycle internals
svc := NewOrderService(ref)
```

### 5. Send Commands via Ref

Callers receive an `*actorref.Ref[T]` and interact with the actor through it:

```go
func (s *OrderService) PlaceOrder(ctx context.Context) error {
    cmd := command.New(func(counter *Counter) (int, error) {
        counter.Value++
        return counter.Value, nil
    })

    if err := s.ref.Send(ctx, cmd); err != nil {
        return err
    }

    result, ok := <-cmd.Done()
    if !ok {
        return cmd.Error()
    }
    fmt.Printf("Counter value: %d\n", result)
    return cmd.Error()
}
```

### 6. Ask Pattern (Request/Response with Timeout)

The `ask` package collapses command creation, sending, waiting, and error checking into a single call. It accepts an `*actorref.Ref[E]`:

```go
import "github.com/barnowlsnest/go-actorlib/v4/pkg/ask"

result, err := ask.New(ctx, ref, func(counter *Counter) (int, error) {
    counter.Value++
    return counter.Value, nil
}, 5*time.Second)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Counter value: %d\n", result)
```

### 7. Actor System (Registry and Spawn)

The `system` package provides a name-based registry with a `Spawn` convenience that combines actor creation, startup, and registration:

```go
import (
    "github.com/barnowlsnest/go-actorlib/v4/pkg/system"
)

sys := system.New()

// Spawn: New + Start + WaitReady + Ref + Register in one call
ref, err := system.Spawn(sys, ctx, "counter-1",
    &CounterProvider{&Counter{}},
    5*time.Second,
    actor.WithInputBufferSize[*Counter](10),
)
if err != nil {
    log.Fatal(err)
}

// Send commands by name
system.Send(sys, ctx, "counter-1", cmd)

// Ask by name with timeout
result, err := system.Ask(sys, ctx, "counter-1", func(c *Counter) (int, error) {
    c.Value++
    return c.Value, nil
}, 5*time.Second)

// Event bus — observe actor lifecycle
sys.OnEvent(func(e system.Event) {
    fmt.Printf("event: %v actor: %s\n", e.Kind, e.ActorName)
})

// Graceful LIFO shutdown
sys.StopAll(10 * time.Second)
```

### 8. Supervision

The `supervision` package monitors child actors and restarts them on failure:

```go
import "github.com/barnowlsnest/go-actorlib/v4/pkg/supervision"

sup := supervision.NewSupervisor(
    supervision.WithPolicy(supervision.RestartPolicy{
        Strategy:       supervision.OneForOne, // or AllForOne
        MaxRestarts:    3,
        WithinDuration: 10 * time.Second,
    }),
)

sup.Add("worker-1", &MyChildSpec{})
sup.Add("worker-2", &MyChildSpec{})

// Death watch — observe child terminations
sup.Watch(func(name string, state uint64) {
    fmt.Printf("child %s terminated with state %d\n", name, state)
})

sup.StartAll(ctx, 5*time.Second)
defer sup.StopAll(10 * time.Second)
```

`ChildSpec` is an interface you implement to define how children are created:

```go
type MyChildSpec struct{}

func (s *MyChildSpec) Start(ctx context.Context) (supervision.ChildRef, error) {
    a, err := actor.StartNew(ctx, 5*time.Second,
        actor.WithProvider(&MyProvider{&MyEntity{}}),
    )
    if err != nil {
        return nil, err
    }
    return actorref.New(a)
}
```

### 9. Behavior Change (Become/Unbecome)

Actors can dynamically switch their message handling logic at runtime via `GoActorContext`:

```go
func initialHandler(ctx context.Context, e actor.Executable[*MyEntity], entity *MyEntity) {
    e.Execute(ctx, entity)

    // Switch to a different behavior
    actorCtx := actor.GetGoActorContext[*MyEntity](ctx)
    actorCtx.Become(authenticatedHandler)
}

func authenticatedHandler(ctx context.Context, e actor.Executable[*MyEntity], entity *MyEntity) {
    e.Execute(ctx, entity)

    // Revert to previous behavior
    actorCtx := actor.GetGoActorContext[*MyEntity](ctx)
    actorCtx.Unbecome()
}
```

### 10. Middleware

Add cross-cutting concerns to actor message processing with composable middleware:

```go
import "github.com/barnowlsnest/go-actorlib/v4/pkg/middleware"

metrics := &middleware.Metrics{}

myActor, err := actor.StartNew(ctx, 5*time.Second,
    actor.WithProvider(provider),
    actor.WithName[*Counter]("counter"),
    actor.WithMiddleware(
        middleware.Recovery[*Counter](slog.Default()),       // catch panics
        middleware.Logging[*Counter](slog.Default()),        // log messages
        middleware.MetricsMiddleware[*Counter](metrics),     // collect stats
    ),
)

// Query metrics concurrently
fmt.Printf("processed: %d, avg: %s\n", metrics.MessageCount(), metrics.AverageDuration())
```

### 11. Dead Letters

Capture undeliverable messages for debugging and monitoring:

```go
import "github.com/barnowlsnest/go-actorlib/v4/pkg/deadletter"

dlq := deadletter.New(deadletter.WithCapacity(1000))

dlq.OnDeadLetter(func(l deadletter.Letter) {
    log.Printf("dead letter: target=%s reason=%s", l.Target, l.Reason)
})

dlq.Publish(deadletter.Letter{Target: "worker-1", Reason: "actor stopped"})
```

### 12. Graceful Shutdown with OS Signals

```go
import "github.com/barnowlsnest/go-actorlib/v4/pkg/signal"

// Block until SIGTERM/SIGINT, then stop the system
err := signal.AwaitShutdown(ctx, sys, 10*time.Second)

// Or handle manually
notify, stop := signal.NotifyShutdown()
defer stop()
<-notify
sys.StopAll(10 * time.Second)
```

### 13. Priority Mailbox

Messages with higher priority are processed first:

```go
import "github.com/barnowlsnest/go-actorlib/v4/pkg/mailbox"

mb := mailbox.NewPriority[*MyEntity](100)

mb.Push(normalCmd, mailbox.Normal)
mb.Push(systemCmd, mailbox.System)  // processed first
mb.Push(lowCmd, mailbox.Low)        // processed last

msg, ok := mb.Pop() // returns systemCmd
```

Priority levels (highest to lowest): `System` > `High` > `Normal` > `Low`. FIFO order is maintained within the same priority.

## Packages

| Package           | Description                                                                                         |
|-------------------|-----------------------------------------------------------------------------------------------------|
| `pkg/actor`       | Core actor: GoActor, Entity, Executable, Hooks, BehaviorStack, GoActorContext, Middleware, StartNew |
| `pkg/actorref`    | Typed actor handle: Ref with Send, Stop, State, Done                                                |
| `pkg/command`     | Command pattern: GoCommand with DelegateFn and result channels                                      |
| `pkg/ask`         | Request/response convenience with timeout                                                           |
| `pkg/system`      | Actor system: name registry, Spawn, event bus, Send/Ask by name                                     |
| `pkg/supervision` | Supervisor: OneForOne/AllForOne, ChildSpec, death watch, restart policy                             |
| `pkg/middleware`  | Reference middleware: Logging (slog), Metrics (atomic), Recovery (panic)                            |
| `pkg/deadletter`  | Dead letter queue with capacity and handlers                                                        |
| `pkg/signal`      | OS signal integration: AwaitShutdown, NotifyShutdown                                                |
| `pkg/mailbox`     | Priority mailbox with System/High/Normal/Low levels                                                 |

## Architecture

PlantUML diagrams documenting the architecture are available in [`docs/`](./docs/README.md).

## Performance Considerations

- **Buffer Sizes**: Tune input buffer sizes based on message volume
- **Timeouts**: Configure appropriate timeouts for your use case
- **Resource Cleanup**: Always stop actors to prevent goroutine leaks
- **Middleware**: Middleware chain is composed once at startup — zero per-message allocation overhead
- **Supervision**: Restart frequency limits prevent restart storms from consuming resources

## Development Commands

This project uses [Task](https://taskfile.dev/) for build automation:

```bash
# Install Task
go install github.com/go-task/task/v3/cmd/task@latest

# Essential commands
task sanity       # Run all checks (tidy, fmt, lint, build, vet, test)
task go-build     # Build all packages
task go-test      # Run tests with coverage, benchmarks, and -race
task go-lint      # Run golangci-lint
task go-fmt       # Format all Go files
task go-vet       # Run static analysis
task go-tidy      # go mod tidy
```

## Requirements

- Go 1.26.1 or later
- Dependencies: [go-datalib](https://github.com/barnowlsnest/go-datalib) (Heap for priority mailbox), testify (tests only)

## License

This project is licensed under the terms specified in the LICENSE file.

## References

- [Actor Model](https://en.wikipedia.org/wiki/Actor_model) - Learn about the Actor Model pattern
- [Go Concurrency Patterns](https://go.dev/blog/pipelines) - Go concurrency best practices
