# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

go-actorlib is a lightweight, type-safe Actor Model library for Go built on native concurrency primitives (goroutines and channels). Module: `github.com/barnowlsnest/go-actorlib/v2`, requires Go 1.25.

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

Run a single test:
```bash
go test -run TestName ./pkg/actor/
go test -run TestName ./pkg/command/
go test -run TestName ./pkg/ask/
```

Tests use testify suites, so individual test methods are run as subtests:
```bash
go test -run TestGoActorSuite/TestMethodName ./pkg/actor/
go test -run TestGoCommandSuite/TestMethodName ./pkg/command/
go test -run TestGoAskTestSuite/TestMethodName ./pkg/ask/
```

## Architecture

Three packages under `pkg/`:

### `pkg/actor` — Core actor implementation
- **`GoActor[T Entity]`** — Generic actor that manages an entity of type T in an isolated goroutine. Processes `Executable[T]` commands sequentially from a bounded input channel.
- **`Entity`** interface — Constraint for actor-managed data (requires `IsProvidable() bool`).
- **`Executable[T Entity]`** interface — Commands that can be sent to an actor via `Receive()`.
- **`Hooks`** interface — Lifecycle callbacks: BeforeStart, AfterStart, BeforeStop, AfterStop, OnError. Default is `noopHooks`.
- **State machine** (7 states via `sync/atomic`): Initialized → Started → Stopping → Done/StoppedWithError/Canceled/Panicked.
- Configured via functional options: `WithProvider`, `WithInputBufferSize`, `WithReceiveTimeout`, `WithHooks`.

### `pkg/command` — Command pattern for async operations
- **`GoCommand[E Entity, R any]`** — Wraps a `DelegateFn[E, R]` (func(entity E) (R, error)) as an `Executable[E]`. Returns results via a buffered channel (`Done()`).
- **State machine** (6 states via `sync.Mutex`): Created → Started → Finished/Failed/Canceled/Panic.

### `pkg/ask` — Ask pattern (request/response convenience)
- **`New[E Entity, R any]`** — Single-call request/response with timeout. Wraps command creation, `Receive`, and result waiting into one function. Returns `(R, error)`.
- **`ErrAskTimeout`** — Returned when the result is not received within the specified timeout.

### Design patterns
- **Actor Model**: Isolated actors with exclusive state access, async communication via `Executable` commands.
- **Command Pattern**: `GoCommand` wraps operations as objects; actors execute them, entities process them.
- **Observer Pattern**: `Hooks` interface provides lifecycle event notifications (start, stop, error).

### Concurrency model
- One actor = one goroutine. Bounded channels for message queuing provide natural backpressure.
- Configurable timeouts on send operations; errors propagated when buffers are full.

### PlantUML diagrams
Available in [`docs/`](./docs/README.md): architecture overview, component relationships, actor lifecycle state machine, command execution flow, message passing sequence.

### Typical usage flow
1. Define entity implementing `Entity`, create a provider implementing `EntityProvider[T]`.
2. Create actor: `actor.New(WithProvider(provider), options...)`.
3. Start: `actor.Start(ctx)`, then `actor.WaitReady(ctx, timeout)`.
4. Send commands: `actor.Receive(ctx, command.New(delegateFn))`.
5. Get results: `<-cmd.Done()`, check `cmd.Error()`.
6. Shutdown: `actor.Stop(timeout)`.

## Code Conventions

- Heavy use of Go generics for type safety — avoid raw `interface{}`.
- All concurrency is channel-based with atomic state management; no shared mutable state between actors.
- Panic recovery is built into actor and command execution; panics become errors via `errors.Join()`.
- Test naming convention: `TestX_Condition_ShouldY` (behavior-driven).
- Linting: golangci-lint with 25 linters enabled. Key limits: line length 140, cyclomatic complexity 15, function length 100 lines / 50 statements. Test files are excluded from funlen, dupl, goconst, gocyclo, gosec.