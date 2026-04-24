package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var fileRuntimeLevel = &slog.LevelVar{}
var consoleRuntimeLevel = &slog.LevelVar{}

type Options struct {
	Level        string
	File         string
	DebugConsole bool
}

func New(opts Options) (*slog.Logger, func() error, error) {
	fileLevel, err := parseLevel(opts.Level)
	if err != nil {
		return nil, nil, err
	}
	fileRuntimeLevel.Set(fileLevel)

	handlers := make([]slog.Handler, 0, 2)
	var closers []io.Closer

	if strings.TrimSpace(opts.File) != "" {
		logFile, err := openLogFile(opts.File)
		if err != nil {
			return nil, nil, err
		}
		closers = append(closers, logFile)
		handlers = append(handlers, slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: fileRuntimeLevel}))
	}

	if opts.DebugConsole || len(handlers) == 0 {
		consoleLevel := fileLevel
		if opts.DebugConsole {
			consoleLevel = slog.LevelDebug
		}
		consoleRuntimeLevel.Set(consoleLevel)
		if opts.DebugConsole {
			handlers = append(handlers, slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: consoleRuntimeLevel}))
		} else {
			handlers = append(handlers, slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: consoleRuntimeLevel}))
		}
	}

	return slog.New(fanoutHandler{handlers: handlers}), closeAll(closers), nil
}

func SetLevel(level string) error {
	slogLevel, err := parseLevel(level)
	if err != nil {
		return err
	}
	fileRuntimeLevel.Set(slogLevel)
	consoleRuntimeLevel.Set(slogLevel)
	return nil
}

type fanoutHandler struct {
	handlers []slog.Handler
}

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return fanoutHandler{handlers: handlers}
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return fanoutHandler{handlers: handlers}
}

func openLogFile(path string) (*os.File, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("log.file cannot be empty")
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create log directory %q: %w", dir, err)
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	return file, nil
}

func closeAll(closers []io.Closer) func() error {
	return func() error {
		var firstErr error
		for _, closer := range closers {
			if err := closer.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}

func parseLevel(level string) (slog.Level, error) {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
	return slogLevel, nil
}
