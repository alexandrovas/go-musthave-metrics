// Package audit реализует паттерн «Наблюдатель» для аудита запросов,
// обновляющих метрики: после успешной обработки запроса формируется
// событие аудита, которое рассылается всем подписанным наблюдателям.
package audit

import (
	"context"
	"time"
)

// Event — событие аудита, формируется после успешной обработки запроса на обновление метрик.
type Event struct {
	TS        int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}

// NewEvent создаёт событие аудита с текущей временной меткой.
func NewEvent(metrics []string, ipAddress string) Event {
	return Event{
		TS:        time.Now().Unix(),
		Metrics:   metrics,
		IPAddress: ipAddress,
	}
}

// Observer — наблюдатель, принимающий события аудита.
type Observer interface {
	Notify(ctx context.Context, event Event) error
}

// Компиляционная проверка: наблюдатели удовлетворяют интерфейсу Observer.
var (
	_ Observer = (*FileObserver)(nil)
	_ Observer = (*HTTPObserver)(nil)
)
