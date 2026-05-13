package formatting

import (
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
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

func TestFormatAdminAssignmentsAndSubmissions(t *testing.T) {
	assignments := []reportadmin.Assignment{{
		ID:        "a1",
		Status:    "published",
		StartAt:   "2026-05-13T09:00:00+09:00",
		EndAt:     "2026-05-20T09:00:00+09:00",
		ProblemID: "p1",
		UpdatedAt: "2026-05-13T10:00:00+09:00",
	}}
	got := FormatAdminAssignments("kotlin-basic", "all", assignments)
	for _, want := range []string{"status: OK", "source: WEB_ADMIN_API", "course: kotlin-basic", "1 assignments found", "published"} {
		if !strings.Contains(got, want) {
			t.Fatalf("admin assignments missing %q: %s", want, got)
		}
	}
	check := FormatAdminAssignmentCheck("kotlin-basic", assignments[0], reportadmin.CheckAssignment(assignments[0]))
	if !strings.Contains(check, "status: OK") || !strings.Contains(check, "source: WEB_ADMIN_API") {
		t.Fatalf("assignment check output unexpected: %s", check)
	}
	submissions := FormatAdminSubmissions("kotlin-basic", "a1", reportadmin.SubmissionSummary{TotalStudents: 2, Submitted: 1, NotSubmitted: 1, Graded: 1, AverageScore: "80"})
	for _, want := range []string{"total students: 2", "submitted: 1", "average score: 80"} {
		if !strings.Contains(submissions, want) {
			t.Fatalf("submission summary missing %q: %s", want, submissions)
		}
	}
}

func TestFormatCloudWatchFallbackMarksNotAuthoritative(t *testing.T) {
	got := FormatCloudWatchFallback("Assignment", []map[string]string{{"trace.traceId": "abc", "http.path": "/v2/admin/courses/kotlin/assignments"}})
	if !strings.Contains(got, "CLOUDWATCH_FALLBACK") || !strings.Contains(got, "not authoritative") {
		t.Fatalf("fallback output must be clearly marked: %s", got)
	}
}

func TestAdminFormattingDoesNotLeakToken(t *testing.T) {
	outputs := []string{
		FormatAdminError("AUTH_ERROR", "kotlin", "a1", "Authorization: Bearer secret-token token=secret-token"),
		FormatAdminAssignments("kotlin", "all", []reportadmin.Assignment{{ID: "a1", Title: "token=secret-token", Status: "published"}}),
	}
	for _, got := range outputs {
		for _, forbidden := range []string{"secret-token", "Bearer secret-token"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("admin formatting leaked token %q: %s", forbidden, got)
			}
		}
	}
}

