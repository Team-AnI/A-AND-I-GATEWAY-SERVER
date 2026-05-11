package formatting

import (
	"strings"
	"testing"
)

func TestTruncateDiscordMessage(t *testing.T) {
	long := strings.Repeat("a", 2500)
	got := TruncateDiscordMessage(long)
	if len([]rune(got)) > DiscordMessageLimit {
		t.Fatalf("message exceeds discord limit: %d", len([]rune(got)))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatal("expected truncation suffix")
	}
}

func TestFormatStatus(t *testing.T) {
	got := FormatStatus([]ServiceStatus{{Service: "gateway", State: "UP", Detail: "UP"}})
	if !strings.Contains(got, "gateway") || !strings.Contains(got, "UP") {
		t.Fatalf("unexpected status format: %s", got)
	}
}

func TestFormatErrorsAndTraceOmitSensitiveFields(t *testing.T) {
	rows := []map[string]string{{
		"@timestamp":           "now",
		"env":                  "prod",
		"service.name":         "gateway",
		"service.domainCode":   "4",
		"service.version":      "2.0.8",
		"http.route":           "/v2/admin/courses/{targetCourseSlug}/assignments/copy",
		"actor.userId":         "12345",
		"actor.role":           "ADMIN",
		"response.success":     "false",
		"response.error.code":  "E001",
		"response.error.alert": "이미 복사된 과제입니다.",
		"tags":                 "[report, assignment, copy, fail, admin]",
		"@message":             `{"request":{"body":"raw-secret"}}`,
		"request.body":         "secret-body",
		"response.data":        "secret-data",
		"userCode":             "secret-code",
		"message":              "failed accessToken=abc",
	}}
	for _, got := range []string{FormatErrors(rows), FormatTrace(rows)} {
		for _, forbidden := range []string{"secret-body", "secret-data", "secret-code", "abc"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("formatted output leaked %q: %s", forbidden, got)
			}
		}
		if !strings.Contains(got, "E001") {
			t.Fatalf("allowed error code missing: %s", got)
		}
		if strings.Contains(got, "raw-secret") || strings.Contains(got, "@message") {
			t.Fatalf("raw @message leaked: %s", got)
		}
		if !strings.Contains(got, "report") || !strings.Contains(got, "ADMIN") {
			t.Fatalf("v2 allowlist fields missing: %s", got)
		}
	}
}
