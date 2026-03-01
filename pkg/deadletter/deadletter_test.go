package deadletter

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
)

type DeadLetterTestSuite struct {
	suite.Suite
}

func TestDeadLetterTestSuite(t *testing.T) {
	suite.Run(t, new(DeadLetterTestSuite))
}

func (s *DeadLetterTestSuite) TestNew_Default_ShouldCreateEmptyQueue() {
	q := New()
	s.Equal(0, q.Count())
	s.Empty(q.Letters())
}

func (s *DeadLetterTestSuite) TestPublish_ShouldAddLetter() {
	q := New()
	q.Publish(Letter{Target: "worker", Reason: "stopped"})

	s.Equal(1, q.Count())
	letters := q.Letters()
	s.Len(letters, 1)
	s.Equal("worker", letters[0].Target)
	s.Equal("stopped", letters[0].Reason)
}

func (s *DeadLetterTestSuite) TestPublish_ShouldNotifyHandlers() {
	q := New()
	var received Letter
	q.OnDeadLetter(func(l Letter) {
		received = l
	})

	q.Publish(Letter{Target: "actor-1", Reason: "panicked"})

	s.Equal("actor-1", received.Target)
	s.Equal("panicked", received.Reason)
}

func (s *DeadLetterTestSuite) TestPublish_Capacity_ShouldEvictOldest() {
	q := New(WithCapacity(2))
	q.Publish(Letter{Target: "a", Reason: "1"})
	q.Publish(Letter{Target: "b", Reason: "2"})
	q.Publish(Letter{Target: "c", Reason: "3"})

	s.Equal(2, q.Count())
	letters := q.Letters()
	s.Equal("b", letters[0].Target)
	s.Equal("c", letters[1].Target)
}

func (s *DeadLetterTestSuite) TestClear_ShouldRemoveAll() {
	q := New()
	q.Publish(Letter{Target: "a", Reason: "1"})
	q.Publish(Letter{Target: "b", Reason: "2"})
	q.Clear()

	s.Equal(0, q.Count())
}

func (s *DeadLetterTestSuite) TestConcurrentPublish_ShouldBeSafe() {
	q := New()
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Go(func() {
			q.Publish(Letter{Target: "actor", Reason: string(rune('a' + i%26))})
		})
	}
	wg.Wait()

	s.Equal(100, q.Count())
}

func (s *DeadLetterTestSuite) TestLetters_ShouldReturnCopy() {
	q := New()
	q.Publish(Letter{Target: "a", Reason: "1"})
	letters := q.Letters()
	letters[0].Target = "modified"

	// Original should be unchanged
	s.Equal("a", q.Letters()[0].Target)
}
