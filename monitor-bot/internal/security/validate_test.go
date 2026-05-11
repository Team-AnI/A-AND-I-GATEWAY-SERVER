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