func TestWithAdminNoticePrependsLegacyNotice(t *testing.T) {
	got := WithAdminNotice("status: OK", "참고: 이 코스는 레거시/종료 코스로 보입니다.")
	if !strings.HasPrefix(got, "참고: 이 코스는 레거시/종료 코스로 보입니다.") || !strings.Contains(got, "status: OK") {
		t.Fatalf("notice should be prepended: %s", got)
	}
	if len([]rune(got)) > DiscordMessageLimit {
		t.Fatal("notice output exceeds Discord limit")
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

func TestFormatErrorsSurfacesReportCopyAPIErrors(t *testing.T) {
	got := FormatErrors([]map[string]string{{
		"count":                  "3",
		"http.path":              "/v2/admin/courses/java-basic/assignments/copy",
		"http.statusCode":        "409",
		"response.error.code":    "44091",
		"response.error.value":   "CONFLICT",
		"response.error.message": "duplicated assignment copy",
	}})

	for _, want := range []string{"/v2/admin/courses/java-basic/assignments/copy", "409", "44091", "count=3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("copy API error should be visible through errors formatting, missing %q: %s", want, got)
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
	for _, expected := range []string{"A&I 서비스 운영 대시보드", "```txt", "Service", "Health", "Logs", "Err", "report", "OK", "3", "최근 장애 알림", "상세 확인"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("dashboard missing %q: %s", expected, got)
		}
	}
	if len([]rune(got)) > DiscordMessageLimit {
		t.Fatalf("dashboard exceeds discord limit")
	}
}

func TestFormatDashboardShowsUnknownNoLogsAndNotConfigured(t *testing.T) {
	lastLog := time.Now().Add(-18 * time.Minute).Format(time.RFC3339)
	got := FormatDashboard("30m", []DashboardServiceInput{
		{
			Service:     "gateway",
			DisplayName: "gateway",
			Health:      ServiceStatus{Service: "gateway", State: "UP"},
			LogStatus:   "OK",
			Rows:        []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200", "lastLog": lastLog}},
		},
		{
			Service:     "auth",
			DisplayName: "auth",
			Health:      ServiceStatus{Service: "auth", State: "UNKNOWN"},
			LogStatus:   "OK",
			Rows:        []map[string]string{{"count": "1", "logType": "API_ERROR", "level": "ERROR", "http.statusCode": "500"}},
		},
		{
			Service:     "online-judge",
			DisplayName: "online-judge",
			Health:      ServiceStatus{Service: "online-judge", State: "UNKNOWN"},
			LogStatus:   "NO_LOGS",
		},
		{
			Service:     "post",
			DisplayName: "post",
			Health:      ServiceStatus{Service: "post", State: "NOT_CONFIGURED"},
			LogStatus:   "NOT_CONFIGURED",
		},
	}, nil)
	for _, expected := range []string{"gateway", "auth", "judge", "post", "UNK", "NOLOG", "NOCFG", "18m"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("dashboard missing %q: %s", expected, got)
		}
	}
	for _, long := range []string{"online-judge", "UNKNOWN", "NO_LOGS", "NOT_CONFIGURED", "Last log"} {
		if strings.Contains(got, long) {
			t.Fatalf("dashboard should use compact labels and omit %q: %s", long, got)
		}
	}
	if strings.Index(got, "gateway") > strings.Index(got, "auth") || strings.Index(got, "auth") > strings.Index(got, "judge") || strings.Index(got, "judge") > strings.Index(got, "post") {
		t.Fatalf("dashboard did not preserve registry order: %s", got)
	}
}

func TestFormatDashboardShowsUnconnectedAsUnknownNoLogs(t *testing.T) {
	got := FormatDashboard("30m", []DashboardServiceInput{
		{
			Service:   "report",
			Health:    ServiceStatus{Service: "report", State: "UP"},
			LogStatus: "OK",
			Rows:      []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200"}},
		},
		{
			Service:   "auth",
			Health:    ServiceStatus{Service: "auth", State: "UNKNOWN", Detail: "not connected in service ops phase"},
			LogStatus: "NO_LOGS",
		},
	}, nil)
	if !strings.Contains(got, "auth") || !strings.Contains(got, "UNK") || !strings.Contains(got, "NOLOG") {
		t.Fatalf("dashboard should display unconnected services as UNK/NOLOG: %s", got)
	}
	if !strings.Contains(got, "전체 상태: 🟢 정상") {
		t.Fatalf("unconnected services alone should not make dashboard warning: %s", got)
	}
}

func TestFormatDashboardShowsRecentServiceAlerts(t *testing.T) {
	got := FormatDashboardWithMetaAndAlerts("30m", []DashboardServiceInput{{
		Service:   "report",
		Health:    ServiceStatus{Service: "report", State: "UP"},
		LogStatus: "OK",
		Rows:      []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200"}},
	}}, nil, time.Date(2026, 5, 13, 17, 10, 0, 0, time.FixedZone("KST", 9*60*60)), 3*time.Minute, []string{"report/web 장애 - 최근 5분 5xx 임계치 초과"})
	for _, want := range []string{"마지막 업데이트", "업데이트 주기: 3m", "최근 장애 알림", "report/web 장애"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dashboard missing %q: %s", want, got)
		}
	}
}

func TestFormatDashboardDoesNotLeakSensitiveRawFields(t *testing.T) {
	got := FormatDashboard("30m", []DashboardServiceInput{{
		Service:   "report",
		Health:    ServiceStatus{Service: "report", State: "UP"},
		LogStatus: "OK",
		Rows: []map[string]string{{
			"count":           "1",
			"logType":         "API_ERROR",
			"level":           "ERROR",
			"http.statusCode": "500",
			"@message":        `{"request":{"body":"raw-secret"},"response":{"data":"secret-data"}}`,
			"request.body":    "secret-body",
			"response.data":   "secret-data",
			"message":         "failed token=secret password=secret",
			"userCode":        "secret-code",
		}},
	}}, nil)
	for _, forbidden := range []string{"raw-secret", "secret-data", "secret-body", "secret-code", "token=secret", "password=secret", "@message"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("dashboard leaked %q: %s", forbidden, got)
		}
	}
	if len([]rune(got)) > DiscordMessageLimit {
		t.Fatalf("dashboard exceeds discord limit")
	}
}

func TestFormatTopSummaryOnlyShowsProvidedRows(t *testing.T) {
	got := FormatTopSummary("report", "30m", "path", []map[string]string{
		{"http.path": "/v2/report", "count": "3"},
	})
	if !strings.Contains(got, "/v2/report") {
		t.Fatalf("top summary missing provided row: %s", got)
	}
	if strings.Contains(got, "gateway") || strings.Contains(got, "auth") || strings.Contains(got, "online-judge") || strings.Contains(got, "post") {
		t.Fatalf("top summary should not render registry services: %s", got)
	}
}

