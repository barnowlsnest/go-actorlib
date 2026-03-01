# go-actorlib Architecture Documentation

This directory contains PlantUML diagrams that visualize the architecture and behavior of the go-actorlib actor system.

## Diagrams

### 1. [Architecture Overview](./architecture.puml)
High-level view of all ten packages and their relationships:
- `pkg/actor` — Core actor with middleware, behavior stack, actor context
- `pkg/actorref` — Typed actor handle (Ref)
- `pkg/command` — Command pattern implementation
- `pkg/ask` — Request/response convenience
- `pkg/system` — Actor system with registry, Spawn, event bus
- `pkg/supervision` — Supervisor with restart strategies and death watch
- `pkg/middleware` — Reference middleware (logging, metrics, recovery)
- `pkg/deadletter` — Dead letter queue for undeliverable messages
- `pkg/signal` — OS signal integration for graceful shutdown
- `pkg/mailbox` — Priority mailbox with configurable priority levels

### 2. [Component Relationships](./component-relationships.puml)
Detailed class diagram showing:
- All interfaces and their implementations
- Generic type relationships (`Entity`, `Executable<T>`, `Ref<T>`)
- BehaviorStack, GoActorContext, and middleware types
- System, Supervisor, and their interfaces (ManagedActor, ChildSpec, ChildRef)
- Dead letter queue, priority mailbox, and ask convenience
- Composition, inheritance, and dependency relationships

### 3. [Actor Lifecycle](./actor-lifecycle.puml)
State machine diagram illustrating:
- Actor states: Initialized → Started → Stopping → Done/Error/Cancelled/Panicked
- CAS-based Stop() with atomic.CompareAndSwapUint64
- Start() guard with CheckState(Initialized)
- Four channel types: input, ready, done, stop
- Hooks and middleware integration points

### 4. [Command Flow](./command-flow.puml)
Sequence diagram showing:
- Command creation and execution lifecycle
- Type-safe interaction between commands and entities
- Result handling and error propagation
- Command state machine: Created → Started → Finished/Failed/Canceled/Panic

### 5. [Message Passing](./message-passing.puml)
Detailed sequence diagram of the Actor Model message passing pattern:
- Ref as the entry point for all message sends
- Middleware chain processing before behavior handler
- BehaviorStack for dynamic handler selection
- Stop channel signaling for concurrent-safe shutdown
- Backpressure via bounded input channel
- Ask pattern as simplified request/response via Ref

### 6. [Supervision](./supervision.puml)
Sequence diagram of supervisor lifecycle management:
- OneForOne strategy: only failed child restarts
- AllForOne strategy: all children restart on any failure
- Version-tracked monitors preventing stale restart cascades
- Death watch with WatchCallback notifications
- Max restart frequency with sliding time window
- LIFO shutdown ordering

### 7. [Behavior Change](./behavior-change.puml)
Sequence diagram of dynamic behavior switching:
- BehaviorStack operations: Become (push), BecomeReplace (swap), Unbecome (pop)
- GoActorContext access via GetGoActorContext from context.Context
- Stack depth tracking and base handler protection
- Step-by-step example of behavior transitions during message processing

### 8. [System Lifecycle](./system-lifecycle.puml)
Sequence diagram of actor system operations:
- Spawn convenience: New → Start → WaitReady → Ref → Register in one call
- Event bus: EventActorStarted, EventActorStopped, EventSystemStopping
- Direct registration as alternative to Spawn
- Spawn failure with automatic cleanup
- LIFO shutdown ordering with signal integration
- system.Send and system.Ask for name-based messaging

## Viewing the Diagrams

### Online Viewers
- [PlantUML Online Server](http://www.plantuml.com/plantuml/uml/)
- [PlantText](https://www.planttext.com/)

### Local Tools
```bash
# Install PlantUML (requires Java)
# macOS with Homebrew
brew install plantuml

# Generate PNG images
plantuml docs/*.puml

# Generate SVG images
plantuml -tsvg docs/*.puml
```

### IDE Extensions
- **VS Code**: PlantUML extension
- **IntelliJ/GoLand**: PlantUML integration plugin
- **Vim**: plantuml-syntax

## Architecture Principles

The diagrams illustrate key architectural principles of go-actorlib:

### 1. **Type Safety**
- Generic interfaces ensure compile-time type safety
- Entities and commands are bound to specific types
- Ref[T] provides a type-safe proxy that hides actor internals

### 2. **Isolation**
- Each actor runs in its own goroutine
- Entity state is only accessible within the actor's goroutine
- BehaviorStack and GoActorContext are goroutine-local (no synchronization needed)
- No shared mutable state between actors

### 3. **Asynchronous Communication**
- All inter-actor communication is via message passing through Ref
- Commands are queued and processed asynchronously
- Result channels provide non-blocking result delivery
- Ask pattern simplifies request/response into a single call

### 4. **Lifecycle Management**
- Well-defined state machines for actors and commands
- CAS-based Stop() ensures concurrent safety
- Dedicated stop channel avoids Receive/Stop race conditions
- Coordinated system-level shutdown with LIFO ordering
- OS signal integration for graceful termination

### 5. **Supervision**
- Supervisor monitors child actors via death watch
- Restart strategies: OneForOne (isolated) and AllForOne (coordinated)
- Configurable max restart frequency prevents restart storms
- Version-tracked monitors prevent stale cascade restarts

### 6. **Dynamic Behavior**
- BehaviorStack enables actors to change message handling at runtime
- Become/Unbecome pattern for building protocol actors and state machines
- GoActorContext provides safe access to behavior operations during message processing

### 7. **Composability**
- Hooks provide extension points for lifecycle monitoring
- Middleware chain for cross-cutting concerns (logging, metrics, recovery)
- Functional options pattern for flexible configuration
- Event bus for system-wide lifecycle notifications

## Design Patterns

The library implements several key design patterns:

- **Actor Model**: Core concurrency pattern with isolated actors and async messaging
- **Command Pattern**: Encapsulated operations with typed results
- **Observer Pattern**: Hooks and event bus for lifecycle events
- **Builder Pattern**: Functional options for configuration
- **State Machine**: Well-defined actor and command state transitions
- **Behavior Pattern**: Dynamic handler switching via Become/Unbecome
- **Supervision Pattern**: Parent monitors children with configurable restart policies
- **Proxy Pattern**: Ref wraps actor internals, exposing only safe operations
- **Chain of Responsibility**: Middleware pipeline for message processing
