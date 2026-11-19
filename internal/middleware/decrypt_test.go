package middleware

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexandrovas/go-musthave-metrics/internal/crypto"
	"github.com/stretchr/testify/require"
)

func TestDecryptBodyBeforeCompression(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, err = zw.Write([]byte(`{"metrics":true}`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	ciphertext, err := crypto.Encrypt(&privateKey.PublicKey, compressed.Bytes())
	require.NoError(t, err)

	var got []byte
	handler := DecryptBody(privateKey)(Compression(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(ciphertext))
	req.Header.Set("Content-Encoding", "gzip")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Equal(t, `{"metrics":true}`, string(got))
}

func TestDecryptBodyRejectsInvalidPayload(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	handler := DecryptBody(privateKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader([]byte("invalid")))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestDecryptBodyAllowsRequestsWithoutBody(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	handler := DecryptBody(privateKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusNoContent, resp.Code)
}
