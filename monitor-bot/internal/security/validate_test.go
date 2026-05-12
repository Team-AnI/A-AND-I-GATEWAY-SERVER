package security

import "testing"

func TestValidateServiceAllowlist(t *testing.T) {
	for _, service := range []string{"gateway", "auth", "report", "online-judge", "post"} {
		if !ValidateService(service) {
			t.Fatalf("expected %s to be valid", service)
		}
	}
	if ValidateService("redis") {
		t.Fatal("redis must not be accepted")
	}
}

func TestNormalizeServiceOrAll(t *testing.T) {
	if got, ok := NormalizeServiceOrAll("all"); !ok || got != "all" {
		t.Fatalf("expected all to be accepted, got %q ok=%v", got, ok)
	}
	if _, ok := NormalizeServiceOrAll("redis"); ok {
		t.Fatal("redis must not be accepted")
	}
}

func TestParseSinceAllowlist(t *testing.T) {
	if _, ok := ParseSince("15m"); !ok {
		t.Fatal("15m should be accepted")
	}
	if _, ok := ParseSince("2h"); ok {
		t.Fatal("2h must not be accepted")
	}
}

func TestNormalizeLevelAllowlist(t *testing.T) {
	if level, ok := NormalizeLevel("error"); !ok || level != "ERROR" {
		t.Fatalf("expected ERROR, got %q ok=%v", level, ok)
	}
	if _, ok := NormalizeLevel("DEBUG"); ok {
		t.Fatal("DEBUG must not be accepted")
	}
}

func TestDashboardCommandAllowlists(t *testing.T) {
	for _, value := range []string{"all", "api", "error", "4xx", "5xx"} {
		if got, ok := NormalizeCountType(value); !ok || got != value {
			t.Fatalf("expected count type %s to be accepted", value)
		}
	}
	if _, ok := NormalizeCountType("request.body"); ok {
		t.Fatal("unsafe count type accepted")
	}
	for _, value := range []string{"path", "error", "status"} {
		if got, ok := NormalizeTopBy(value); !ok || got != value {
			t.Fatalf("expected top by %s to be accepted", value)
		}
	}
	if _, ok := NormalizeTopBy("token"); ok {
		t.Fatal("unsafe top by accepted")
	}
	if got := ClampLimit(50, 10, 20); got != 20 {
		t.Fatalf("limit should be clamped to 20, got %d", got)
	}
}

func TestValidateTraceID(t *testing.T) {
	if !ValidateTraceID("abc.DEF_123:-xyz") {
		t.Fatal("valid trace id rejected")
	}
	if ValidateTraceID("abc; drop query") {
		t.Fatal("trace id with semicolon must be rejected")
	}
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	if ValidateTraceID(string(long)) {
		t.Fatal("overly long trace id must be rejected")
	}
}
