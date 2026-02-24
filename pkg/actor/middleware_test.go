package actor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// MiddlewareTestSuite provides a test suite for middleware functionality.
type MiddlewareTestSuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	entity   *TestEntity
	provider *TestEntityProvider
	hooks    *TestHooks
}

func (s *MiddlewareTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.entity = NewTestEntity("test-value", true)
	s.provider = NewTestEntityProvider(s.entity)
	s.hooks = NewTestHooks()
}

func (s *MiddlewareTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

// --- Chain tests ---

func (s *MiddlewareTestSuite) TestChain_Empty_ShouldPassThrough() {
	// Arrange
	var called bool
	base := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) {
		called = true
	}

	// Act
	handler := Chain[*TestEntity]()(base)
	handler(s.ctx, NewTestCommand("test", nil), s.entity)

	// Assert
	s.True(called)
}

func (s *MiddlewareTestSuite) TestChain_Single_ShouldWrapHandler() {
	// Arrange
	var order []string
	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			order = append(order, "before")
			next(ctx, e, entity)
			order = append(order, "after")
		}
	}
	base := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) {
		order = append(order, "handler")
	}

	// Act
	handler := Chain(mw)(base)
	handler(s.ctx, NewTestCommand("test", nil), s.entity)

	// Assert
	s.Equal([]string{"before", "handler", "after"}, order)
}

func (s *MiddlewareTestSuite) TestChain_Multiple_ShouldExecuteInOrder() {
	// Arrange
	var order []string
	makeMW := func(name string) Middleware[*TestEntity] {
		return func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
			return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
				order = append(order, name+"-before")
				next(ctx, e, entity)
				order = append(order, name+"-after")
			}
		}
	}
	base := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) {
		order = append(order, "handler")
	}

	// Act — Chain(A, B, C) should execute as A → B → C → handler → C → B → A
	handler := Chain(makeMW("A"), makeMW("B"), makeMW("C"))(base)
	handler(s.ctx, NewTestCommand("test", nil), s.entity)

	// Assert
	s.Equal([]string{
		"A-before", "B-before", "C-before",
		"handler",
		"C-after", "B-after", "A-after",
	}, order)
}

// --- WithMiddleware tests ---

func (s *MiddlewareTestSuite) TestWithMiddleware_Single_ShouldConfigure() {
	// Arrange
	var called bool
	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			called = true
			next(ctx, e, entity)
		}
	}

	// Act
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithMiddleware(mw),
	)

	// Assert
	s.NoError(err)
	s.Len(actor.middleware, 1)

	err = actor.Start(s.ctx)
	s.NoError(err)
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	cmd := NewTestCommand("test", nil)
	s.NoError(actor.Receive(s.ctx, cmd))

	time.Sleep(20 * time.Millisecond)
	s.True(called)
}

func (s *MiddlewareTestSuite) TestWithMiddleware_MultipleCalls_ShouldAppend() {
	// Arrange
	noop1 := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] { return next }
	noop2 := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] { return next }
	noop3 := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] { return next }

	// Act
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithMiddleware(noop1),
		WithMiddleware(noop2, noop3),
	)

	// Assert
	s.NoError(err)
	s.Len(actor.middleware, 3)
}

// --- Integration tests ---

func (s *MiddlewareTestSuite) TestActor_WithMiddleware_ShouldProcessCommandsThroughChain() {
	// Arrange
	var order []string
	var mu sync.Mutex
	appendOrder := func(v string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, v)
	}

	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			appendOrder("middleware-before")
			next(ctx, e, entity)
			appendOrder("middleware-after")
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithMiddleware(mw),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	done := make(chan struct{})
	cmd := NewTestCommand("test", func(_ context.Context, entity *TestEntity) {
		appendOrder("execute")
		close(done)
	})

	// Act
	s.NoError(actor.Receive(s.ctx, cmd))
	<-done

	// Allow middleware-after to complete
	time.Sleep(10 * time.Millisecond)

	// Assert
	mu.Lock()
	defer mu.Unlock()
	s.Equal([]string{"middleware-before", "execute", "middleware-after"}, order)
}

