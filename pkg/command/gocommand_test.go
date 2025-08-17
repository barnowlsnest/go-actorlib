package command

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestEntity is a mock entity for testing purposes
type TestEntity struct {
	value   string
	isReady bool
	callLog []string
	mu      sync.Mutex
}

func NewTestEntity(value string, isReady bool) *TestEntity {
	return &TestEntity{
		value:   value,
		isReady: isReady,
		callLog: make([]string, 0),
	}
}

func (te *TestEntity) IsProvidable() bool {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = append(te.callLog, "IsProvidable")
	return te.isReady
}

func (te *TestEntity) GetValue() string {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = append(te.callLog, "GetValue")
	return te.value
}

func (te *TestEntity) SetValue(value string) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = append(te.callLog, fmt.Sprintf("SetValue:%s", value))
	te.value = value
}

func (te *TestEntity) GetCallLog() []string {
	te.mu.Lock()
	defer te.mu.Unlock()
	result := make([]string, len(te.callLog))
	copy(result, te.callLog)
	return result
}

func (te *TestEntity) ClearCallLog() {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = te.callLog[:0]
}

// GoCommandTestSuite provides a test suite for GoCommand
type GoCommandTestSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	entity *TestEntity
}

func (suite *GoCommandTestSuite) SetupTest() {
	suite.ctx, suite.cancel = context.WithCancel(context.Background())
	suite.entity = NewTestEntity("test-value", true)
}

func (suite *GoCommandTestSuite) TearDownTest() {
	if suite.cancel != nil {
		suite.cancel()
	}
}

func TestGoCommandTestSuite(t *testing.T) {
	suite.Run(t, new(GoCommandTestSuite))
}

// Test command creation and initialization
func (suite *GoCommandTestSuite) TestNew_ShouldCreateCommandWithCorrectInitialState() {
	// Arrange
	delegateFn := func(entity *TestEntity) (string, error) {
		return entity.GetValue(), nil
	}
	
	// Act
	cmd := New(delegateFn)
	
	// Assert
	require.NotNil(suite.T(), cmd)
	assert.Equal(suite.T(), Created, cmd.State())
	assert.Nil(suite.T(), cmd.Error())
	assert.NotNil(suite.T(), cmd.Done())
	assert.NotNil(suite.T(), cmd.delegateFn)
}

func (suite *GoCommandTestSuite) TestNew_ShouldCreateChannelWithBufferSizeOne() {
	// Arrange
	delegateFn := func(entity *TestEntity) (int, error) {
		return 42, nil
	}
	
	// Act
	cmd := New(delegateFn)
	
	// Assert
	require.NotNil(suite.T(), cmd)
	
	// Test that channel has buffer size 1 by sending without blocking
	cmd.done <- 123
	select {
	case value := <-cmd.Done():
		assert.Equal(suite.T(), 123, value)
	default:
		suite.T().Fatal("Channel should have received value")
	}
}

