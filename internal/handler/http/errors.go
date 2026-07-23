package handler

import "errors"

var (
	errJSONDecode          = errors.New("cannot decode JSON")
	errUnknownMetricType   = errors.New("unknown metric type")
	errFailedUpdateMetrics = errors.New("failed to update metric")
	errMetricIdIsRequired  = errors.New("metric ID cannot be empty")
	errValueIsRequired     = errors.New("value is required for gauge metric")
	errDeltaIsRequired     = errors.New("delta is required for counter metric")
	errInternal            = errors.New("internal server error")
)
