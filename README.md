# go-actorlib

A lightweight, type-safe [Actor Model](https://en.wikipedia.org/wiki/Actor_model) library for Go. Actors run on native goroutines and channels — no extra runtime, no shared mutable state.

[![Go Reference](https://pkg.go.dev/badge/github.com/barnowlsnest/go-actorlib/v4.svg)](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4)
[![Go Version](https://img.shields.io/github/go-mod/go-version/barnowlsnest/go-actorlib)](go.mod)
[![Build](https://github.com/barnowlsnest/go-actorlib/actions/workflows/build.yml/badge.svg)](https://github.com/barnowlsnest/go-actorlib/actions/workflows/build.yml)
[![Lint](https://github.com/barnowlsnest/go-actorlib/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/barnowlsnest/go-actorlib/actions/workflows/golangci-lint.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Module:** [`github.com/barnowlsnest/go-actorlib/v4`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4) · **Go:** 1.27+ · **License:** [MIT](LICENSE)

## Features

- **Type-safe** — generics bind entities, commands, and refs at compile time
- **Lightweight** — one actor is one goroutine plus a bounded channel
- **ActorRef** — immutable proxy that exposes `Send`, `Stop`, `State`, and `Done`
- **Command & Ask** — async commands with result channels, or a single-call request/response
- **Actor System** — name registry, `Spawn`, typed `Send`/`Ask` by name, event bus, LIFO shutdown
- **Supervision** — OneForOne / AllForOne restarts, death watch, restart-frequency limits
- **Behavior change** — `Become` / `BecomeReplace` / `Unbecome` via `GoActorContext`
- **Middleware** — composable logging, metrics, and panic recovery (`log/slog`)
- **Dead letters** — queue for undeliverable messages with handlers
- **Priority mailbox** — standalone `System` > `High` > `Normal` > `Low` queue (FIFO within a level)
- **Signals** — SIGTERM / SIGINT graceful shutdown for systems and supervisors
- **Panic recovery** — panics become errors; actors and commands stay isolated
- **Race-tested** — the suite runs with the `-race` detector

## Why an actor library in Go?

Go already has goroutines, channels, and `sync.Mutex`. This library is for the cases where those primitives are not enough on their own.

### Pros

- **No locks, no data races by design.** Each actor owns its state. There is no shared mutable state to protect, so deadlocks, forgotten mutexes, and lock-ordering bugs are structural impossibilities rather than discipline problems.
- **Predictable sequential execution.** Commands are processed one at a time. Complex mutations stay single-threaded.
- **Clear ownership.** The actor is the single source of truth for its entity.
- **Structured lifecycle.** Start / stop / hooks give a consistent way to set up and tear down many concurrent components.
- **Natural backpressure.** Bounded input channels signal overload instead of growing without limit.
- **Fault isolation.** A panic in one actor is recovered and contained. Supervisors can restart failed children.

### Cons

- **Overhead for trivial concurrency.** A `sync.Mutex` or a bare channel is lighter for a counter or a simple fan-out.
- **Latency from message passing.** Every interaction is a channel send plus sequential processing. Hot shared reads are faster with `sync.RWMutex` or `sync/atomic`.
- **Debugging indirection.** Stack traces stop at channel operations. Cross-actor requests need correlation IDs or structured logs.

### When to use

- Long-lived stateful components (connection managers, session stores, caches, worker coordinators)
- State that many goroutines must read and mutate, where locking is error-prone
- Systems that need ordered startup and coordinated shutdown
- Domains that decompose into independent entities (sessions, devices, per-tenant state)

### When not to use

- Request-scoped concurrency — `sync.WaitGroup` or `errgroup` is enough
- Read-heavy, write-rare shared state — `sync.RWMutex` or `atomic.Value`
- Fire-and-forget work — a goroutine and a channel
- CPU-bound pipelines — worker pools and fan-out / fan-in

## Installation

```bash
go get github.com/barnowlsnest/go-actorlib/v4
```

Requires **Go 1.27** or later.

## Quick start

Define an entity, start an actor, and ask it a question:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
    "github.com/barnowlsnest/go-actorlib/v4/pkg/actorref"
    "github.com/barnowlsnest/go-actorlib/v4/pkg/ask"
)

type Counter struct{ Value int }

func (c *Counter) IsProvidable() bool { return true }

type CounterProvider struct{ counter *Counter }

func (p *CounterProvider) Provide() *Counter { return p.counter }

func main() {
    ctx := context.Background()

    a, err := actor.StartNew(ctx, 5*time.Second,
        actor.WithProvider(&CounterProvider{&Counter{}}),
        actor.WithName[*Counter]("counter"),
    )
    if err != nil {
        log.Fatal(err)
    }

    ref, err := actorref.New(a)
    if err != nil {
        log.Fatal(err)
    }
    defer ref.Stop(5 * time.Second)

    n, err := ask.New(ctx, ref, func(c *Counter) (int, error) {
        c.Value++
        return c.Value, nil
    }, 5*time.Second)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(n) // 1
}
```

Defaults: input buffer size `1`, receive timeout `5s`. A receive timeout of `0` waits indefinitely (still respects `ctx`).

### ActorRef

`Ref` decouples the lifecycle owner from senders. Callers get `Send`, `Stop`, `State`, and `Done` — not `Start`, `WaitReady`, or internals. Multiple refs may point at the same actor.

```go
ref, err := actorref.New(myActor)
if err != nil {
    log.Fatal(err)
}

svc := NewOrderService(ref) // can send; cannot start or restart
```

### Commands

```go
cmd := command.New(func(counter *Counter) (int, error) {
    counter.Value++
    return counter.Value, nil
})

if err := ref.Send(ctx, cmd); err != nil {
    return err
}

result, ok := <-cmd.Done()
if !ok {
    return cmd.Error()
}
fmt.Printf("Counter value: %d\n", result)
return cmd.Error()
```

### Ask (request / response with timeout)

```go
result, err := ask.New(ctx, ref, func(counter *Counter) (int, error) {
    counter.Value++
    return counter.Value, nil
}, 5*time.Second)
```

`ask.ErrAskTimeout` is returned when the timeout elapses before a result arrives.

### Manual lifecycle

`StartNew` is `New` + `Start` + `WaitReady`. The step-by-step form is useful when you need hooks between those calls:

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

### Actor system

`Spawn` is `New` + `Start` + `WaitReady` + `actorref.New` + `Register`. It applies `WithName` from the registry name. Failed registration stops the actor so nothing is left running unregistered.

`Register`, `Send`, and `Ask` are generic free functions.

```go
sys := system.New()

ref, err := system.Spawn(sys, ctx, "counter-1",
    &CounterProvider{&Counter{}},
    5*time.Second,
    actor.WithInputBufferSize[*Counter](10),
)
if err != nil {
    log.Fatal(err)
}

_ = system.Send(sys, ctx, "counter-1", cmd)

result, err := system.Ask(sys, ctx, "counter-1", func(c *Counter) (int, error) {
    c.Value++
    return c.Value, nil
}, 5*time.Second)

sys.OnEvent(func(e system.Event) {
    fmt.Printf("event: %v actor: %s\n", e.Kind, e.ActorName)
})

sys.StopAll(10 * time.Second) // LIFO, then the system is stopped
```

Events: `EventActorStarted` (Spawn), `EventActorStopped`, `EventSystemStopping`.

### Supervision

Default policy: OneForOne, max 3 restarts within 5 seconds. Clean `Done` stops are not restarted.

```go
sup := supervision.NewSupervisor(
    supervision.WithPolicy(supervision.RestartPolicy{
        Strategy:       supervision.OneForOne, // or AllForOne
        MaxRestarts:    3,
        WithinDuration: 10 * time.Second,
    }),
)

if err := sup.Add("worker-1", &MyChildSpec{}); err != nil {
    log.Fatal(err)
}

sup.Watch(func(name string, state uint64) {
    fmt.Printf("child %s terminated with state %d\n", name, state)
})

if err := sup.StartAll(ctx, 5*time.Second); err != nil {
    log.Fatal(err)
}
defer sup.StopAll(10 * time.Second)
```

`ChildSpec` is the factory; `actorref.Ref` already implements `ChildRef`:

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

### Behavior change

```go
func initialHandler(ctx context.Context, e actor.Executable[*MyEntity], entity *MyEntity) {
    e.Execute(ctx, entity)
    actor.GetGoActorContext[*MyEntity](ctx).Become(authenticatedHandler)
}

func authenticatedHandler(ctx context.Context, e actor.Executable[*MyEntity], entity *MyEntity) {
    e.Execute(ctx, entity)
    actor.GetGoActorContext[*MyEntity](ctx).Unbecome()
}
```

`GoActorContext` is valid only while the current message is processed. `Unbecome` will not pop the base handler.

### Middleware

Place `Recovery` first if you want panics caught before they put the actor in `Panicked`. The chain is composed once at `Start`.

```go
metrics := &middleware.Metrics{}

myActor, err := actor.StartNew(ctx, 5*time.Second,
    actor.WithProvider(provider),
    actor.WithName[*Counter]("counter"),
    actor.WithMiddleware(
        middleware.Recovery[*Counter](slog.Default()),
        middleware.Logging[*Counter](slog.Default()),
        middleware.MetricsMiddleware[*Counter](metrics),
    ),
)
```

### Dead letters

The queue is opt-in: publish undeliverable messages yourself (for example from a send-error path). Default capacity is 1000; the oldest letter is dropped when full.

```go
dlq := deadletter.New(deadletter.WithCapacity(1000))

dlq.OnDeadLetter(func(l deadletter.Letter) {
    log.Printf("dead letter: target=%s reason=%s", l.Target, l.Reason)
})

dlq.Publish(deadletter.Letter{Target: "worker-1", Reason: "actor stopped"})
```

### OS signals

`AwaitShutdown` works with anything that implements `StopAll` — both `ActorSystem` and `Supervisor`.

```go
err := signal.AwaitShutdown(ctx, sys, 10*time.Second)

notify, stop := signal.NotifyShutdown()
defer stop()
<-notify
sys.StopAll(10 * time.Second)
```

### Priority mailbox

`PriorityMailbox` is a standalone heap. The actor's built-in mailbox is a bounded Go channel; this type is not plugged in automatically.

```go
mb := mailbox.NewPriority[*MyEntity](100)

mb.Push(normalCmd, mailbox.Normal)
mb.Push(systemCmd, mailbox.System) // processed first
mb.Push(lowCmd, mailbox.Low)

msg, ok := mb.Pop() // systemCmd
```

Priority (highest first): `System` > `High` > `Normal` > `Low`. FIFO within the same level. `Push` returns `false` when the mailbox is full or closed.

## Packages

| Package | Description |
|---|---|
| [`pkg/actor`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/actor) | `GoActor`, `Entity`, `Executable`, `Hooks`, `BehaviorStack`, `GoActorContext`, middleware, `StartNew` |
| [`pkg/actorref`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/actorref) | Typed `Ref`: `Send`, `Stop`, `State`, `Done` |
| [`pkg/command`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/command) | `GoCommand` with `DelegateFn` and a result channel |
| [`pkg/ask`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/ask) | Request/response with timeout |
| [`pkg/system`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/system) | Name registry, `Spawn`, `Register`/`Send`/`Ask`, event bus |
| [`pkg/supervision`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/supervision) | Supervisor: OneForOne / AllForOne, `ChildSpec`, death watch |
| [`pkg/middleware`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/middleware) | Logging (`slog`), Metrics (atomic), Recovery |
| [`pkg/deadletter`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/deadletter) | Dead-letter queue with capacity and handlers |
| [`pkg/signal`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/signal) | `AwaitShutdown`, `NotifyShutdown` |
| [`pkg/mailbox`](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4/pkg/mailbox) | Standalone priority mailbox |

PlantUML diagrams: [`docs/`](./docs/README.md).

## Performance notes

- Tune input buffer size to your message volume; the default is `1`.
- Set receive timeouts for backpressure; `0` means wait until `ctx` is done.
- Always `Stop` actors (or `StopAll` on the system / supervisor) to avoid leaked goroutines.
- Middleware is composed once at startup — no per-message allocation from the chain itself.
- Restart-frequency limits on supervisors prevent restart storms.

## Development

Build automation uses [Task](https://taskfile.dev/):

```bash
go install github.com/go-task/task/v3/cmd/task@latest

task sanity        # tidy, fmt, lint --fix, build, vet, test
task go-build      # go build ./...
task go-test       # tests with -race, coverage, and benchmarks
task go-lint-fix   # golangci-lint run --fix
task go-fmt        # go fmt ./...
task go-vet        # go vet ./...
task go-tidy       # go mod tidy
```

CI on `main` (push and pull request) runs build/test and [golangci-lint](https://golangci-lint.run/) v2.

Fuzz tests:

```bash
go test -fuzz FuzzActorLifecycle -fuzztime 5s ./pkg/actor/
go test -fuzz FuzzConcurrentStopAndSend -fuzztime 5s ./pkg/actor/
```

Runtime dependency: [go-datalib](https://github.com/barnowlsnest/go-datalib) (heap for the priority mailbox). [testify](https://github.com/stretchr/testify) is test-only.

## Contributing

Issues and pull requests are welcome.

1. Open an issue first for larger design changes.
2. Keep PRs focused; match existing style and the `TestX_Condition_ShouldY` test names.
3. `task sanity` must pass locally (includes `-race` tests and lint).

Please do not commit generated coverage files or editor config (see `.gitignore`).

## License

[MIT](LICENSE) © 2025 Barn Owls Nest

## References

- [Actor Model](https://en.wikipedia.org/wiki/Actor_model)
- [Go concurrency patterns](https://go.dev/blog/pipelines)
- [pkg.go.dev documentation](https://pkg.go.dev/github.com/barnowlsnest/go-actorlib/v4)