// Test successful command execution
func (suite *GoCommandTestSuite) TestExecute_SuccessfulExecution_ShouldReturnCorrectResult() {
	// Arrange
	expectedResult := "processed-test-value"
	delegateFn := func(entity *TestEntity) (string, error) {
		return expectedResult, nil
	}
	cmd := New(delegateFn)
	
	// Act
	cmd.Execute(suite.ctx, suite.entity)
	
	// Assert
	select {
	case result := <-cmd.Done():
		assert.Equal(suite.T(), expectedResult, result)
	case <-time.After(100 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	assert.Equal(suite.T(), Finished, cmd.State())
	assert.Nil(suite.T(), cmd.Error())
}

func (suite *GoCommandTestSuite) TestExecute_SuccessfulExecution_ShouldInteractWithEntity() {
	// Arrange
	delegateFn := func(entity *TestEntity) (string, error) {
		entity.SetValue("updated-value")
		return entity.GetValue(), nil
	}
	cmd := New(delegateFn)
	
	// Act
	cmd.Execute(suite.ctx, suite.entity)
	
	// Assert
	select {
	case result := <-cmd.Done():
		assert.Equal(suite.T(), "updated-value", result)
	case <-time.After(100 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	callLog := suite.entity.GetCallLog()
	assert.Contains(suite.T(), callLog, "SetValue:updated-value")
	assert.Contains(suite.T(), callLog, "GetValue")
}

// Test command execution with delegate function error
func (suite *GoCommandTestSuite) TestExecute_DelegateFunctionError_ShouldSetFailedState() {
	// Arrange
	expectedError := errors.New("delegate function failed")
	delegateFn := func(entity *TestEntity) (string, error) {
		return "", expectedError
	}
	cmd := New(delegateFn)
	
	// Act
	cmd.Execute(suite.ctx, suite.entity)
	
	// Assert
	select {
	case <-cmd.Done():
		// Channel should be closed but empty
	case <-time.After(100 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	assert.Equal(suite.T(), Failed, cmd.State())
	assert.Equal(suite.T(), expectedError, cmd.Error())
}

// Test context cancellation
func (suite *GoCommandTestSuite) TestExecute_ContextCancellation_ShouldSetCanceledState() {
	// Arrange
	delegateFn := func(entity *TestEntity) (string, error) {
		return "should not reach here", nil
	}
	cmd := New(delegateFn)
	
	// Cancel context before execution
	suite.cancel()
	
	// Act
	cmd.Execute(suite.ctx, suite.entity)
	
	// Assert
	select {
	case <-cmd.Done():
		// Channel should be closed but empty
	case <-time.After(100 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	assert.Equal(suite.T(), Canceled, cmd.State())
	assert.NotNil(suite.T(), cmd.Error())
	assert.ErrorIs(suite.T(), cmd.Error(), ErrCommandContextCancelled)
	assert.ErrorIs(suite.T(), cmd.Error(), context.Canceled)
}

func (suite *GoCommandTestSuite) TestExecute_ContextCancellationDuringExecution_ShouldNotAffectRunningCommand() {
	// Arrange
	started := make(chan struct{})
	delegateFn := func(entity *TestEntity) (string, error) {
		close(started)
		// Simulate some work that takes time
		time.Sleep(50 * time.Millisecond)
		return "completed", nil
	}
	cmd := New(delegateFn)
	
	// Act
	go cmd.Execute(suite.ctx, suite.entity)
	
	// Wait for execution to start, then cancel context
	<-started
	suite.cancel()
	
	// Assert
	select {
	case result := <-cmd.Done():
		// Should complete successfully despite context cancellation
		assert.Equal(suite.T(), "completed", result)
	case <-time.After(200 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	assert.Equal(suite.T(), Finished, cmd.State())
	assert.Nil(suite.T(), cmd.Error())
}

// Test panic recovery
func (suite *GoCommandTestSuite) TestExecute_DelegateFunctionPanic_ShouldRecoverAndSetPanicState() {
	// Arrange
	delegateFn := func(entity *TestEntity) (string, error) {
		panic("something went wrong")
	}
	cmd := New(delegateFn)
	
	// Act
	cmd.Execute(suite.ctx, suite.entity)
	
	// Assert
	select {
	case <-cmd.Done():
		// Channel should be closed but empty
	case <-time.After(100 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	assert.Equal(suite.T(), Panic, cmd.State())
	assert.Equal(suite.T(), ErrCommandPanic, cmd.Error())
}

func (suite *GoCommandTestSuite) TestExecute_DelegateFunctionPanicWithDifferentTypes_ShouldRecoverCorrectly() {
	testCases := []struct {
		name       string
		panicValue interface{}
	}{
		{"string panic", "string panic message"},
		{"error panic", errors.New("error panic message")},
		{"int panic", 42},
		{"nil panic", nil},
		{"struct panic", struct{ msg string }{"struct panic"}},
	}
	
	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			// Arrange
			delegateFn := func(entity *TestEntity) (string, error) {
				panic(tc.panicValue)
			}
			cmd := New(delegateFn)
			
			// Act
			cmd.Execute(suite.ctx, suite.entity)
			
			// Assert
			select {
			case <-cmd.Done():
				// Channel should be closed but empty
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Command should have completed")
			}
			
			assert.Equal(t, Panic, cmd.State())
			assert.Equal(t, ErrCommandPanic, cmd.Error())
		})
	}
}

// Test state transitions
func (suite *GoCommandTestSuite) TestStateTransitions_ShouldFollowCorrectSequence() {
	// Arrange
	delegateFn := func(entity *TestEntity) (string, error) {
		return "result", nil
	}
	cmd := New(delegateFn)
	
	// Assert initial state
	assert.Equal(suite.T(), Created, cmd.State())
	
	// Act & Assert state during execution
	go cmd.Execute(suite.ctx, suite.entity)
	
	// Wait briefly to ensure execution starts
	time.Sleep(10 * time.Millisecond)
	
	// State should be Started or Finished (depends on timing)
	state := cmd.State()
	assert.True(suite.T(), state == Started || state == Finished)
	
	// Wait for completion
	select {
	case <-cmd.Done():
	case <-time.After(100 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	// Final state should be Finished
	assert.Equal(suite.T(), Finished, cmd.State())
}

// Test concurrent access to command state
func (suite *GoCommandTestSuite) TestConcurrentAccess_ShouldBeSafe() {
	// Arrange
	delegateFn := func(entity *TestEntity) (int, error) {
		time.Sleep(50 * time.Millisecond)
		return 42, nil
	}
	cmd := New(delegateFn)
	
	// Act - Start command execution and concurrent state access
	var wg sync.WaitGroup
	
	// Start execution
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd.Execute(suite.ctx, suite.entity)
	}()
	
	// Concurrent state reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				state := cmd.State()
				err := cmd.Error()
				// States should be valid
				assert.True(suite.T(), state >= Created && state <= Panic)
				// Error should be nil or a valid error
				_ = err
			}
		}()
	}
	
	// Wait for all goroutines
	wg.Wait()
	
	// Assert final state
	select {
	case result := <-cmd.Done():
		assert.Equal(suite.T(), 42, result)
	case <-time.After(100 * time.Millisecond):
		suite.T().Fatal("Command should have completed")
	}
	
	assert.Equal(suite.T(), Finished, cmd.State())
}

// Test command with different result types
func (suite *GoCommandTestSuite) TestExecute_DifferentResultTypes_ShouldWork() {
	testCases := []struct {
		name     string
		delegate interface{}
		expected interface{}
	}{
		{
			name: "string result",
			delegate: func(entity *TestEntity) (string, error) {
				return "string result", nil
			},
			expected: "string result",
		},
		{
			name: "int result",
			delegate: func(entity *TestEntity) (int, error) {
				return 42, nil
			},
			expected: 42,
		},
		{
			name: "bool result",
			delegate: func(entity *TestEntity) (bool, error) {
				return true, nil
			},
			expected: true,
		},
		{
			name: "struct result",
			delegate: func(entity *TestEntity) (struct{ Value string }, error) {
				return struct{ Value string }{"struct result"}, nil
			},
			expected: struct{ Value string }{"struct result"},
		},
		{
			name: "pointer result",
			delegate: func(entity *TestEntity) (*string, error) {
				result := "pointer result"
				return &result, nil
			},
			expected: func() *string { s := "pointer result"; return &s }(),
		},
	}
	
	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			var state int
			switch fn := tc.delegate.(type) {
			case func(*TestEntity) (string, error):
				cmd := New(fn)
				cmd.Execute(suite.ctx, suite.entity)
				select {
				case result := <-cmd.Done():
					assert.Equal(t, tc.expected, result)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Command should have completed")
				}
				state = cmd.State()
			case func(*TestEntity) (int, error):
				cmd := New(fn)
				cmd.Execute(suite.ctx, suite.entity)
				select {
				case result := <-cmd.Done():
					assert.Equal(t, tc.expected, result)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Command should have completed")
				}
				state = cmd.State()
			case func(*TestEntity) (bool, error):
				cmd := New(fn)
				cmd.Execute(suite.ctx, suite.entity)
				select {
				case result := <-cmd.Done():
					assert.Equal(t, tc.expected, result)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Command should have completed")
				}
				state = cmd.State()
			case func(*TestEntity) (struct{ Value string }, error):
				cmd := New(fn)
				cmd.Execute(suite.ctx, suite.entity)
				select {
				case result := <-cmd.Done():
					assert.Equal(t, tc.expected, result)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Command should have completed")
				}
				state = cmd.State()
			case func(*TestEntity) (*string, error):
				cmd := New(fn)
				cmd.Execute(suite.ctx, suite.entity)
				select {
				case result := <-cmd.Done():
					assert.Equal(t, tc.expected, result)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Command should have completed")
				}
				state = cmd.State()
			}
			
			assert.Equal(t, Finished, state)
		})
	}
}

// Benchmark tests
func BenchmarkGoCommand_Execute_Success(b *testing.B) {
	entity := NewTestEntity("bench-test", true)
	delegateFn := func(entity *TestEntity) (string, error) {
		return entity.GetValue(), nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := New(delegateFn)
		ctx := context.Background()
		cmd.Execute(ctx, entity)
		<-cmd.Done()
	}
}

func BenchmarkGoCommand_Execute_WithError(b *testing.B) {
	entity := NewTestEntity("bench-test", true)
	delegateFn := func(entity *TestEntity) (string, error) {
		return "", errors.New("benchmark error")
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := New(delegateFn)
		ctx := context.Background()
		cmd.Execute(ctx, entity)
		<-cmd.Done()
	}
}

func BenchmarkGoCommand_ConcurrentStateAccess(b *testing.B) {
	entity := NewTestEntity("bench-test", true)
	delegateFn := func(entity *TestEntity) (string, error) {
		time.Sleep(time.Duration(b.N) * time.Millisecond) // Simulate work
		return entity.GetValue(), nil
	}
	cmd := New(delegateFn)
	
	ctx := context.Background()
	go cmd.Execute(ctx, entity)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cmd.State()
			if err := cmd.Error(); err != nil {
				b.Errorf("Unexpected error: %v", err)
			}
		}
	})
}
