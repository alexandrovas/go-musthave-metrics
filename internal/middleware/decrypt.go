package middleware

import (
	"bytes"
	"crypto/rsa"
	"io"
	"net/http"

	"github.com/alexandrovas/go-musthave-metrics/internal/crypto"
)

// DecryptBody расшифровывает тело запроса до того, как его обработают следующие
// middleware (в частности, распаковка gzip) и HTTP-обработчики.
func DecryptBody(privateKey *rsa.PrivateKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "cannot read encrypted request body", http.StatusBadRequest)
				return
			}
			_ = r.Body.Close()
			if len(data) == 0 {
				r.Body = io.NopCloser(bytes.NewReader(data))
				next.ServeHTTP(w, r)
				return
			}
			plaintext, err := crypto.Decrypt(privateKey, data)
			if err != nil {
				http.Error(w, "cannot decrypt request body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(plaintext))
			r.ContentLength = int64(len(plaintext))
			next.ServeHTTP(w, r)
		})
	}
}