func (s *MiddlewareTestSuite) TestActor_WithMultipleMiddleware_ShouldExecuteInOrder() {
	// Arrange
	var order []string
	var mu sync.Mutex
	appendOrder := func(v string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, v)
	}

	makeMW := func(name string) Middleware[*TestEntity] {
		return func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
			return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
				appendOrder(name + "-before")
				next(ctx, e, entity)
				appendOrder(name + "-after")
			}
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithMiddleware(makeMW("A"), makeMW("B")),
		WithMiddleware(makeMW("C")),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	done := make(chan struct{})
	cmd := NewTestCommand("test", func(_ context.Context, _ *TestEntity) {
		appendOrder("execute")
		close(done)
	})

	// Act
	s.NoError(actor.Receive(s.ctx, cmd))
	<-done
	time.Sleep(10 * time.Millisecond)

	// Assert
	mu.Lock()
	defer mu.Unlock()
	s.Equal([]string{
		"A-before", "B-before", "C-before",
		"execute",
		"C-after", "B-after", "A-after",
	}, order)
}

func (s *MiddlewareTestSuite) TestActor_MultipleCommands_ShouldAllInvokeMiddleware() {
	// Arrange
	var count int
	var mu sync.Mutex
	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			mu.Lock()
			count++
			mu.Unlock()
			next(ctx, e, entity)
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithInputBufferSize[*TestEntity](10),
		WithMiddleware(mw),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	// Act — send 5 commands
	for i := 0; i < 5; i++ {
		cmd := NewTestCommand(fmt.Sprintf("cmd-%d", i), nil)
		s.NoError(actor.Receive(s.ctx, cmd))
	}

	time.Sleep(50 * time.Millisecond)

	// Assert
	mu.Lock()
	defer mu.Unlock()
	s.Equal(5, count)
}

// --- Edge cases ---

func (s *MiddlewareTestSuite) TestMiddleware_ShortCircuit_ShouldSkipExecution() {
	// Arrange — middleware that does not call next
	mw := func(_ HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) {
			// intentionally not calling next
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithMiddleware(mw),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	cmd := NewTestCommand("test", func(_ context.Context, entity *TestEntity) {
		entity.SetValue("should-not-happen")
	})

	// Act
	s.NoError(actor.Receive(s.ctx, cmd))
	time.Sleep(20 * time.Millisecond)

	// Assert — command was not executed, entity unchanged
	s.False(cmd.IsExecuted())
	s.Equal("test-value", s.entity.GetValue())
}

func (s *MiddlewareTestSuite) TestMiddleware_ModifiesContext_ShouldPropagateToHandler() {
	// Arrange
	type ctxKey struct{}
	var receivedVal string
	var mu sync.Mutex

	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			next(context.WithValue(ctx, ctxKey{}, "injected"), e, entity)
		}
	}

	innerMW := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			mu.Lock()
			receivedVal, _ = ctx.Value(ctxKey{}).(string)
			mu.Unlock()
			next(ctx, e, entity)
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithMiddleware(mw, innerMW),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	cmd := NewTestCommand("test", nil)

	// Act
	s.NoError(actor.Receive(s.ctx, cmd))
	time.Sleep(20 * time.Millisecond)

	// Assert
	mu.Lock()
	defer mu.Unlock()
	s.Equal("injected", receivedVal)
}

func (s *MiddlewareTestSuite) TestMiddleware_PanicInMiddleware_ShouldBeCaughtByActor() {
	// Arrange
	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) {
			panic("middleware panic")
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](s.hooks),
		WithMiddleware(mw),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	cmd := NewTestCommand("test", nil)

	// Act
	s.NoError(actor.Receive(s.ctx, cmd))
	time.Sleep(20 * time.Millisecond)

	// Assert — actor caught the panic
	s.Equal(uint64(Panicked), actor.State())
	hookErrors := s.hooks.GetErrors()
	s.NotEmpty(hookErrors)
}

func (s *MiddlewareTestSuite) TestMiddleware_PanicWithoutRecovery_ShouldPropagateToCatchPanic() {
	// Arrange — a panic inside the command (after middleware) should still be caught
	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			next(ctx, e, entity)
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](s.hooks),
		WithMiddleware(mw),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	cmd := NewTestCommand("test", func(_ context.Context, _ *TestEntity) {
		panic("command panic")
	})

	// Act
	s.NoError(actor.Receive(s.ctx, cmd))
	time.Sleep(20 * time.Millisecond)

	// Assert
	s.Equal(uint64(Panicked), actor.State())
}

