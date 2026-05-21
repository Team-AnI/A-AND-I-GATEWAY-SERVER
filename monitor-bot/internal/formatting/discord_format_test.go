package formatting

import (
	"os"
	"path/filepath"
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
			DisplayName: "blog",
			Health:      ServiceStatus{Service: "post", State: "NOT_CONFIGURED"},
			LogStatus:   "NOT_CONFIGURED",
		},
	}, nil)
	for _, expected := range []string{"gateway", "auth", "judge", "blog", "UNK", "NOLOG", "NOCFG", "18m"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("dashboard missing %q: %s", expected, got)
		}
	}
	for _, long := range []string{"online-judge", "UNKNOWN", "NO_LOGS", "NOT_CONFIGURED", "Last log", "post"} {
		if strings.Contains(got, long) {
			t.Fatalf("dashboard should use compact labels and omit %q: %s", long, got)
		}
	}
	if strings.Index(got, "gateway") > strings.Index(got, "auth") || strings.Index(got, "auth") > strings.Index(got, "judge") || strings.Index(got, "judge") > strings.Index(got, "blog") {
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
		"A&I Ops Bot 도움말",
		"기본 명령은 5개입니다",
		"1. /ops dashboard",
		"2. /ops logs",
		"3. /ops alert",
		"4. /ops assignment",
		"5. /ops help",
		"/ops dashboard service:report",
		"/ops logs service:report mode:errors since:30m limit:10",
		"/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20",
		"/ops logs mode:trace query:<traceId>",
		"/ops alert action:channel target:general channel:#ops-log",
		"/ops alert action:channel target:critical channel:#ops-critical",
		"/ops alert action:role role:@운영팀",
		"/ops assignment course:3rd-cs",
		"/ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis",
		"/ops assignment course:<courseSlug> id:<assignmentId> view:events",
		"봇은 과제를 생성/수정/삭제/공개하지 않습니다",
		"CRITICAL 서버 장애만 role mention",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help text missing %q: %s", want, got)
		}
	}
	for _, legacy := range []string{"/dashboard since:", "/service service:", "/logs service:", "/errors service:", "/ops " + "copy", "/ops service", "/ops trace", "/ops watch", "/ops logs-watch", "/ops assignments", "/ops assignment-check", "/ops submissions"} {
		if strings.Contains(got, legacy) {
			t.Fatalf("help text should be ops-focused and omit legacy command %q: %s", legacy, got)
		}
	}
}

func TestHelpTopicAssignmentsAndCommandAssignmentCheckExplainPurpose(t *testing.T) {
	topic := HelpTextFor("assignments", "", "")
	for _, want := range []string{"/ops assignment", "action:list", "action:check", "action:submissions", "view:diagnosis", "view:events", "action:ack", "query:<assignmentId|traceId|actorId>", "Assignment audit notifications", "bot은 과제를 생성/수정/삭제/공개하지 않습니다", "mode:events"} {
		if !strings.Contains(topic, want) {
			t.Fatalf("assignment topic missing %q: %s", want, topic)
		}
	}
	command := HelpTextFor("", "assignment", "")
	for _, want := range []string{"역할:", "확인하는 것:", "problemId", "view:events", "action:ack"} {
		if !strings.Contains(command, want) {
			t.Fatalf("assignment help missing %q: %s", want, command)
		}
	}
}

func TestHelpTopicsCoverRoutingAuditAndTroubleshooting(t *testing.T) {
	cases := map[string][]string{
		"dashboard":       {"/ops dashboard action:watch", "service/watch/unwatch"},
		"logs":            {"mode:critical", "mode:events", "mode:trace", "@message는 fallback"},
		"alerts":          {"target:general", "target:critical", "HIGH/general/audit/WARN"},
		"routing":         {"general route", "critical route", "role mention"},
		"audit":           {"Report V2 EVENT logs", "ASSIGNMENT_CREATED", "WEB Admin API snapshot에서 actor를 추측하지 않습니다"},
		"troubleshooting": {"DISCORD_REGISTER_COMMANDS=true", "Too many assignment warnings", "mode:trace"},
	}
	for topic, wants := range cases {
		got := HelpTextFor(topic, "", "")
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Fatalf("help topic %s missing %q: %s", topic, want, got)
			}
		}
	}
}

