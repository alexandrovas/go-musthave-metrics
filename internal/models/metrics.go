package models

import (
	"encoding/json"
)

type MetricType string

const (
	Counter MetricType = "counter"
	Gauge   MetricType = "gauge"
)

// NOTE: Не усложняем пример, вводя иерархическую вложенность структур.
// Органичиваясь плоской моделью.
// Delta и Value объявлены через указатели,
// что бы отличать значение "0", от не заданного значения
// и соответственно не кодировать в структуру.
type Metrics struct {
	ID    string     `json:"id"`
	MType MetricType `json:"type"`
	Delta *int64     `json:"delta,omitempty"`
	Value *float64   `json:"value,omitempty"`
	Hash  string     `json:"hash,omitempty"`
}

type ErrorResponse struct {
	Error error `json:"error"`
}

func (er ErrorResponse) MarshalJSON() ([]byte, error) {
	type ErrorResponseAlias ErrorResponse
	aliasValue := struct {
		ErrorResponseAlias
		Error string `json:"error"`
	}{
		ErrorResponseAlias: ErrorResponseAlias(er),
		Error:              er.Error.Error(),
	}
	return json.Marshal(aliasValue)
}
