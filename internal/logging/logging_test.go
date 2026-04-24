package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWritesToConfiguredFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "proxy.log")
	logger, closeLogs, err := New(Options{
		Level: "info",
		File:  logPath,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := closeLogs(); err != nil {
			t.Fatalf("closeLogs() error = %v", err)
		}
	})

	logger.Info("test log entry", slog.String("component", "logging_test"))

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "test log entry") {
		t.Fatalf("log file missing message: %s", text)
	}
	if !strings.Contains(text, "\"component\":\"logging_test\"") {
		t.Fatalf("log file missing structured field: %s", text)
	}
}
