package health

import (
	"context"
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