func (s *MiddlewareTestSuite) TestMiddleware_RecoveryMiddleware_ShouldPreventActorPanic() {
	// Arrange — recovery middleware that catches panics
	recoveryMW := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			defer func() {
				recover() //nolint:errcheck // intentional recovery in test
			}()
			next(ctx, e, entity)
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithMiddleware(recoveryMW),
	)
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	panicCmd := NewTestCommand("panic", func(_ context.Context, _ *TestEntity) {
		panic("recovered panic")
	})

	// Act
	s.NoError(actor.Receive(s.ctx, panicCmd))
	time.Sleep(20 * time.Millisecond)

	// Assert — actor should still be running since recovery middleware caught the panic
	s.Equal(uint64(Started), actor.State())

	// Verify actor can still process commands after recovery
	done := make(chan struct{})
	nextCmd := NewTestCommand("after-recovery", func(_ context.Context, _ *TestEntity) {
		close(done)
	})
	s.NoError(actor.Receive(s.ctx, nextCmd))

	select {
	case <-done:
		// success
	case <-time.After(100 * time.Millisecond):
		s.Fail("timed out waiting for command after recovery")
	}
}

func (s *MiddlewareTestSuite) TestActor_WithoutMiddleware_ShouldHaveZeroOverhead() {
	// Arrange — no middleware configured
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)
	s.NoError(actor.Start(s.ctx))
	s.NoError(actor.WaitReady(s.ctx, 100*time.Millisecond))

	// Act — handler should still be set (direct execute)
	s.NotNil(actor.handler)

	done := make(chan struct{})
	cmd := NewTestCommand("test", func(_ context.Context, entity *TestEntity) {
		entity.SetValue("direct")
		close(done)
	})
	s.NoError(actor.Receive(s.ctx, cmd))
	<-done

	// Assert
	s.Equal("direct", s.entity.GetValue())
}

// --- Benchmarks ---

func BenchmarkGoActor_NoMiddleware(b *testing.B) {
	entity := NewTestEntity("bench", true)
	provider := NewTestEntityProvider(entity)

	actor, err := New(WithProvider[*TestEntity](provider))
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	if err := actor.Start(ctx); err != nil {
		b.Fatal(err)
	}
	if err := actor.WaitReady(ctx, time.Second); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := NewTestCommand("bench", func(_ context.Context, e *TestEntity) {
			e.GetValue()
		})
		if err := actor.Receive(ctx, cmd); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	if err := actor.Stop(time.Second); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkGoActor_SingleMiddleware(b *testing.B) {
	entity := NewTestEntity("bench", true)
	provider := NewTestEntityProvider(entity)

	mw := func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
		return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			next(ctx, e, entity)
		}
	}

	actor, err := New(
		WithProvider[*TestEntity](provider),
		WithMiddleware(mw),
	)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	if err := actor.Start(ctx); err != nil {
		b.Fatal(err)
	}
	if err := actor.WaitReady(ctx, time.Second); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := NewTestCommand("bench", func(_ context.Context, e *TestEntity) {
			e.GetValue()
		})
		if err := actor.Receive(ctx, cmd); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	if err := actor.Stop(time.Second); err != nil {
		b.Fatal(err)
	}
}

func BenchmarkGoActor_FiveMiddlewares(b *testing.B) {
	entity := NewTestEntity("bench", true)
	provider := NewTestEntityProvider(entity)

	makeMW := func() Middleware[*TestEntity] {
		return func(next HandlerFunc[*TestEntity]) HandlerFunc[*TestEntity] {
			return func(ctx context.Context, e Executable[*TestEntity], entity *TestEntity) {
				next(ctx, e, entity)
			}
		}
	}

	mws := make([]Middleware[*TestEntity], 5)
	for i := range mws {
		mws[i] = makeMW()
	}

	actor, err := New(
		WithProvider[*TestEntity](provider),
		WithMiddleware(mws...),
	)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	if err := actor.Start(ctx); err != nil {
		b.Fatal(err)
	}
	if err := actor.WaitReady(ctx, time.Second); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := NewTestCommand("bench", func(_ context.Context, e *TestEntity) {
			e.GetValue()
		})
		if err := actor.Receive(ctx, cmd); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	if err := actor.Stop(time.Second); err != nil {
		b.Fatal(err)
	}
}
