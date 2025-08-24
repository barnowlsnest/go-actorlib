# go-actorlib Architecture

This document provides a detailed overview of the go-actorlib architecture, design principles, and implementation patterns.

## Overview

go-actorlib is a lightweight, type-safe implementation of the Actor Model pattern for Go. The library is designed around three core packages that work together to provide a complete actor system:

- **`pkg/actor`** - Core actor implementation with lifecycle management
- **`pkg/command`** - Command pattern for type-safe operations with results  
- **`pkg/registry`** - Centralized actor management and coordination

## Visual Architecture

Detailed PlantUML diagrams are available in the [`docs/`](./docs/) directory:

- [Architecture Overview](./docs/architecture.puml) - High-level package relationships
- [Component Relationships](./docs/component-relationships.puml) - Detailed class diagram
- [Actor Lifecycle](./docs/actor-lifecycle.puml) - State machine diagram
- [Command Flow](./docs/command-flow.puml) - Command execution sequence
- [Message Passing](./docs/message-passing.puml) - Actor Model message flow
- [Registry Operations](./docs/registry-operations.puml) - Registry coordination

## Core Components

**pkg/actor** - The fundamental actor implementation
- `GoActor[T Entity]` - Generic actor that processes `Executable[T]` commands asynchronously
- Uses channels for message passing and atomic state management
- Supports lifecycle hooks, configurable timeouts, and panic recovery
- States: Initialized → Started → Stopping → Done/StoppedWithError/Cancelled/Panicked

**pkg/command** - Command pattern implementation
- `GoCommand[E Entity, R any]` - Wraps operations that return values
- Implements `Executable[E]` interface to work with actors
- Thread-safe state tracking with result channels
- States: Created → Started → Finished/Failed/Canceled/Panic

**pkg/registry** - Actor lifecycle management
- `GoRegistry` - Manages multiple actors with UUID-based identification
- Provides Start/Stop operations for individual actors or all registered actors
- Thread-safe operations with concurrent startup/shutdown using errgroups
- Name-based lookup with state validation

## Key Patterns

**Type Safety**: Heavy use of Go generics for compile-time type safety
- `Entity` interface ensures actor-managed data is valid
- `Executable[T Entity]` binds commands to specific entity types
- `DelegateFn[T Entity, R any]` provides type-safe operation wrappers

**Concurrency**: Built on goroutines and channels
- Actors run in separate goroutines with message queues
- Registry operations use errgroups for concurrent actor management
- Atomic operations for thread-safe state updates

**Error Handling**: Comprehensive error propagation
- Context cancellation support throughout
- Panic recovery with proper error conversion
- Detailed error joining for multiple failure scenarios

## Design Patterns

### Actor Model
The core pattern implemented throughout the library:
- **Actors**: Lightweight, isolated entities with exclusive state access
- **Messages**: Asynchronous communication via `Executable` commands
- **No Shared State**: All communication through message passing

### Command Pattern  
Encapsulates operations as objects:
- **Commands**: `GoCommand` wraps operations with results
- **Invoker**: Actors execute commands asynchronously
- **Receiver**: Entity receives and processes operations

### Registry Pattern
Centralized service management:
- **Registration**: Actors register with unique names and UUIDs
- **Lifecycle Management**: Coordinated start/stop operations
- **Service Discovery**: Lookup actors by name or UUID

### Observer Pattern
Event notification system:
- **Hooks**: Lifecycle event handlers for monitoring
- **Subject**: Actors notify hooks of state changes
- **Observer**: Hook implementations respond to events

## Concurrency Model

### Goroutine Usage
- **One Actor = One Goroutine**: Each actor runs in a dedicated goroutine
- **Channel Communication**: Bounded channels for message queuing
- **Registry Coordination**: Separate goroutines for concurrent operations

### Thread Safety
- **Actor Isolation**: No shared mutable state between actors
- **Registry Locks**: Minimal locking for registry operations
- **Command Safety**: Thread-safe command state management

### Backpressure
- **Bounded Channels**: Input buffers provide natural backpressure  
- **Timeout Handling**: Configurable timeouts for send operations
- **Error Propagation**: Proper error handling when buffers are full
