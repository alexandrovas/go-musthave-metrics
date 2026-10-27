package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(data)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func gunzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer zr.Close()
	out, err := io.ReadAll(zr)
	require.NoError(t, err)
	return out
}

func TestCompression_ResponseIsCompressed(t *testing.T) {
	body := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, contentTypeApplicationJSON)
		w.Write(body)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(acceptEncodingHeader, acceptEncodingGzip)

	rec := httptest.NewRecorder()
	Compression(next).ServeHTTP(rec, req)

	assert.Equal(t, acceptEncodingGzip, rec.Header().Get(contentEncodingHeader))
	assert.Equal(t, body, gunzipBytes(t, rec.Body.Bytes()))
}

func TestCompression_NotCompressedWithoutAcceptEncoding(t *testing.T) {
	body := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, contentTypeApplicationJSON)
		w.Write(body)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	Compression(next).ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get(contentEncodingHeader))
	assert.Equal(t, body, rec.Body.Bytes())
}

func TestCompression_NotCompressedForDisallowedContentType(t *testing.T) {
	body := []byte("plain text")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, "text/plain")
		w.Write(body)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(acceptEncodingHeader, acceptEncodingGzip)

	rec := httptest.NewRecorder()
	Compression(next).ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get(contentEncodingHeader))
	assert.Equal(t, body, rec.Body.Bytes())
}

func TestCompression_NotCompressedForErrorStatus(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contentTypeHeader, contentTypeApplicationJSON)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(acceptEncodingHeader, acceptEncodingGzip)

	rec := httptest.NewRecorder()
	Compression(next).ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get(contentEncodingHeader))
}

func TestCompression_RequestIsDecompressed(t *testing.T) {
	payload := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)
	compressed := gzipBytes(t, payload)

	var received []byte
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressed))
	req.Header.Set(contentEncodingHeader, acceptEncodingGzip)

	rec := httptest.NewRecorder()
	Compression(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, payload, received)
}

func TestCompression_InvalidGzipBodyIsRejected(t *testing.T) {
	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not gzip")))
	req.Header.Set(contentEncodingHeader, acceptEncodingGzip)

	rec := httptest.NewRecorder()
	Compression(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, called)
}
