# go-actorlib Architecture Documentation

This directory contains PlantUML diagrams that visualize the architecture and behavior of the go-actorlib actor system.

## Diagrams

### 1. [Architecture Overview](./architecture.puml)
High-level view of the three main packages and their relationships:
- `pkg/actor` - Core actor implementation
- `pkg/command` - Command pattern implementation  
- `pkg/registry` - Actor lifecycle management

### 2. [Component Relationships](./component-relationships.puml)
Detailed class diagram showing:
- All interfaces and their implementations
- Generic type relationships
- Composition and dependency relationships
- Key methods and fields

### 3. [Actor Lifecycle](./actor-lifecycle.puml)
State machine diagram illustrating:
- Actor states (Initialized → Started → Stopping → Done/Error)
- State transitions and triggers
- Terminal states and error conditions

### 4. [Command Flow](./command-flow.puml)
Sequence diagram showing:
- Command creation and execution lifecycle
- Type-safe interaction between commands and entities
- Result handling and error propagation

### 5. [Message Passing](./message-passing.puml)
Detailed sequence diagram of the Actor Model message passing pattern:
- Asynchronous message sending and receiving
- Backpressure and flow control mechanisms
- Isolated state access guarantees

### 6. [Registry Operations](./registry-operations.puml)
Sequence diagram demonstrating:
- Actor registration and management
- Concurrent startup and shutdown operations
- Message dispatching by UUID

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
- No runtime type assertions needed

### 2. **Isolation** 
- Each actor runs in its own goroutine
- Entity state is only accessible within the actor's goroutine
- No shared mutable state between actors

### 3. **Asynchronous Communication**
- All inter-actor communication is via message passing
- Commands are queued and processed asynchronously
- Result channels provide non-blocking result delivery

### 4. **Lifecycle Management**
- Well-defined state machines for actors
- Coordinated startup and shutdown operations
- Comprehensive error handling and recovery

### 5. **Composability**
- Registry system manages collections of actors
- Hooks provide extension points for monitoring
- Functional options pattern for flexible configuration

## Design Patterns

The library implements several key design patterns:

- **Actor Model**: Core concurrency pattern with isolated actors
- **Command Pattern**: Encapsulated operations with results
- **Registry Pattern**: Centralized actor lifecycle management  
- **Observer Pattern**: Hooks for lifecycle events
- **Builder Pattern**: Functional options for configuration
- **State Machine**: Well-defined actor state transitions