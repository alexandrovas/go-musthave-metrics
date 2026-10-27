package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/alexandrovas/go-musthave-metrics/internal/retry"
)

// HTTPObserver — наблюдатель, отправляющий события аудита POST-запросом
// на удалённый сервер-приёмник.
type HTTPObserver struct {
	url    string
	client *http.Client
}

// NewHTTPObserver создаёт наблюдателя, отправляющего события на url.
func NewHTTPObserver(url string) *HTTPObserver {
	return &HTTPObserver{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (o *HTTPObserver) Notify(ctx context.Context, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	return retry.Do(ctx, isRetriableHTTPError, retry.Intervals, func() error {
		// Запрос создаём внутри попытки, чтобы при повторе тело читалось заново.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("create audit request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := o.client.Do(req)
		if err != nil {
			return &auditSendError{err: fmt.Errorf("send audit event: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			return &auditSendError{
				status: resp.StatusCode,
				err:    fmt.Errorf("audit endpoint %s returned status %d", o.url, resp.StatusCode),
			}
		}
		return nil
	})
}

// auditSendError — ошибка отправки события аудита. status хранит HTTP-статус
// ответа (0, если ответ не был получен — сетевая ошибка).
type auditSendError struct {
	status int
	err    error
}

func (e *auditSendError) Error() string { return e.err.Error() }
func (e *auditSendError) Unwrap() error { return e.err }

// isRetriableHTTPError сообщает, стоит ли повторить отправку события аудита:
// временные сетевые ошибки и 5xx-ответы считаются повторяемыми, остальные — нет.
func isRetriableHTTPError(err error) bool {
	if se, ok := errors.AsType[*auditSendError](err); ok && se.status >= 500 {
		return true
	}
	_, ok := errors.AsType[net.Error](err)
	return ok
}
