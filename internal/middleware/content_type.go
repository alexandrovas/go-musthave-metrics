package middleware

import "net/http"

const (
	contentTypeHeader          = "Content-Type"
	contentTypeApplicationJson = "application/json"
)

func RequireContentTypeJson(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get(contentTypeHeader)
		if contentType != contentTypeApplicationJson {
			http.Error(w, "unexpected content type", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}
