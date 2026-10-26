package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingObserver struct {
	events []Event
	err    error
}

func (o *recordingObserver) Notify(_ context.Context, event Event) error {
	o.events = append(o.events, event)
	return o.err
}

func TestNotifyObserversNotifiesAllObservers(t *testing.T) {
	a := &recordingObserver{}
	b := &recordingObserver{}
	d := NewDispatcher(nil, 0, 0, a, b)
	defer d.Close(context.Background())

	event := NewEvent([]string{"Alloc"}, "127.0.0.1")
	require.NoError(t, d.notifyObservers(context.Background(), event))

	assert.Equal(t, []Event{event}, a.events)
	assert.Equal(t, []Event{event}, b.events)
}

func TestNotifyObserversContinuesAfterObserverError(t *testing.T) {
	failing := &recordingObserver{err: errors.New("boom")}
	ok := &recordingObserver{}
	d := NewDispatcher(nil, 0, 0, failing, ok)
	defer d.Close(context.Background())

	event := NewEvent([]string{"Alloc"}, "127.0.0.1")
	err := d.notifyObservers(context.Background(), event)

	assert.Error(t, err)
	assert.Len(t, ok.events, 1)
}

func TestNotifyObserversAggregatesAllObserverErrors(t *testing.T) {
	errA := errors.New("observer A failed")
	errB := errors.New("observer B failed")
	a := &recordingObserver{err: errA}
	b := &recordingObserver{err: errB}
	d := NewDispatcher(nil, 0, 0, a, b)
	defer d.Close(context.Background())

	err := d.notifyObservers(context.Background(), NewEvent([]string{"Alloc"}, "127.0.0.1"))

	require.Error(t, err)
	assert.ErrorIs(t, err, errA, "error from the first observer must not be lost")
	assert.ErrorIs(t, err, errB, "error from the second observer must not be lost")
}

func TestNilDispatcherNotifyIsNoop(t *testing.T) {
	var d *Dispatcher
	d.Notify(NewEvent(nil, "")) // no-op, без паники
}

func TestFileObserverAppendsJSONLines(t *testing.T) {
	path := t.TempDir() + "/audit.log"
	o := NewFileObserver(path)
	defer o.Close()

	e1 := NewEvent([]string{"Alloc"}, "127.0.0.1")
	e2 := NewEvent([]string{"PollCount"}, "10.0.0.1")

	require.NoError(t, o.Notify(context.Background(), e1))
	require.NoError(t, o.Notify(context.Background(), e2))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got []Event
	for _, line := range splitLines(data) {
		var e Event
		require.NoError(t, json.Unmarshal(line, &e))
		got = append(got, e)
	}
	assert.Equal(t, []Event{e1, e2}, got)
}

func TestFileObserverCloseIsIdempotent(t *testing.T) {
	path := t.TempDir() + "/audit.log"
	o := NewFileObserver(path)

	// Close до первой записи — безопасен.
	require.NoError(t, o.Close())

	require.NoError(t, o.Notify(context.Background(), NewEvent([]string{"Alloc"}, "127.0.0.1")))
	require.NoError(t, o.Close())
	require.NoError(t, o.Close()) // повторный Close — безопасен
}

func TestDispatcherCloseClosesObservers(t *testing.T) {
	o := NewFileObserver(t.TempDir() + "/audit.log")
	d := NewDispatcher(nil, 0, 0, o)

	require.NoError(t, d.notifyObservers(context.Background(), NewEvent([]string{"Alloc"}, "127.0.0.1")))
	require.NoError(t, d.Close(context.Background()))
}

func TestDispatcherNotifiesObserver(t *testing.T) {
	o := &recordingObserver{}
	d := NewDispatcher(nil, 0, 0, o)

	event := NewEvent([]string{"Alloc"}, "127.0.0.1")
	d.Notify(event)
	require.NoError(t, d.Close(context.Background()))

	assert.Equal(t, []Event{event}, o.events)
}

func TestDispatcherNotifyDuringCloseDoesNotPanic(t *testing.T) {
	d := NewDispatcher(nil, 0, 0, &recordingObserver{})

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			d.Notify(NewEvent([]string{"Alloc"}, "127.0.0.1"))
		}
	})

	require.NoError(t, d.Close(context.Background()))
	wg.Wait()
}

func TestDispatcherCloseCancelsSlowDelivery(t *testing.T) {
	blocking := &blockingObserver{unblocked: make(chan struct{})}
	defer close(blocking.unblocked)
	d := NewDispatcher(nil, 0, 0, blocking)

	d.Notify(NewEvent([]string{"Alloc"}, "127.0.0.1"))

	// Короткий дедлайн вместо DefaultDrainTimeout — Close должен прервать
	// зависшую доставку по истечении переданного ctx, а не ждать его бесконечно.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- d.Close(ctx) }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not return promptly after ctx deadline")
	}
}

type blockingObserver struct {
	unblocked chan struct{}
}

func (o *blockingObserver) Notify(ctx context.Context, _ Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-o.unblocked:
		return nil
	}
}

func TestDispatcherCloseIsIdempotent(t *testing.T) {
	d := NewDispatcher(nil, 0, 0, &recordingObserver{})

	require.NoError(t, d.Close(context.Background()))
	require.NoError(t, d.Close(context.Background())) // повторный Close — безопасен

	var nilDispatcher *Dispatcher
	require.NoError(t, nilDispatcher.Close(context.Background()))
	nilDispatcher.Notify(NewEvent(nil, "")) // no-op, без паники
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	return lines
}

func TestHTTPObserverPostsEvent(t *testing.T) {
	var gotEvent Event
	var gotMethod, gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotEvent))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	o := NewHTTPObserver(srv.URL)
	event := NewEvent([]string{"Alloc", "Frees"}, "192.168.0.42")

	require.NoError(t, o.Notify(context.Background(), event))
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, event, gotEvent)
}

func TestHTTPObserverReturnsErrorOnNonSuccessStatus(t *testing.T) {
	// 4xx — не-retriable ошибка: Notify должен вернуть её сразу, без повторов.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	o := NewHTTPObserver(srv.URL)
	err := o.Notify(context.Background(), NewEvent([]string{"Alloc"}, "127.0.0.1"))
	assert.Error(t, err)
}

func TestHTTPObserverRetriesOnServerError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	o := NewHTTPObserver(srv.URL)
	err := o.Notify(context.Background(), NewEvent([]string{"Alloc"}, "127.0.0.1"))

	require.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
}
