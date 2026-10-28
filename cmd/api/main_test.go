package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const listeningMsg = "http server listening"

var envKeys = []string{
	"HTTP_ADDR",
	"HTTP_READ_HEADER_TIMEOUT",
	"HTTP_READ_TIMEOUT",
	"HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT",
	"SHUTDOWN_TIMEOUT",
	"LOG_LEVEL",
}

// setEnv clears every config variable so values from the developer's shell
// don't leak into the test, then applies the given overrides.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range envKeys {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// logLines is an io.Writer that forwards every log line to a channel, so
// tests can wait for a specific message instead of sleeping.
type logLines chan string

func (l logLines) Write(p []byte) (int, error) {
	l <- string(p)
	return len(p), nil
}

func newTestLogger() (*slog.Logger, logLines) {
	lines := make(logLines, 64)
	return slog.New(slog.NewJSONHandler(lines, nil)), lines
}

// waitLog blocks until a log line with the given msg is written.
func waitLog(t *testing.T, lines logLines, msg string) string {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case line := <-lines:
			if strings.Contains(line, `"msg":"`+msg+`"`) {
				return line
			}
		case <-timeout:
			t.Fatalf("log message %q was not written within 5s", msg)
			return ""
		}
	}
}

// drainLogs returns the log lines that were written so far.
func drainLogs(lines logLines) []string {
	var out []string
	for {
		select {
		case line := <-lines:
			out = append(out, line)
		default:
			return out
		}
	}
}

func waitResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return within 5s")
		return nil
	}
}

// blockingHandler signals on started when a request arrives and responds
// only after release is closed. It simulates a slow in-flight request.
type blockingHandler struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{started: make(chan struct{}), release: make(chan struct{})}
}

func (h *blockingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.once.Do(func() { close(h.started) })
	<-h.release
	w.WriteHeader(http.StatusOK)
}

type testServer struct {
	addr   string
	logs   logLines
	done   <-chan error
	cancel context.CancelCauseFunc
}

// startRun starts run in the background on a random port and waits until
// it accepts connections.
func startRun(t *testing.T, env map[string]string, handler http.Handler) *testServer {
	t.Helper()
	env["HTTP_ADDR"] = "127.0.0.1:0"
	setEnv(t, env)

	ctx, cancel := context.WithCancelCause(context.Background())
	t.Cleanup(func() { cancel(nil) })
	logger, logs := newTestLogger()
	addrs := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, logger, new(slog.LevelVar), handler, func(a net.Addr) { addrs <- a })
	}()

	select {
	case a := <-addrs:
		return &testServer{addr: a.String(), logs: logs, done: done, cancel: cancel}
	case err := <-done:
		t.Fatalf("run returned before listening: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start within 5s")
	}
	return nil
}

// get sends a request in the background and delivers the status code (or
// the error) on the returned channels.
func get(url string) (<-chan int, <-chan error) {
	status := make(chan int, 1)
	errs := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Get(url)
		if err != nil {
			errs <- err
			return
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		status <- res.StatusCode
	}()
	return status, errs
}

func TestRunInvalidConfig(t *testing.T) {
	setEnv(t, map[string]string{"LOG_LEVEL": "loud"})
	logger, logs := newTestLogger()

	err := run(context.Background(), logger, new(slog.LevelVar), http.NotFoundHandler(), nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "load config: ") {
		t.Errorf("unexpected error: want prefix %q, got %q", "load config: ", err)
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("error should mention LOG_LEVEL, got %q", err)
	}
	if got := drainLogs(logs); len(got) != 0 {
		t.Errorf("nothing should be logged before the config is valid, got %q", got)
	}
}

func TestRunPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	setEnv(t, map[string]string{"HTTP_ADDR": ln.Addr().String()})
	logger, logs := newTestLogger()

	err = run(context.Background(), logger, new(slog.LevelVar), http.NotFoundHandler(), nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "listen: ") {
		t.Errorf("unexpected error: want prefix %q, got %q", "listen: ", err)
	}
	for _, line := range drainLogs(logs) {
		if strings.Contains(line, listeningMsg) {
			t.Errorf("%q must not be logged when the port cannot be bound", listeningMsg)
		}
	}
}

func TestRunGracefulShutdownWaitsForInFlightRequest(t *testing.T) {
	h := newBlockingHandler()
	srv := startRun(t, map[string]string{"SHUTDOWN_TIMEOUT": "5s"}, h)

	status, reqErr := get("http://" + srv.addr + "/")
	<-h.started

	srv.cancel(errors.New("test shutdown"))
	line := waitLog(t, srv.logs, "shutdown started")
	if !strings.Contains(line, "test shutdown") {
		t.Errorf("shutdown log should contain the reason, got %s", line)
	}

	select {
	case err := <-srv.done:
		t.Fatalf("run returned while a request was in flight: %v", err)
	default:
	}

	close(h.release)

	select {
	case code := <-status:
		if code != http.StatusOK {
			t.Errorf("in-flight request: want status 200, got %d", code)
		}
	case err := <-reqErr:
		t.Fatalf("in-flight request failed: %v", err)
	}

	if err := waitResult(t, srv.done); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
	waitLog(t, srv.logs, "server stopped")

	if conn, err := net.Dial("tcp", srv.addr); err == nil {
		conn.Close()
		t.Error("server still accepts connections after shutdown")
	}
}

func TestRunShutdownTimeout(t *testing.T) {
	h := newBlockingHandler()
	t.Cleanup(func() { close(h.release) })
	srv := startRun(t, map[string]string{"SHUTDOWN_TIMEOUT": "100ms"}, h)

	_, _ = get("http://" + srv.addr + "/")
	<-h.started

	srv.cancel(errors.New("test shutdown"))

	err := waitResult(t, srv.done)
	if err == nil {
		t.Fatal("expected a shutdown timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context.DeadlineExceeded, got %v", err)
	}
}
