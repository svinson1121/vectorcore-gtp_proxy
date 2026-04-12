package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var runtimeLevel = &slog.LevelVar{}

func New(level string) (*slog.Logger, error) {
	slogLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	runtimeLevel.Set(slogLevel)

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: runtimeLevel})
	return slog.New(handler), nil
}

func SetLevel(level string) error {
	slogLevel, err := parseLevel(level)
	if err != nil {
		return err
	}
	runtimeLevel.Set(slogLevel)
	return nil
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
