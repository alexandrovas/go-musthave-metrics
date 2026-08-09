package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

func alwaysRetriable(error) bool { return true }
func neverRetriable(error) bool  { return false }

func TestDoSucceedsFirstTry(t *testing.T) {
	calls := 0
	err := Do(context.Background(), alwaysRetriable,
		[]time.Duration{time.Millisecond, time.Millisecond},
		func() error {
			calls++
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestDoRetriesUntilSuccess(t *testing.T) {
	calls := 0
	err := Do(context.Background(), alwaysRetriable,
		[]time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
		func() error {
			calls++
			if calls < 3 {
				return errBoom
			}
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestDoExhaustsAllAttempts(t *testing.T) {
	calls := 0
	err := Do(context.Background(), alwaysRetriable,
		[]time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
		func() error {
			calls++
			return errBoom
		})
	assert.ErrorIs(t, err, errBoom)
	assert.Equal(t, 4, calls, "1 initial attempt + 3 retries")
}

func TestDoStopsWhenNotRetriable(t *testing.T) {
	calls := 0
	err := Do(context.Background(), neverRetriable,
		[]time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
		func() error {
			calls++
			return errBoom
		})
	assert.ErrorIs(t, err, errBoom)
	assert.Equal(t, 1, calls, "non-retriable error must fail fast")
}

func TestDoNoIntervalsMeansSingleAttempt(t *testing.T) {
	calls := 0
	err := Do(context.Background(), alwaysRetriable, nil,
		func() error {
			calls++
			return errBoom
		})
	assert.ErrorIs(t, err, errBoom)
	assert.Equal(t, 1, calls)
}

func TestDoAbortsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Do(ctx, alwaysRetriable,
		[]time.Duration{50 * time.Millisecond},
		func() error {
			calls++
			if calls == 1 {
				cancel()
			}
			return errBoom
		})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}
