# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

go-actorlib is a lightweight, type-safe Actor Model library for Go built on native concurrency primitives (goroutines and channels). Module: `github.com/barnowlsnest/go-actorlib/v4`, requires Go 1.27. License: MIT.

## Commands

Build, test, and lint tasks use [Task](https://taskfile.dev). There is no `go-lint` task; lint is `go-lint-fix`.

```bash
task sanity          # tidy, fmt, lint --fix, build, vet, test
task go-test         # go test -race -cover ./... then benchmarks
task go-lint-fix     # golangci-lint run --fix
task go-build        # go build ./...
task go-vet          # go vet ./...
task go-fmt          # go fmt ./...
task go-tidy         # go mod tidy
```

CI (`.github/workflows/`):

- `build.yml` — `task go-build` then `task go-test`; Go version from `go.mod`
- `golangci-lint.yml` — `golangci/golangci-lint-action@v9` with golangci-lint **v2.13**

Fuzz tests:

```bash
go test -fuzz FuzzActorLifecycle -fuzztime 5s ./pkg/actor/
go test -fuzz FuzzConcurrentStopAndSend -fuzztime 5s ./pkg/actor/
```

## Architecture

Ten packages under `pkg/`. The actor's built-in mailbox is a bounded Go channel. `pkg/mailbox` is a standalone heap and is **not** wired into `GoActor.Receive`.

### `pkg/actor` — Core actor implementation

- **`GoActor[T Entity]`** — Generic actor that manages an entity of type T in an isolated goroutine. Processes `Executable[T]` commands sequentially from a bounded input channel.
- **`Entity`** — `IsProvidable() bool`.
- **`Executable[T Entity]`** — Commands sent via `Receive()`.
- **`EntityProvider[T]`** — `Provide() T`; required at construction (`ErrActorNilProvider`).
- **`Hooks`** — BeforeStart, AfterStart, BeforeStop, AfterStop, OnError. Default is `noopHooks`.
- **`BehaviorStack[T]`** — Stack of `HandlerFunc[T]`. `Become` (push), `BecomeReplace` (swap top), `Unbecome` (pop; cannot pop the base handler). Goroutine-local; no synchronization.
- **`GoActorContext[T]`** — Injected into the message `context.Context` at start. `GetGoActorContext[T](ctx)` returns nil outside a handler. Valid only for the current message. Exposes `Become`, `BecomeReplace`, `Unbecome`, `Name`.
- **`StartNew[T]`** — `New` + `Start` + `WaitReady`.
- **Middleware** — `Middleware[T]` wraps `HandlerFunc[T]`. `Chain` folds right-to-left (A, B, C → A then B then C then handler). `WithMiddleware` **appends**; composed once in `Start()`. Empty chain calls `Execute` directly.
- **State machine** (7 states via `sync/atomic`): Initialized → Started → Stopping → Done / StoppedWithError / Canceled / Panicked.
- **Stop** uses atomic CAS (`CompareAndSwapUint64`); a dedicated `stop` channel signals shutdown.
- **Receive** rejects nil commands and any state after Started (`state > 1`). Timeout `0` skips the timer and still honors `ctx`.
- Defaults: `inputBufSize = 1`, `receiveTimeout = 5s`.
- Options: `WithProvider`, `WithInputBufferSize`, `WithReceiveTimeout`, `WithHooks`, `WithName`, `WithMiddleware`.
- Also: `Name()`, `Done()`, `InputBufferSize()`, `CheckState`, `State`.

### `pkg/actorref` — Typed actor handle (proxy)

- **`Ref[T Entity]`** — Immutable proxy (`actor` field unexported). `Send`, `Stop`, `State`, `Done`. Hides `Start`, `WaitReady`, `CheckState`.
- **`New[T](a *GoActor[T]) (*Ref[T], error)`** — `ErrActorRefNilActor` if `a` is nil.
- Safe for concurrent use; multiple refs can point to the same actor. `Send` delegates to `Receive`.

### `pkg/command` — Command pattern for async operations

- **`GoCommand[E Entity, R any]`** — Wraps `DelegateFn[E, R]` (`func(entity E) (R, error)`) as `Executable[E]`. Buffered result channel (`Done()`, cap 1).
- **State machine** (6 states via `sync.Mutex`): Created → Started → Finished / Failed / Canceled / Panic.
- On success: one value on `Done()`, then close. On failure/cancel/panic: close without a value; inspect `Error()`.

### `pkg/ask` — Ask pattern (request/response convenience)

- **`New[E Entity, R any](ctx, *actorref.Ref[E], DelegateFn, timeout)`** — Command + `Send` + wait. Accepts `*actorref.Ref[E]`, not `*GoActor[E]`.
- **`ErrAskTimeout`** — Result not received within timeout. Also returns `ctx.Err()` if the context is done first.

### `pkg/system` — Actor system with registry and lifecycle

- **`ActorSystem`** — Flat name registry. Thread-safe. After `StopAll`, further ops return `ErrSystemStopped`.
- **`Register[T](s, name, ref)`**, **`Send[T]`**, **`Ask[T, R]`** — Generic **free functions** (not methods). Register captures a type-erased dispatch closure; Send/Ask type-assert and return `ErrCommandTypeMismatch` on mismatch.
- **`Spawn[T]`** — `actor.New` + `Start` + `WaitReady` + `actorref.New` + `Register`. Always applies `WithName` from the registry name. On register failure, stops the actor (best effort) so the system is unchanged.
- Methods: `Get` → `ManagedActor`, `Unregister` (does **not** stop the actor; tombstones the LIFO slot), `Count`, `StopAll` (LIFO, then emit events), `OnEvent`.
- **Event bus** — `EventActorStarted` (Spawn only), `EventActorStopped`, `EventSystemStopping`. Handlers run synchronously in registration order. `StopAll` emits `EventSystemStopping` then one `EventActorStopped` per actor.

### `pkg/supervision` — Supervisor for actor lifecycle management

- **`Supervisor`** — Monitors children and restarts on failure. Thread-safe.
- **`ChildSpec`** — `Start(ctx) (ChildRef, error)`.
- **`ChildRef`** — `Stop`, `State`, `Done`. `actorref.Ref` satisfies this.
- **Strategies**: `OneForOne` (restart only the failed child), `AllForOne` (stop+restart all). Clean `actor.Done` does **not** restart.
- **`RestartPolicy`** — Strategy, `MaxRestarts` (0 = unlimited), `WithinDuration`. **`DefaultRestartPolicy()`**: OneForOne, max 3 within 5s.
- Options: `WithPolicy`, `WithStopTimeout` (default 5s, used when stopping siblings on AllForOne).
- **Death watch** — `Watch(callback)` on any child termination (including clean stops).
- Version-tracked monitors prevent stale restart cascades.
- Also: `Add`, `StartAll`, `StopAll` (LIFO), `Children()`, `ChildState(name)`.

### `pkg/middleware` — Reference middleware implementations

- **`Logging`** — `slog` debug logs around each message, with duration and actor name from context.
- **`Metrics` / `MetricsMiddleware`** — Atomic counters: `MessageCount`, `TotalDuration`, `AverageDuration`.
- **`Recovery`** — Catches panics in downstream handlers, logs at error, prevents actor `Panicked`. Place **first** in the chain to wrap everything else.

### `pkg/deadletter` — Dead letter queue

- **`Queue`** — Opt-in; callers `Publish` undeliverable messages. Default capacity 1000, oldest evicted. Thread-safe.
- **`Letter`** — `Target` (actor name), `Reason`.
- **`OnDeadLetter`**, **`Letters()`** (copy), **`Count()`**, **`Clear()`**.

### `pkg/signal` — OS signal integration

- **`AwaitShutdown(ctx, stoppable, timeout)`** — Blocks until SIGTERM/SIGINT **or** `ctx` done, then `StopAll`.
- **`NotifyShutdown()`** — Signal channel + `stop` to deregister.
- **`Stoppable`** — `StopAll(timeout) error`. Both `ActorSystem` and `Supervisor` satisfy it.

### `pkg/mailbox` — Alternative mailbox implementations

- **`PriorityMailbox[T]`** — Standalone, thread-safe heap (`go-datalib` `tree.Heap`). Not used by `GoActor`.
- Priority: `System` > `High` > `Normal` > `Low` (iota; lower value = higher priority). FIFO via insertion `seq` within the same priority.
- **`Push`** returns false if full (`maxSize > 0`) or closed. **`Pop`**, **`Notify`** (buffered 1, non-blocking signal), **`Size`**, **`Close`**, **`IsEmpty`**.

### Design patterns

- **Actor Model**: Isolated actors with exclusive state access, async communication via `Executable` commands.
- **Command Pattern**: `GoCommand` wraps operations as objects; actors execute them, entities process them.
- **Observer Pattern**: `Hooks` and the system event bus.
- **Behavior Pattern**: `BehaviorStack` via `Become` / `Unbecome`.
- **Supervision Pattern**: Supervisor with restart policies and death watch.
- **Proxy Pattern**: `Ref` hides lifecycle internals.
- **Chain of Responsibility**: Middleware pipeline.

### Concurrency model

- One actor = one goroutine. Bounded channels provide backpressure.
- Stop uses atomic CAS; a dedicated stop channel avoids Receive/Stop races.
- Configurable receive timeouts; `ErrActorReceiveTimeout` when the buffer is full.
- All tests run with `-race`.

### Dependencies

- **go-datalib** (`github.com/barnowlsnest/go-datalib`) — Heap for priority mailbox.
- **testify** — Test assertions and suites (test-only).

### PlantUML diagrams

Available in [`docs/`](./docs/README.md): architecture overview, component relationships, actor lifecycle, command flow, message passing, supervision, behavior change, system lifecycle.

### Typical usage flow

1. Define entity implementing `Entity`, create a provider implementing `EntityProvider[T]`.
2. Create and start: `actor.StartNew(ctx, timeout, WithProvider(provider), opts...)` or `New` → `Start` → `WaitReady`.
3. Obtain a ref: `ref, err := actorref.New(myActor)`.
4. Or spawn: `ref, err := system.Spawn(sys, ctx, "name", provider, timeout, opts...)`.
5. Send: `ref.Send(ctx, command.New(delegateFn))` or `system.Send` / `ask.New` / `system.Ask`.
6. Get results: `<-cmd.Done()`, then `cmd.Error()`.
7. Dynamic behavior: `actor.GetGoActorContext[T](ctx).Become(newHandler)`.
8. Shutdown: `ref.Stop(timeout)` or `sys.StopAll(timeout)` or `signal.AwaitShutdown(ctx, sys, timeout)`.

## Code Conventions

- Heavy use of Go generics for type safety — avoid raw `interface{}`.
- All concurrency is channel-based with atomic state management; no shared mutable state between actors.
- Panic recovery is built into actor and command execution; panics become errors via `errors.Join()`.
- Test naming convention: `TestX_Condition_ShouldY` (behavior-driven).
- Linting: golangci-lint **v2**, 22 linters enabled (`default: none` then an explicit enable list). Key limits: line length 140, cyclomatic complexity 15, function length 100 lines / 50 statements. Test files are excluded from funlen, dupl, goconst, gocyclo, gosec. Formatters: gofmt + goimports (local prefix: `github.com/barnowlsnest/go-actorlib`).
