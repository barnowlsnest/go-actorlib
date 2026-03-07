# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

go-actorlib is a lightweight, type-safe Actor Model library for Go built on native concurrency primitives (goroutines and channels). Module: `github.com/barnowlsnest/go-actorlib/v4`, requires Go 1.25.

## Commands

Build, test, and lint tasks use [Task](https://taskfile.dev):

```bash
task sanity          # Run all checks (tidy, fmt, lint, build, vet, test)
task go-test         # Run tests with coverage + benchmarks
task go-lint         # Run golangci-lint
task go-build        # Build all packages
task go-vet          # Static analysis
task go-fmt          # Format code
task go-tidy         # go mod tidy
```

Run fuzz tests:
```bash
go test -fuzz FuzzActorLifecycle -fuzztime 5s ./pkg/actor/
go test -fuzz FuzzConcurrentStopAndSend -fuzztime 5s ./pkg/actor/
```

## Architecture

Ten packages under `pkg/`:

### `pkg/actor` — Core actor implementation
- **`GoActor[T Entity]`** — Generic actor that manages an entity of type T in an isolated goroutine. Processes `Executable[T]` commands sequentially from a bounded input channel.
- **`Entity`** interface — Constraint for actor-managed data (requires `IsProvidable() bool`).
- **`Executable[T Entity]`** interface — Commands that can be sent to an actor via `Receive()`.
- **`Hooks`** interface — Lifecycle callbacks: BeforeStart, AfterStart, BeforeStop, AfterStop, OnError. Default is `noopHooks`.
- **`BehaviorStack[T]`** — Stack of `HandlerFunc[T]` for dynamic behavior changes. Supports `Become` (push), `BecomeReplace` (swap top), `Unbecome` (pop). Only accessed from within the actor's goroutine.
- **`GoActorContext[T]`** — Available during message processing via `GetGoActorContext[T](ctx)`. Provides `Become()`, `BecomeReplace()`, `Unbecome()`, and `Name()`. Injected automatically when the actor starts.
- **`StartNew[T]`** — Convenience function combining `New`, `Start`, and `WaitReady` into one call.
- **State machine** (7 states via `sync/atomic`): Initialized → Started → Stopping → Done/StoppedWithError/Canceled/Panicked.
- **Stop uses atomic CAS** (`CompareAndSwapUint64`) for concurrent safety; a dedicated `stop` channel signals shutdown.
- Configured via functional options: `WithProvider`, `WithInputBufferSize`, `WithReceiveTimeout`, `WithHooks`, `WithName`, `WithMiddleware`.

### `pkg/actorref` — Typed actor handle (proxy)
- **`Ref[T Entity]`** — Immutable, lightweight proxy struct for interacting with a `GoActor[T]`. Exposes `Send`, `Stop`, `State`, and `Done` — hides lifecycle methods (`Start`, `WaitReady`, `CheckState`).
- **`New[T Entity](a *GoActor[T]) (*Ref[T], error)`** — Constructor; validates non-nil actor.
- **`Done() <-chan struct{}`** — Returns a channel that closes when the underlying actor terminates.
- Safe for concurrent use; multiple refs can point to the same actor.

### `pkg/command` — Command pattern for async operations
- **`GoCommand[E Entity, R any]`** — Wraps a `DelegateFn[E, R]` (func(entity E) (R, error)) as an `Executable[E]`. Returns results via a buffered channel (`Done()`).
- **State machine** (6 states via `sync.Mutex`): Created → Started → Finished/Failed/Canceled/Panic.

### `pkg/ask` — Ask pattern (request/response convenience)
- **`New[E Entity, R any]`** — Single-call request/response with timeout. Accepts `*actorref.Ref[E]` (not `*GoActor[E]`). Wraps command creation, `Send`, and result waiting into one function. Returns `(R, error)`.
- **`ErrAskTimeout`** — Returned when the result is not received within the specified timeout.

### `pkg/system` — Actor system with registry and lifecycle
- **`ActorSystem`** — Flat name registry for actor refs. LIFO-ordered shutdown via `StopAll`.
- **`Spawn[T]`** — Convenience function combining `actor.New`, `Start`, `WaitReady`, `actorref.New`, and `Register` into one call with automatic cleanup on failure.
- **Event bus** — `OnEvent(handler)` registers handlers; emits `EventActorStarted`, `EventActorStopped`, `EventSystemStopping`.

### `pkg/supervision` — Supervisor for actor lifecycle management
- **`Supervisor`** — Monitors child actors and restarts them according to a configurable restart policy. Thread-safe.
- **`ChildSpec`** interface — Factory for creating child actors (`Start(ctx) (ChildRef, error)`).
- **`ChildRef`** interface — Supervised child must implement `Stop`, `State`, `Done`.
- **Strategies**: `OneForOne` (restart only failed child), `AllForOne` (restart all children on any failure).
- **`RestartPolicy`** — Configures strategy, max restarts, and time window.
- **Death watch** — `Watch(callback)` registers watchers notified on child termination.
- Version-tracked monitors prevent stale restart cascades.

### `pkg/middleware` — Reference middleware implementations
- **`Logging`** — Logs each message with duration using `log/slog`.
- **`Metrics`** — Atomic counters for message count and total/average processing duration.
- **`Recovery`** — Catches panics in downstream handlers, logs them, prevents actor Panicked state.

### `pkg/deadletter` — Dead letter queue
- **`Queue`** — Collects undeliverable messages with configurable capacity (default 1000, oldest evicted). Thread-safe.
- **`Letter`** — Contains `Target` (actor name) and `Reason` (delivery failure description).
- **`OnDeadLetter(handler)`** — Registers callbacks for new dead letters.

### `pkg/signal` — OS signal integration
- **`AwaitShutdown(ctx, stoppable, timeout)`** — Blocks until SIGTERM/SIGINT, then calls `StopAll`.
- **`NotifyShutdown()`** — Returns a signal channel and stop function for manual handling.
- Works with any `Stoppable` interface (both `ActorSystem` and `Supervisor`).

### `pkg/mailbox` — Alternative mailbox implementations
- **`PriorityMailbox[T]`** — Thread-safe priority queue using go-datalib's Heap. Priority levels: `System` > `High` > `Normal` > `Low`. FIFO within same priority.
- **`Push(executable, priority)`** / **`Pop()`** — O(log n) insert and extract.
- **`Notify()`** — Channel signaled when messages are available.

### Design patterns
- **Actor Model**: Isolated actors with exclusive state access, async communication via `Executable` commands.
- **Command Pattern**: `GoCommand` wraps operations as objects; actors execute them, entities process them.
- **Observer Pattern**: `Hooks` interface and event bus provide lifecycle event notifications.
- **Behavior Pattern**: `BehaviorStack` enables dynamic handler switching via `Become`/`Unbecome`.
- **Supervision Pattern**: `Supervisor` monitors children with restart policies and death watch.

### Concurrency model
- One actor = one goroutine. Bounded channels for message queuing provide natural backpressure.
- Stop uses atomic CAS for concurrent safety; a dedicated stop channel signals shutdown to avoid Receive/Stop races.
- Configurable timeouts on send operations; errors propagated when buffers are full.
- All tests run with `-race` detector enabled.

### Dependencies
- **go-datalib** (`github.com/barnowlsnest/go-datalib`) — Provides Heap data structure for priority mailbox.
- **testify** — Test assertions and suites (test-only).

### PlantUML diagrams
Available in [`docs/`](./docs/README.md): architecture overview, component relationships, actor lifecycle state machine, command execution flow, message passing sequence.

### Typical usage flow
1. Define entity implementing `Entity`, create a provider implementing `EntityProvider[T]`.
2. Create and start actor: `actor.StartNew(ctx, timeout, WithProvider(provider), opts...)` or manually: `actor.New` → `Start` → `WaitReady`.
3. Obtain a ref: `ref, err := actorref.New(myActor)`.
4. Or use system: `ref, err := system.Spawn(sys, ctx, "name", provider, timeout, opts...)`.
5. Send commands: `ref.Send(ctx, command.New(delegateFn))`.
6. Get results: `<-cmd.Done()`, check `cmd.Error()`.
7. Dynamic behavior: `actorCtx := actor.GetGoActorContext[T](ctx); actorCtx.Become(newHandler)`.
8. Shutdown: `ref.Stop(timeout)` or `sys.StopAll(timeout)` or `signal.AwaitShutdown(ctx, sys, timeout)`.

## Code Conventions

- Heavy use of Go generics for type safety — avoid raw `interface{}`.
- All concurrency is channel-based with atomic state management; no shared mutable state between actors.
- Panic recovery is built into actor and command execution; panics become errors via `errors.Join()`.
- Test naming convention: `TestX_Condition_ShouldY` (behavior-driven).
- Linting: golangci-lint with 21 linters enabled. Key limits: line length 140, cyclomatic complexity 15, function length 100 lines / 50 statements. Test files are excluded from funlen, dupl, goconst, gocyclo, gosec. Formatters: gofmt + goimports (local prefix: `github.com/barnowlsnest/go-actorlib`).