func TestHelpCommandPagesMatchFiveCommandFamilies(t *testing.T) {
	cases := map[string][]string{
		"dashboard":  {"전체/단일 서비스 상태", "action:watch", "예전 service/watch"},
		"logs":       {"mode:events", "mode:trace", "structured V2 fields"},
		"alert":      {"target:general", "target:critical", "CRITICAL 서버 장애만"},
		"assignment": {"action:submissions", "view:events", "read-only"},
		"help":       {"query:", "topic:<topic>", "command:<command>"},
	}
	for command, wants := range cases {
		got := HelpTextFor("", command, "ignored query")
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Fatalf("help command %s missing %q: %s", command, want, got)
			}
		}
	}
}

func TestHelpQueryExplainsAssignmentAuditAndRouting(t *testing.T) {
	got := HelpTextFor("", "", "과제 수정 누가")
	for _, want := range []string{"Report EVENT logs", "query:<assignmentId|traceId|actorId>", "/ops assignment course:<courseSlug> id:<assignmentId> view:events", "현재 assignment state does not prove actor", "bot은 과제를 업데이트하지 않습니다"} {
		if !strings.Contains(got, want) {
			t.Fatalf("assignment audit query help missing %q: %s", want, got)
		}
	}
	got = HelpTextFor("", "", "과제 삭제 언제")
	for _, want := range []string{"mode:events", "query:<assignmentId|traceId|actorId>", "삭제된 assignment는 /ops assignment course:<courseSlug> id:<assignmentId>에서 더 이상 조회되지 않을 수 있습니다", "Report V2 EVENT logs", "현재 WEB Admin state"} {
		if !strings.Contains(got, want) {
			t.Fatalf("assignment deletion query help missing %q: %s", want, got)
		}
	}
	got = HelpTextFor("", "", "critical role")
	for _, want := range []string{"target:critical", "/ops alert action:role", "CRITICAL only", "HIGH/general/audit/WARN do not role-mention"} {
		if !strings.Contains(got, want) {
			t.Fatalf("critical routing query help missing %q: %s", want, got)
		}
	}
	got = HelpTextFor("", "", "critical 알림 role")
	for _, forbidden := range []string{"반복 알림 정책", "lifecycle state", "action:ack event:<eventType>"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("critical alert role query should not use repeated-alert help %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "target:critical") || !strings.Contains(got, "CRITICAL only") {
		t.Fatalf("critical alert role query should route to critical help: %s", got)
	}
	got = HelpTextFor("", "", "일반 로그 채널")
	for _, want := range []string{"target:general", "assignment audit", "normal ops logs go to general", "CRITICAL goes to critical"} {
		if !strings.Contains(got, want) {
			t.Fatalf("general channel query help missing %q: %s", want, got)
		}
	}
	got = HelpTextFor("", "", "로그 검색")
	for _, want := range []string{"mode:errors", "mode:trace", "mode:events"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log search query help missing %q: %s", want, got)
		}
	}
	got = HelpTextFor("", "", "반복 알림")
	for _, want := range []string{"assignment issues are lifecycle state, not an event stream", "cooldown", "digest groups repeated assignment issues", "action:ack event:<eventType>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("repeat alert query help missing %q: %s", want, got)
		}
	}
	got = HelpTextFor("", "", "과제 공개 지연")
	for _, want := range []string{"ASSIGNMENT_PUBLISH_DELAYED", "publishedAt", "ASSIGNMENT_DRAFT_PAST_START", "stale draft"} {
		if !strings.Contains(got, want) {
			t.Fatalf("publish delay query help missing %q: %s", want, got)
		}
	}
	got = HelpTextFor("", "", "태그 배포")
	for _, want := range []string{"tag/deploy", "git tag", "DISCORD_REGISTER_COMMANDS=true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tag deploy query help missing %q: %s", want, got)
		}
	}
}

func TestHelpExamplesUseExplicitPlaceholders(t *testing.T) {
	pages := map[string]string{
		"default":         HelpText(),
		"assignments":     HelpTextFor("assignments", "", ""),
		"logs":            HelpTextFor("logs", "", ""),
		"audit":           HelpTextFor("audit", "", ""),
		"troubleshooting": HelpTextFor("troubleshooting", "", ""),
		"assignment":      HelpTextFor("", "assignment", ""),
		"assignmentCheck": HelpTextFor("", "assignment-check", ""),
		"deleteQuery":     HelpTextFor("", "", "과제 삭제 언제"),
		"updateQuery":     HelpTextFor("", "", "과제 수정 누가"),
		"repeatQuery":     HelpTextFor("", "", "반복 알림"),
	}
	for name, got := range pages {
		for _, forbidden := range []string{"id:<id>", "event:<event>", "reason:\"old draft\"", "/ops logs ... query:", "query:<assignmentId> since:24h"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("help page %s has ambiguous placeholder %q: %s", name, forbidden, got)
			}
		}
	}
	for name, got := range pages {
		for _, want := range []string{"<assignmentId>", "<courseSlug>"} {
			if strings.Contains(got, "/ops assignment") && !strings.Contains(got, want) {
				t.Fatalf("help page %s should include explicit placeholder %q: %s", name, want, got)
			}
		}
	}
}

