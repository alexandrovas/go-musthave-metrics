package handler

import "errors"

var (
	ErrJSONDecode          = errors.New("cannot decode JSON")
	ErrUnknownMetricType   = errors.New("unknown metric type")
	ErrFailedUpdateMetrics = errors.New("failed to update metric")
	ErrMetricIdIsRequired  = errors.New("metric ID cannot be empty")
	ErrValueIsRequired     = errors.New("value is required for gauge metric")
	ErrDeltaIsRequired     = errors.New("delta is required for counter metric")
	ErrInternal            = errors.New("internal server error")
)
