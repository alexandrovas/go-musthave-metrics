package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alexandrovas/go-musthave-metrics/internal/config"
	"github.com/alexandrovas/go-musthave-metrics/internal/models"
)

func TestServerSavesStateOnSIGTERM(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	storagePath := filepath.Join(t.TempDir(), "metrics.json")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(&config.ServerConfig{
		Address:         address,
		StoreInterval:   time.Hour,
		FileStoragePath: storagePath,
		AuditWorkers:    1,
		AuditQueueSize:  4,
	}, logger)
	runDone := make(chan error, 1)
	go func() { runDone <- server.Run() }()

	client := &http.Client{Timeout: time.Second}
	pingURL := "http://" + address + "/ping"
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, pingErr := client.Get(pingURL)
		if pingErr == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", pingErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	delta := int64(17)
	requestBody, err := json.Marshal([]models.Metrics{{ID: "PollCount", MType: models.Counter, Delta: &delta}})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+address+"/updates", bytes.NewReader(requestBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, process.Signal(syscall.SIGTERM))
	select {
	case err := <-runDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after SIGTERM")
	}

	contents, err := os.ReadFile(storagePath)
	require.NoError(t, err)
	var saved []models.Metrics
	require.NoError(t, json.Unmarshal(contents, &saved))
	require.Len(t, saved, 1)
	require.Equal(t, "PollCount", saved[0].ID)
	require.Equal(t, delta, *saved[0].Delta)
}
