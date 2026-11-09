package pool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testValue struct {
	value      int
	resetCalls int
}

func (v *testValue) Reset() {
	v.value = 0
	v.resetCalls++
}

func TestGetCreatesValueWhenPoolIsEmpty(t *testing.T) {
	created := 0
	p := New(func() *testValue {
		created++
		return &testValue{value: 42}
	})

	got := p.Get()
	require.NotNil(t, got)
	assert.Equal(t, 42, got.value)
	assert.Equal(t, 1, created)
}

func TestGetCreatesValueWhenPoolContainsUnexpectedType(t *testing.T) {
	created := 0
	p := New(func() *testValue {
		created++
		return &testValue{value: 42}
	})
	p.pool.Put("unexpected type")

	got := p.Get()
	require.NotNil(t, got)
	assert.Equal(t, 42, got.value)
	assert.Equal(t, 1, created)
}

func TestPutResetsValueBeforeReturningItToPool(t *testing.T) {
	p := New(func() *testValue { return &testValue{} })
	value := p.Get()
	value.value = 99

	p.Put(value)

	assert.Zero(t, value.value)
	assert.Equal(t, 1, value.resetCalls)

	got := p.Get()
	require.NotNil(t, got)
	assert.Zero(t, got.value)
}

func TestNewPanicsOnNilConstructor(t *testing.T) {
	assert.PanicsWithValue(t, "pool: nil constructor", func() {
		New[*testValue](nil)
	})
}
