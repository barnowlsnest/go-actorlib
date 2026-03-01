package system

// EventKind identifies the type of system event.
type EventKind int

const (
	// EventActorStarted is emitted when an actor is started via Spawn.
	EventActorStarted EventKind = iota

	// EventActorStopped is emitted when an actor is stopped.
	EventActorStopped

	// EventSystemStopping is emitted when StopAll begins.
	EventSystemStopping
)

// Event represents a system lifecycle event.
type Event struct {
	Kind      EventKind
	ActorName string
}

// EventHandler is a callback for system events.
type EventHandler func(Event)

// OnEvent registers an event handler that is called for all system events.
// Multiple handlers can be registered.
func (s *ActorSystem) OnEvent(handler EventHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventHandlers = append(s.eventHandlers, handler)
}

// emitEvent notifies all registered event handlers.
// Handlers are called synchronously in registration order.
func (s *ActorSystem) emitEvent(event Event) {
	s.mu.RLock()
	handlers := make([]EventHandler, len(s.eventHandlers))
	copy(handlers, s.eventHandlers)
	s.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}
