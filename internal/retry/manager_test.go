package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTestOperation = errors.New("test operation failed")

func TestNewManager_defaults(t *testing.T) {
	m := NewManager(Config{})

	cfg := m.GetConfig()
	assert.Equal(t, uint(5), cfg.MaxAttempts)
	assert.Equal(t, time.Second, cfg.InitialDelay)
	assert.Equal(t, defaultMaxDelaySeconds*time.Second, cfg.MaxDelay)
	assert.Equal(t, "exponential", cfg.Strategy)
	assert.Equal(t, 2.0, cfg.Multiplier)
}

func TestNewManager_customValues(t *testing.T) {
	m := NewManager(Config{
		MaxAttempts:  3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Strategy:     "linear",
		Multiplier:   1.5,
	})

	cfg := m.GetConfig()
	assert.Equal(t, uint(3), cfg.MaxAttempts)
	assert.Equal(t, "linear", cfg.Strategy)
	assert.Equal(t, 1.5, cfg.Multiplier)
}

func TestManager_executeSucceeds(t *testing.T) {
	m := NewManager(Config{MaxAttempts: 3, InitialDelay: time.Millisecond})

	callCount := 0
	err := m.Execute(context.Background(), func() error {
		callCount++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestManager_executeRetriesOnFailure(t *testing.T) {
	m := NewManager(Config{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
	})

	callCount := 0

	err := m.Execute(context.Background(), func() error {
		callCount++
		if callCount < 2 {
			return errTestOperation
		}

		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestManager_executeExhaustsAttempts(t *testing.T) {
	m := NewManager(Config{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
	})

	callCount := 0

	err := m.Execute(context.Background(), func() error {
		callCount++
		return errTestOperation
	})

	require.Error(t, err)
	assert.Equal(t, 2, callCount)
}

func TestManager_executeWithContextPropagation(t *testing.T) {
	m := NewManager(Config{MaxAttempts: 3, InitialDelay: time.Millisecond})

	var capturedCtx context.Context

	err := m.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
		capturedCtx = ctx
		return nil
	})

	require.NoError(t, err)
	assert.NotNil(t, capturedCtx)
}

func TestManager_isRetryableError(t *testing.T) {
	m := NewManager(Config{})

	assert.False(t, m.IsRetryableError(nil))
	assert.True(t, m.IsRetryableError(errTestOperation))
}

func TestManager_getStats(t *testing.T) {
	m := NewManager(Config{
		MaxAttempts:  4,
		InitialDelay: 2 * time.Second,
		MaxDelay:     60 * time.Second,
		Strategy:     "random",
		Multiplier:   3.0,
	})

	stats := m.GetStats()
	assert.Equal(t, uint(4), stats["max_attempts"])
	assert.Equal(t, "random", stats["strategy"])
	assert.Equal(t, 3.0, stats["multiplier"])
}

func TestDelayTypeForStrategy_linear(t *testing.T) {
	m := NewManager(Config{Strategy: "linear", Multiplier: 2.0})

	delayFn := m.delayTypeForStrategy(100*time.Millisecond, 1*time.Second)
	require.NotNil(t, delayFn)

	d := delayFn(0, nil, nil)
	assert.Equal(t, 100*time.Millisecond, d)

	d2 := delayFn(1, nil, nil)
	assert.Equal(t, 200*time.Millisecond, d2)
}

func TestDelayTypeForStrategy_exponential(t *testing.T) {
	m := NewManager(Config{Strategy: "exponential", Multiplier: 2.0})

	delayFn := m.delayTypeForStrategy(100*time.Millisecond, 10*time.Second)
	require.NotNil(t, delayFn)

	d0 := delayFn(0, nil, nil)
	assert.Equal(t, 100*time.Millisecond, d0)

	d1 := delayFn(1, nil, nil)
	assert.Equal(t, 200*time.Millisecond, d1)
}

func TestDelayTypeForStrategy_random(t *testing.T) {
	m := NewManager(Config{Strategy: "random", Multiplier: 2.0})

	delayFn := m.delayTypeForStrategy(100*time.Millisecond, 10*time.Second)
	require.NotNil(t, delayFn)

	d := delayFn(2, nil, nil)
	assert.GreaterOrEqual(t, d, 100*time.Millisecond)
	assert.LessOrEqual(t, d, 10*time.Second)
}
