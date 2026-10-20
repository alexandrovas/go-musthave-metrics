package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FileObserver — наблюдатель, дописывающий события аудита в конец файла,
// каждое событие на новой строке.
type FileObserver struct {
	path string
	mu   sync.Mutex
}

// NewFileObserver создаёт файловый наблюдатель, пишущий в файл по пути path.
func NewFileObserver(path string) *FileObserver {
	return &FileObserver{path: path}
}

func (o *FileObserver) Notify(_ context.Context, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	data = append(data, '\n')

	o.mu.Lock()
	defer o.mu.Unlock()

	f, err := os.OpenFile(o.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}
