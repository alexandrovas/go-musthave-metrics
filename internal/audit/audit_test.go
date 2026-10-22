package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func TestSubjectNotifiesAllObservers(t *testing.T) {
	a := &recordingObserver{}
	b := &recordingObserver{}
	s := NewSubject(a, b)

	event := NewEvent([]string{"Alloc"}, "127.0.0.1")
	require.NoError(t, s.Notify(context.Background(), event))

	assert.Equal(t, []Event{event}, a.events)
	assert.Equal(t, []Event{event}, b.events)
}

func TestSubjectContinuesAfterObserverError(t *testing.T) {
	failing := &recordingObserver{err: errors.New("boom")}
	ok := &recordingObserver{}
	s := NewSubject(failing, ok)

	event := NewEvent([]string{"Alloc"}, "127.0.0.1")
	err := s.Notify(context.Background(), event)

	assert.Error(t, err)
	assert.Len(t, ok.events, 1)
}

func TestNilSubjectNotifyIsNoop(t *testing.T) {
	var s *Subject
	assert.NoError(t, s.Notify(context.Background(), NewEvent(nil, "")))
}

func TestFileObserverAppendsJSONLines(t *testing.T) {
	path := t.TempDir() + "/audit.log"
	o := NewFileObserver(path)

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	o := NewHTTPObserver(srv.URL)
	err := o.Notify(context.Background(), NewEvent([]string{"Alloc"}, "127.0.0.1"))
	assert.Error(t, err)
}
