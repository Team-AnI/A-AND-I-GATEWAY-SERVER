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

func TestFormatDashboardAndAggregations(t *testing.T) {
	rows := []map[string]string{
		{"count": "9", "logType": "API", "level": "INFO", "http.statusCode": "200", "p95": "148", "lastLog": "2026-04-14T20:31:12+09:00"},
		{"count": "3", "logType": "API_ERROR", "level": "WARN", "http.statusCode": "409", "response.error.code": "44091"},
		{"count": "1", "logType": "API_ERROR", "level": "ERROR", "http.statusCode": "500", "response.error.code": "49000"},
	}
	summary := SummarizeRows(rows)
	if summary.Total != 13 || summary.APIError != 4 || summary.FourXX != 3 || summary.FiveXX != 1 || summary.P95 != 148 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	got := FormatDashboard("30m", []DashboardServiceInput{{
		Service: "report",
		Health:  ServiceStatus{Service: "report", State: "UP", Detail: "UP"},
		Rows:    rows,
	}}, nil)
	for _, expected := range []string{"A&I Service Dashboard", "report", "13", "3"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("dashboard missing %q: %s", expected, got)
		}
	}
	if len([]rune(got)) > DiscordMessageLimit {
		t.Fatalf("dashboard exceeds discord limit")
	}
}

func TestFormatNewCommandsDoNotLeakSensitiveRawFields(t *testing.T) {
	rows := []map[string]string{{
		"count":                  "2",
		"http.method":            "POST",
		"http.path":              "/v2/admin/courses/java-basic/assignments/copy",
		"http.statusCode":        "409",
		"http.latencyMs":         "640",
		"trace.traceId":          "abc123",
		"response.error.code":    "44091",
		"response.error.message": "duplicated assignment copy password=secret",
		"@message":               `{"request":{"body":"raw-secret"},"response":{"data":"secret-data"}}`,
		"request.body":           "secret-body",
		"response.data":          "secret-data",
		"userCode":               "secret-code",
		"privateTestCases":       "secret-testcase",
	}}
	outputs := []string{
		FormatCountSummary("report", "1h", "error", rows),
		FormatTopSummary("report", "1h", "error", rows),
		FormatSlowSummary("report", "1h", rows),
		FormatCopyStatus("1h", rows),
		FormatServiceDetail(ServiceDetailInput{
			Service:   "report",
			LogGroup:  "/a-and-i/prod/report",
			Since:     "1h",
			Health:    ServiceStatus{Service: "report", State: "UP"},
			CountRows: rows,
			TopRows:   rows,
			ErrorRows: rows,
		}),
	}
	for _, got := range outputs {
		for _, forbidden := range []string{"raw-secret", "secret-data", "secret-body", "secret-code", "secret-testcase", "password=secret", "@message"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("formatted output leaked %q: %s", forbidden, got)
			}
		}
		if len([]rune(got)) > DiscordMessageLimit {
			t.Fatalf("output exceeds discord limit")
		}
	}
}
