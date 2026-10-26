package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// FileObserver — наблюдатель, дописывающий события аудита в конец файла,
// каждое событие на новой строке. Файл открывается один раз (при первой
// записи) и держится открытым всё время жизни наблюдателя.
type FileObserver struct {
	path string
	mu   sync.Mutex
	f    *os.File
}

// Компиляционная проверка соответствия интерфейсу io.Closer.
var _ io.Closer = (*FileObserver)(nil)

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

	if o.f == nil {
		f, err := os.OpenFile(o.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open audit file: %w", err)
		}
		o.f = f
	}

	if _, err := o.f.Write(data); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

// Close закрывает файл наблюдателя. Вызов на ещё не открытом или уже закрытом
// наблюдателе безопасен (no-op).
func (o *FileObserver) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.f == nil {
		return nil
	}
	err := o.f.Close()
	o.f = nil
	return err
}
