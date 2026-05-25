package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteUsesOpenAICompatibleEndpoint(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "hi") {
			t.Errorf("request body missing user message content")
		}
		w.Header().Set("Content-Type", "application/json")
		resp := "{\"choices\":[{\"message\":{\"content\":\"```toml\\n[feed.rule]\\nitem = \\\".item\\\"\\n```\"}}]}"
		io.WriteString(w, resp)
	}))
	defer ts.Close()

	c := New(ts.URL, "secret", "test-model")
	out, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(out, "[feed.rule]") {
		t.Errorf("output missing [feed.rule], got %q", out)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("auth = %q, want Bearer secret", gotAuth)
	}
}