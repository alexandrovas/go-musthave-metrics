package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create audit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("send audit event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("audit endpoint %s returned status %d", o.url, resp.StatusCode)
	}
	return nil
}
