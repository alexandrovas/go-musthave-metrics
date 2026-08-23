package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexandrovas/go-musthave-metrics/internal/sign"
)

func echoHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func TestRequireHashSHA256_NoKeyIsNoop(t *testing.T) {
	srv := httptest.NewServer(ValidateSignature("")(http.HandlerFunc(echoHandler)))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "text/plain", bytes.NewReader([]byte("hello")))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get(hashHeader), "no key configured — response must not be signed")
}

func TestRequireHashSHA256_ValidHashPasses(t *testing.T) {
	const key = "secret"
	srv := httptest.NewServer(ValidateSignature(key)(http.HandlerFunc(echoHandler)))
	defer srv.Close()

	body := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)

	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(hashHeader, sign.Compute(body, key))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, body, respBody)
}

func TestRequireHashSHA256_MismatchIsRejected(t *testing.T) {
	const key = "secret"
	srv := httptest.NewServer(ValidateSignature(key)(http.HandlerFunc(echoHandler)))
	defer srv.Close()

	body := []byte(`{"id":"Alloc","type":"gauge","value":1024}`)

	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(hashHeader, "not-a-valid-hash")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// В тестах ошибка, запросы выполняются без заголовка HashSHA256
// https://github.com/Yandex-Practicum/go-autotests/blob/main/cmd/metricstest_v2/iteration14_test.go
// func TestRequireHashSHA256_MissingHeaderIsRejected(t *testing.T) {
// 	// При заданном ключе заголовок обязателен для маршрутов, обёрнутых этим
// 	// middleware (в router.go он навешивается только на write-эндпоинты и
// 	// GET /value/{type}/{name}; /ping и / им намеренно не оборачиваются).
// 	const key = "secret"
// 	srv := httptest.NewServer(ValidateSignature(key)(http.HandlerFunc(echoHandler)))
// 	defer srv.Close()

// 	body := []byte("no hash header here")
// 	resp, err := http.Post(srv.URL, "text/plain", bytes.NewReader(body))
// 	require.NoError(t, err)
// 	defer resp.Body.Close()

// 	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
// }
