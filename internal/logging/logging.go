package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

func New(levelName, format string, w io.Writer) (*slog.Logger, error) {
	level, err := parseLevel(levelName)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(format) {
	case "tint":
		return slog.New(tint.NewHandler(w, &tint.Options{
			Level:      level,
			TimeFormat: time.Kitchen,
		})), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})), nil
	default:
		return nil, fmt.Errorf("invalid log format %q", format)
	}
}

func parseLevel(levelName string) (slog.Level, error) {
	switch strings.ToLower(levelName) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level %q", levelName)
	}
}
