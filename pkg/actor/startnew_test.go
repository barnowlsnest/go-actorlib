package actor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type StartNewTestSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *StartNewTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *StartNewTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestStartNewTestSuite(t *testing.T) {
	suite.Run(t, new(StartNewTestSuite))
}

func (s *StartNewTestSuite) TestStartNew_ValidProvider_ShouldReturnStartedActor() {
	// Arrange
	entity := NewTestEntity("test", true)
	provider := NewTestEntityProvider(entity)

	// Act
	a, err := StartNew(s.ctx, 100*time.Millisecond, WithProvider(provider))

	// Assert
	s.NoError(err)
	s.NotNil(a)
	s.Equal(uint64(Started), a.State())
}

func (s *StartNewTestSuite) TestStartNew_NilProvider_ShouldReturnError() {
	// Act
	a, err := StartNew[*TestEntity](s.ctx, 100*time.Millisecond)

	// Assert
	s.Error(err)
	s.Nil(a)
	s.Equal(ErrActorNilProvider, err)
}

func (s *StartNewTestSuite) TestStartNew_WithOptions_ShouldApply() {
	// Arrange
	entity := NewTestEntity("test", true)
	provider := NewTestEntityProvider(entity)

	// Act
	a, err := StartNew(s.ctx, 100*time.Millisecond,
		WithProvider(provider),
		WithInputBufferSize[*TestEntity](10),
		WithName[*TestEntity]("my-actor"),
	)

	// Assert
	s.NoError(err)
	s.Equal(10, a.InputBufferSize())
	s.Equal("my-actor", a.Name())
}
