package sign

import "testing"

func BenchmarkCompute(b *testing.B) {
	data := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Compute(data, "secret-key")
	}
}
