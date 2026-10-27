package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkCompression_Response(b *testing.B) {
	body := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, contentTypeApplicationJSON)
		w.Write(body)
	})
	h := Compression(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(acceptEncodingHeader, acceptEncodingGzip)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
}

func BenchmarkCompression_RequestDecompress(b *testing.B) {
	payload := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(payload)
	_ = gw.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	})
	h := Compression(next)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(buf.Bytes()))
	req.Header.Set(contentEncodingHeader, acceptEncodingGzip)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
}
