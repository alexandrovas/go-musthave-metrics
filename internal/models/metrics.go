package models

import (
	"encoding/json"
)

// MetricType — тип метрики: counter накапливается, gauge хранит мгновенное значение.
type MetricType string

const (
	// Counter — счётчик, значение которого только увеличивается на дельту.
	Counter MetricType = "counter"
	// Gauge — метрика, хранящая мгновенное числовое значение.
	Gauge MetricType = "gauge"
)

// Metrics — метрика с плоской моделью (без иерархической вложенности).
// Delta и Value объявлены через указатели, чтобы отличать значение "0"
// от не заданного значения и, благодаря omitempty, не кодировать его в JSON.
type Metrics struct {
	ID    string     `json:"id"`
	MType MetricType `json:"type"`
	Delta *int64     `json:"delta,omitempty"`
	Value *float64   `json:"value,omitempty"`
	Hash  string     `json:"hash,omitempty"`
}

// ErrorResponse — тело JSON-ответа с ошибкой. Поле Error сериализуется
// как строка с текстом ошибки.
type ErrorResponse struct {
	Error error `json:"error"`
}

// MarshalJSON сериализует ErrorResponse, заменяя ошибку её строковым представлением.
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