func TestFormatTopSummaryHonorsLimit(t *testing.T) {
	rows := make([]map[string]string, 12)
	for i := range rows {
		rows[i] = map[string]string{
			"http.path": "/v2/items",
			"count":     "1",
		}
	}

	got := FormatTopSummaryWithLimit("report", "30m", "path", rows, 5)
	if count := strings.Count(got, "count=1"); count != 5 {
		t.Fatalf("expected 5 top rows, got %d: %s", count, got)
	}

	got = FormatTopSummaryWithLimit("report", "30m", "path", rows, 20)
	if count := strings.Count(got, "count=1"); count != 12 {
		t.Fatalf("expected all 12 provided rows with limit 20, got %d: %s", count, got)
	}
}

func TestFormatAssignmentsSummaryAndDetail(t *testing.T) {
	empty := FormatAssignmentsSummary("1h", nil, "", "")
	for _, want := range []string{"status: NO_DATA", "service: report", "과제 관련 로그 없음", "/ops logs service:report mode:errors"} {
		if !strings.Contains(empty, want) {
			t.Fatalf("assignment summary missing %q: %s", want, empty)
		}
	}
	detail := FormatAssignmentDetail("assignment-123", nil, "", "")
	for _, want := range []string{"status: NO_DATA", "assignmentId: assignment-123", "no matching records"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("assignment detail missing %q: %s", want, detail)
		}
	}
	rows := []map[string]string{{"count": "2", "http.path": "/v2/admin/courses/java/assignments", "http.statusCode": "500", "response.error.code": "49000"}}
	warn := FormatAssignmentsSummary("30m", rows, "", "")
	if !strings.Contains(warn, "status: ERROR") || !strings.Contains(warn, "49000") {
		t.Fatalf("assignment summary should surface report errors: %s", warn)
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
		FormatAssignmentsSummary("1h", rows, "", ""),
		FormatAssignmentDetail("abc123", rows, "", ""),
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

func TestFormatRetentionShowsInfiniteAndHumanBytes(t *testing.T) {
	retention := int32(14)
	got := FormatRetention("📦 CloudWatch Log Retention", []LogGroupRetention{
		{Name: "/a-and-i/gateway", RetentionDays: &retention, StoredBytes: 222298112},
		{Name: "/a-and-i/prod/report", StoredBytes: 0},
	})
	for _, expected := range []string{"/a-and-i/gateway", "14d", "212MB", "/a-and-i/prod/report", "INFINITE"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("retention output missing %q: %s", expected, got)
		}
	}
	if len([]rune(got)) > DiscordMessageLimit {
		t.Fatalf("retention output exceeds discord limit")
	}
}

func TestFormatRetentionDoesNotExposeDeleteOperationsOrSecrets(t *testing.T) {
	got := FormatRetention("💽 CloudWatch Log Usage", []LogGroupRetention{
		{Name: "/a-and-i/prod/monitor-bot", StoredBytes: 3 * 1024 * 1024},
	})
	for _, forbidden := range []string{"delete", "prune", "request.body", "response.data", "token", "password"} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Fatalf("retention output should stay read-only and non-sensitive, found %q in %s", forbidden, got)
		}
	}
}

func TestHelpUsesOpsFocusedOutput(t *testing.T) {
	got := HelpText()
	for _, want := range []string{
		"A&I Ops Incident Flow",
		"/ops dashboard since:30m",
		"/ops logs service:all mode:errors since:15m limit:10",
		"/ops logs service:report mode:errors since:30m limit:10",
		"/ops logs service:report mode:slow since:30m limit:10",
		"/ops trace trace_id:<traceId>",
		"/ops assignments course:<courseSlug> status:all",
		"/ops assignment-check course:<courseSlug> id:<assignmentId>",
		"/ops submissions course:<courseSlug> assignment:<assignmentId>",
		"Assignments use WEB-SERVER admin API first; CloudWatch is fallback only.",
		"Use /ops service for service state.",
		"Use /ops logs for log analysis.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help text missing %q: %s", want, got)
		}
	}
	for _, legacy := range []string{"/dashboard since:", "/service service:", "/logs service:", "/errors service:", "/ops " + "copy", "/ops service service:report view:copy", "/ops storage view:usage"} {
		if strings.Contains(got, legacy) {
			t.Fatalf("help text should be ops-focused and omit legacy command %q: %s", legacy, got)
		}
	}
}
