package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireContentTypeJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{
			name:        "application/json is accepted",
			contentType: contentTypeApplicationJSON,
			wantStatus:  http.StatusOK,
		},
		{
			name:       "missing content type is rejected",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "text/plain is rejected",
			contentType: contentTypeTextHTML,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "json with charset is rejected (exact match)",
			contentType: "application/json; charset=utf-8",
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.contentType != "" {
				req.Header.Set(contentTypeHeader, tt.contentType)
			}

			rec := httptest.NewRecorder()
			RequireContentTypeJSON(next).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantStatus == http.StatusOK, called)
		})
	}
}