func TestMonitorBotDocsMatchCurrentUX(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	files := map[string]string{
		"root":    filepath.Join(root, "README.md"),
		"bot":     filepath.Join(root, "monitor-bot", "README.md"),
		"opsdocs": filepath.Join(root, "docs", "discord-monitor-bot.md"),
	}
	contents := make(map[string]string, len(files))
	for name, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s docs: %v", name, err)
		}
		contents[name] = string(body)
	}
	for _, want := range []string{"Discord Monitor Bot", "monitor-bot/README.md", "docs/discord-monitor-bot.md", "bot never creates/updates/deletes/publishes assignments", "Report V2 EVENT logs", "CRITICAL server alerts"} {
		if !strings.Contains(contents["root"], want) {
			t.Fatalf("root README missing %q", want)
		}
	}
	for _, want := range []string{"## UX Contract", "/ops dashboard", "/ops logs", "/ops alert", "/ops assignment", "/ops help", "no ASSIGNMENT_OPS_CHANNEL_ID", "No tag"} {
		if !strings.Contains(contents["bot"], want) {
			t.Fatalf("monitor-bot README missing %q", want)
		}
	}
	for _, want := range []string{"Legacy Command Migration", "/ops service", "/ops dashboard service:<service>", "same assignment issue does not resend every cooldown", "DISCORD_REGISTER_COMMANDS=true", "No tag deployment"} {
		if !strings.Contains(contents["opsdocs"], want) {
			t.Fatalf("discord monitor docs missing %q", want)
		}
	}
	for name, content := range contents {
		for _, forbidden := range []string{
			"query: since",
			"query: \n",
			"course: id:",
			"event: until",
			"/ops logs service: mode:",
			"/ops logs mode:trace query:\n",
			"/ops assignment course:<course>",
			"/ops help topic:<dashboard|logs|alerts|assignments|routing|audit|troubleshooting>",
			"/ops help command:<dashboard|logs|alert|assignment|help>",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s docs contain bare placeholder %q", name, forbidden)
			}
		}
		for _, line := range strings.Split(content, "\n") {
			if strings.HasSuffix(line, "reason:") || strings.HasSuffix(line, "query:") {
				t.Fatalf("%s docs contain bare placeholder line %q", name, line)
			}
		}
	}
	for _, want := range []string{
		"query:<traceId>",
		"query:<assignmentId|traceId|actorId>",
		"course:<courseSlug>",
		"id:<assignmentId>",
		"event:<eventType>",
		"reason:<reason>",
	} {
		if !strings.Contains(contents["bot"], want) {
			t.Fatalf("monitor-bot README missing explicit placeholder %q", want)
		}
		if !strings.Contains(contents["opsdocs"], want) {
			t.Fatalf("discord monitor docs missing explicit placeholder %q", want)
		}
	}
	defaultHelpSection := contents["opsdocs"]
	if strings.Contains(defaultHelpSection, "Primary commands:\n\n- `/ops service") {
		t.Fatal("discord monitor docs must not present legacy commands as primary UX")
	}
}

func TestFormatAdminAssignmentCheckChecklistIncludesBotIssue(t *testing.T) {
	got := FormatAdminAssignmentCheck("3rd-cs", reportadmin.Assignment{
		ID:      "1d74df8d-c501-405e-9327-d8f39b4d98cb",
		Status:  "DRAFT",
		StartAt: "2025-05-19T09:00:00+09:00",
		EndAt:   "2025-05-23T18:00:00+09:00",
	}, reportadmin.AssignmentCheck{Status: reportadmin.StatusWarn, Findings: []string{"problemId가 비어 있습니다."}}, "OPEN")
	for _, want := range []string{"checks:", "title: MISSING", "problemId: MISSING", "botIssue: OPEN", "does not explain ASSIGNMENT_DRAFT_PAST_START"} {
		if !strings.Contains(got, want) {
			t.Fatalf("assignment check missing %q: %s", want, got)
		}
	}
}
