package audit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"
)

// DefaultWorkers и DefaultQueueSize — значения по умолчанию для NewDispatcher
// (используются, если переданные workers/queueSize <= 0), а также как
// значения по умолчанию конфигурации сервера. Очередь ограничена, чтобы при
// высокой нагрузке события аудита не накапливались неограниченно в памяти.
const (
	DefaultWorkers   = 4
	DefaultQueueSize = 64
)

// DefaultDrainTimeout — таймаут по умолчанию для контекста, который
// передаётся в Close, если вызывающему не принципиален собственный дедлайн
// (та же роль, что у отдельного контекста, передаваемого в http.Server.Shutdown).
const DefaultDrainTimeout = 10 * time.Second

// Dispatcher — асинхронная рассылка событий аудита набору наблюдателей. Не
// блокирует вызывающего: событие ставится в ограниченную очередь, которую
// разбирает фиксированный пул воркеров, рассылающих его всем наблюдателям
// параллельно (чтобы медленный или недоступный приёмник не задерживал
// доставку остальным). При переполнении очереди событие отбрасывается (аудит
// — best effort). Владеет наблюдателями: Close завершает воркеры, дожидается
// доставки уже поставленных событий и закрывает наблюдателей, поддерживающих
// io.Closer.
type Dispatcher struct {
	observers []Observer
	logger    *slog.Logger

	// cancel прерывает доставки, ещё не завершённые к моменту истечения
	// drainTimeout в Close. Сам контекст, который слушают воркеры, в структуре
	// не хранится — он локальная переменная NewDispatcher, захваченная
	// замыканиями воркеров: контекст нужен только им, а Close должен лишь
	// дотянуться до cancel из другого вызова.
	cancel context.CancelFunc

	jobs chan Event
	wg   sync.WaitGroup
	once sync.Once

	// mu защищает closed: Notify держит RLock на время проверки+отправки,
	// Close берёт Lock, что гарантирует отсутствие отправки в уже закрытый
	// канал jobs (send on closed channel иначе паникует).
	mu     sync.RWMutex
	closed bool
}

// NewDispatcher создаёт диспетчер с заданными наблюдателями и запускает пул
// из workers воркеров, разбирающих очередь ёмкостью queueSize. Если workers
// или queueSize <= 0, используются значения по умолчанию (DefaultWorkers,
// DefaultQueueSize) — это защищает от нерабочей конфигурации: например, при
// 0 воркеров события никогда бы не доставлялись. Если logger == nil,
// используется «молчащий» логгер, чтобы методам Dispatcher не нужно было
// проверять logger на nil перед каждым вызовом.
func NewDispatcher(logger *slog.Logger, workers, queueSize int, observers ...Observer) *Dispatcher {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if workers <= 0 {
		workers = DefaultWorkers
	}
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{
		observers: observers,
		logger:    logger,
		cancel:    cancel,
		jobs:      make(chan Event, queueSize),
	}
	for range workers {
		d.wg.Go(func() {
			for event := range d.jobs {
				if err := d.notifyObservers(ctx, event); err != nil {
					d.logger.Error("audit notify failed", "error", err)
				}
			}
		})
	}
	return d
}

// Notify ставит событие в очередь, не задерживая вызывающего. Если очередь
// переполнена или диспетчер уже закрыт, событие отбрасывается. Вызов на
// nil-диспетчере — no-op.
func (d *Dispatcher) Notify(event Event) {
	if d == nil {
		return
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		d.logger.Warn("audit dispatcher closed, event dropped", "metrics", event.Metrics)
		return
	}
	select {
	case d.jobs <- event:
	default:
		d.logger.Warn("audit queue full, event dropped", "metrics", event.Metrics)
	}
}

// notifyObservers рассылает событие всем наблюдателям параллельно. Ошибка
// одного наблюдателя не прерывает рассылку остальным; все ошибки собираются
// и возвращаются вместе через errors.Join, чтобы ни один сбой доставки не
// потерялся молча.
func (d *Dispatcher) notifyObservers(ctx context.Context, event Event) error {
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, o := range d.observers {
		wg.Go(func() {
			if err := o.Notify(ctx, event); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Close останавливает приём новых событий, закрывает очередь и дожидается
// доставки уже поставленных в неё событий, пока не истечёт ctx (так же, как
// это делает http.Server.Shutdown). Если ctx завершается раньше, чем все
// доставки закончились, оставшиеся отменяются через внутренний контекст —
// это прерывает, в частности, ретраи HTTPObserver, чтобы остановка сервера
// не блокировалась надолго из-за недоступного приёмника аудита. Затем
// закрывает наблюдателей, поддерживающих io.Closer. Метод идемпотентен; на
// nil-диспетчере — no-op.
func (d *Dispatcher) Close(ctx context.Context) error {
	if d == nil {
		return nil
	}
	var err error
	d.once.Do(func() {
		d.mu.Lock()
		d.closed = true
		close(d.jobs)
		d.mu.Unlock()

		done := make(chan struct{})
		go func() {
			d.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			d.logger.Warn("audit dispatcher drain deadline exceeded, cancelling in-flight deliveries", "error", ctx.Err())
			d.cancel()
			<-done
		}

		err = d.closeObservers()
	})
	return err
}

// closeObservers закрывает наблюдателей, поддерживающих io.Closer (например,
// файловый FileObserver).
func (d *Dispatcher) closeObservers() error {
	var errs []error
	for _, o := range d.observers {
		if c, ok := o.(io.Closer); ok {
			if err := c.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
