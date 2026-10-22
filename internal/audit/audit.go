// Package audit реализует паттерн «Наблюдатель» для аудита запросов,
// обновляющих метрики: после успешной обработки запроса формируется
// событие аудита, которое рассылается всем подписанным наблюдателям.
package audit

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
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

// Subject — источник событий аудита (субъект наблюдения), рассылающий
// каждое событие всем зарегистрированным наблюдателям.
type Subject struct {
	observers []Observer
}

// NewSubject создаёт субъект аудита с заданным набором наблюдателей.
func NewSubject(observers ...Observer) *Subject {
	return &Subject{observers: observers}
}

// Notify рассылает событие всем наблюдателям параллельно, чтобы медленный
// или недоступный приёмник не задерживал доставку остальным. Ошибка одного
// наблюдателя не прерывает рассылку остальным. Вызов на nil-субъекте —
// no-op, что позволяет безопасно использовать его в коде вызывающей
// стороны без дополнительных проверок.
func (s *Subject) Notify(ctx context.Context, event Event) error {
	if s == nil {
		return nil
	}

	var g errgroup.Group
	for _, o := range s.observers {
		g.Go(func() error {
			return o.Notify(ctx, event)
		})
	}
	return g.Wait()
}
