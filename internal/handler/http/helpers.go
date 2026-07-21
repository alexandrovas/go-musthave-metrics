package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

func writeJsonBody(w http.ResponseWriter, resp any, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if resp != nil {
		enc := json.NewEncoder(w)
		if err := enc.Encode(resp); err != nil {
			slog.Debug("error encoding response", "error", err)
		}
	}
}

func decodeMetric(r *http.Request) (models.Metrics, error) {
	var metric models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		slog.Error("cannot decode request JSON body", "error", err)
		return metric, err
	}
	return metric, nil
}
