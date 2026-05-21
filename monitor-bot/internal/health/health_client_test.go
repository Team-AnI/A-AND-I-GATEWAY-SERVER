package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckReturnsUnknownWhenHealthURLMissing(t *testing.T) {
	client := NewClient(map[string]string{"auth": ""}, 10*time.Millisecond)

	got := client.Check(context.Background(), "auth")

	if got.State != "UNKNOWN" {
		t.Fatalf("expected UNKNOWN for missing health URL, got %#v", got)
	}
}

func TestCheckReturnsUnknownWhenHealthURLIsUnreachable(t *testing.T) {
	client := NewClient(map[string]string{"auth": "http://127.0.0.1:1/healthz"}, 10*time.Millisecond)

	got := client.Check(context.Background(), "auth")

	if got.State != "UNKNOWN" {
		t.Fatalf("expected UNKNOWN for unreachable health URL, got %#v", got)
	}
}

func TestCheckTreatsOKHealthStatusAsUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	client := NewClient(map[string]string{"post": server.URL + "/health"}, time.Second)

	got := client.Check(context.Background(), "post")

	if got.State != "UP" || got.Detail != "ok" {
		t.Fatalf("expected UP for ok health status, got %#v", got)
	}
}
