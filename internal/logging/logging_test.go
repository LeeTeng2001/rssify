package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewTintLoggerWritesMessage(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New("debug", "tint", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	logger.Info("hello", slog.String("feed", "hn"))
	out := buf.String()
	if !strings.Contains(out, "hello") || !strings.Contains(out, "feed") || !strings.Contains(out, "hn") {
		t.Fatalf("tint output missing expected content: %q", out)
	}
}

func TestNewJSONLoggerWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New("info", "json", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	logger.Info("hello")
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Fatalf("json output missing message: %q", out)
	}
}

func TestNewRejectsInvalidLevelAndFormat(t *testing.T) {
	if _, err := New("verbose", "tint", &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid level error")
	}
	if _, err := New("info", "xml", &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid format error")
	}
}
