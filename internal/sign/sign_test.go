package sign

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeIsDeterministic(t *testing.T) {
	data := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)
	h1 := Compute(data, "secret")
	h2 := Compute(data, "secret")
	assert.Equal(t, h1, h2)
	assert.Len(t, h1, 64, "hex-encoded SHA256 must be 64 characters")
}

func TestComputeDependsOnKey(t *testing.T) {
	data := []byte("payload")
	assert.NotEqual(t, Compute(data, "key1"), Compute(data, "key2"))
}

func TestComputeDependsOnData(t *testing.T) {
	assert.NotEqual(t,
		Compute([]byte("payload1"), "key"),
		Compute([]byte("payload2"), "key"),
	)
}

func TestComputeDoesNotMutateInput(t *testing.T) {
	data := make([]byte, 4, 16) // избыточная capacity, чтобы проверить отсутствие append-мутации
	copy(data, "abcd")
	before := append([]byte(nil), data...)

	Compute(data, "some-long-key-longer-than-spare-capacity")

	assert.Equal(t, before, data, "Compute must not mutate the caller's slice")
}
