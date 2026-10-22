package middleware

import "net/http"

const (
	contentTypeHeader          = "Content-Type"
	contentTypeApplicationJSON = "application/json"
	contentTypeTextHTML        = "text/html"
)

func RequireContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get(contentTypeHeader)
		if contentType != contentTypeApplicationJSON {
			http.Error(w, "unexpected content type", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}
