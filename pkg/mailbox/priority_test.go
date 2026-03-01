package mailbox

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v3/pkg/actor"
)

type testEntity struct{}

func (te *testEntity) IsProvidable() bool { return true }

type testCommand struct {
	name string
}

func (tc *testCommand) Execute(_ context.Context, _ *testEntity) {}

type PriorityMailboxTestSuite struct {
	suite.Suite
}

func TestPriorityMailboxTestSuite(t *testing.T) {
	suite.Run(t, new(PriorityMailboxTestSuite))
}

func (s *PriorityMailboxTestSuite) TestNewPriority_ShouldCreateEmptyMailbox() {
	mb := NewPriority[*testEntity](10)
	s.True(mb.IsEmpty())
	s.Equal(0, mb.Size())
}

func (s *PriorityMailboxTestSuite) TestPush_ShouldAddMessage() {
	mb := NewPriority[*testEntity](10)
	ok := mb.Push(&testCommand{name: "test"}, Normal)
	s.True(ok)
	s.Equal(1, mb.Size())
}

func (s *PriorityMailboxTestSuite) TestPush_Full_ShouldReturnFalse() {
	mb := NewPriority[*testEntity](1)
	s.True(mb.Push(&testCommand{name: "a"}, Normal))
	s.False(mb.Push(&testCommand{name: "b"}, Normal))
}

func (s *PriorityMailboxTestSuite) TestPush_Closed_ShouldReturnFalse() {
	mb := NewPriority[*testEntity](10)
	mb.Close()
	s.False(mb.Push(&testCommand{name: "a"}, Normal))
}

func (s *PriorityMailboxTestSuite) TestPop_Empty_ShouldReturnFalse() {
	mb := NewPriority[*testEntity](10)
	_, ok := mb.Pop()
	s.False(ok)
}

func (s *PriorityMailboxTestSuite) TestPriority_SystemBeforeNormal() {
	mb := NewPriority[*testEntity](10)
	mb.Push(&testCommand{name: "normal"}, Normal)
	mb.Push(&testCommand{name: "system"}, System)
	mb.Push(&testCommand{name: "low"}, Low)

	msg1, ok := mb.Pop()
	s.True(ok)
	s.Equal("system", msg1.(*testCommand).name)

	msg2, ok := mb.Pop()
	s.True(ok)
	s.Equal("normal", msg2.(*testCommand).name)

	msg3, ok := mb.Pop()
	s.True(ok)
	s.Equal("low", msg3.(*testCommand).name)
}

func (s *PriorityMailboxTestSuite) TestPriority_FIFOWithinSamePriority() {
	mb := NewPriority[*testEntity](10)
	mb.Push(&testCommand{name: "first"}, Normal)
	mb.Push(&testCommand{name: "second"}, Normal)
	mb.Push(&testCommand{name: "third"}, Normal)

	msg1, _ := mb.Pop()
	s.Equal("first", msg1.(*testCommand).name)

	msg2, _ := mb.Pop()
	s.Equal("second", msg2.(*testCommand).name)

	msg3, _ := mb.Pop()
	s.Equal("third", msg3.(*testCommand).name)
}

func (s *PriorityMailboxTestSuite) TestNotify_ShouldSignalOnPush() {
	mb := NewPriority[*testEntity](10)
	mb.Push(&testCommand{name: "test"}, Normal)

	select {
	case <-mb.Notify():
		// Success
	default:
		s.Fail("expected notify signal")
	}
}

func (s *PriorityMailboxTestSuite) TestConcurrentPushPop_ShouldBeSafe() {
	mb := NewPriority[*testEntity](1000)
	var wg sync.WaitGroup

	// Concurrent pushes
	for range 50 {
		wg.Go(func() {
			mb.Push(&testCommand{name: "concurrent"}, Normal)
		})
	}

	// Concurrent pops
	for range 50 {
		wg.Go(func() {
			mb.Pop()
		})
	}

	wg.Wait()
	// No panic = success
}

// Verify that PriorityMailbox works with the actor.Executable interface
func (s *PriorityMailboxTestSuite) TestPriorityMailbox_CompatibleWithExecutable() {
	mb := NewPriority[*testEntity](10)
	var cmd actor.Executable[*testEntity] = &testCommand{name: "typed"}
	ok := mb.Push(cmd, High)
	s.True(ok)

	result, ok := mb.Pop()
	s.True(ok)
	s.NotNil(result)
}
