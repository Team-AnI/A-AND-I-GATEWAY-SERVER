package cloudwatch

import (
	"strings"
	"testing"
)

func TestBuildQueriesValidateUserInput(t *testing.T) {
	if _, err := BuildRecentLogsQuery("report", `ERROR" or request.body like /secret/`, 20); err == nil {
		t.Fatal("unvalidated level accepted")
	}
	if _, err := BuildRecentLogsQuery(`report" or request.body like /secret/`, "ERROR", 20); err == nil {
		t.Fatal("unvalidated service accepted")
	}
	if _, err := BuildTraceQuery(`abc" or request.body like /secret/`); err == nil {
		t.Fatal("unsafe trace id accepted")
	}
}

func TestBuildQueriesUseAllowlistedValues(t *testing.T) {
	query, err := BuildRecentLogsQuery("report", "ERROR", 20)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(query, "request.body") || strings.Contains(query, "response.data") {
		t.Fatalf("query should not select raw sensitive fields: %s", query)
	}
	if !strings.Contains(query, `service.name = "report-service"`) {
		t.Fatalf("report query should filter report-service: %s", query)
	}
	traceQuery, err := BuildTraceQuery("trace_123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(traceQuery, `trace.traceId = "trace_123"`) {
		t.Fatalf("trace id not included after validation: %s", traceQuery)
	}
}

func TestBuildRecentAndErrorsQueriesApplyLimit(t *testing.T) {
	recentQuery, err := BuildRecentLogsQuery("report", "ERROR", 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recentQuery, "limit 5") {
		t.Fatalf("recent query should apply requested limit: %s", recentQuery)
	}
	errorsQuery := BuildErrorsQuery("report", 5)
	if !strings.Contains(errorsQuery, "limit 5") {
		t.Fatalf("errors query should apply requested limit: %s", errorsQuery)
	}
}

func TestLogGroupsForServiceAllowlist(t *testing.T) {
	groups := map[string]string{"gateway": "/a-and-i/gateway", "auth": "/a-and-i/auth"}
	got, err := LogGroupsForService(groups, "gateway")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/a-and-i/gateway" {
		t.Fatalf("unexpected log groups: %#v", got)
	}
	if _, err := LogGroupsForService(groups, "redis"); err == nil {
		t.Fatal("unsupported service accepted")
	}
}

func TestLogGroupsForReport(t *testing.T) {
	groups := map[string]string{"report": "/a-and-i/prod/report"}
	got, err := LogGroupsForService(groups, "report")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/a-and-i/prod/report" {
		t.Fatalf("unexpected report log group: %#v", got)
	}
}

func TestBuildErrorsQueryIncludesWarn(t *testing.T) {
	query := BuildErrorsQuery("report", 20)
	if !strings.Contains(query, `level = "WARN"`) {
		t.Fatalf("WARN should be included in errors query: %s", query)
	}
	if !strings.Contains(query, `service.name = "report-service"`) {
		t.Fatalf("report errors query should filter report-service: %s", query)
	}
}

func TestBuildDashboardAndAggregationQueriesValidateInput(t *testing.T) {
	if _, err := BuildDashboardSummaryQuery(`report" or request.body like /secret/`); err == nil {
		t.Fatal("unsafe dashboard service accepted")
	}
	if _, err := BuildCountQuery("report", `all" or token like /x/`); err == nil {
		t.Fatal("unsafe count type accepted")
	}
	if _, err := BuildTopQuery("report", "request.body", 10); err == nil {
		t.Fatal("unsafe top by accepted")
	}
	if _, err := BuildSlowQuery("redis", 0, 10); err == nil {
		t.Fatal("unsupported slow service accepted")
	}
}

func TestBuildDashboardQueriesUseSafeV2Fields(t *testing.T) {
	query, err := BuildDashboardSummaryQuery("report")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"request.body", "response.data", "@message"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("dashboard query selected forbidden field %s: %s", forbidden, query)
		}
	}
	if !strings.Contains(query, `service.name = "report-service"`) {
		t.Fatalf("report dashboard query should filter report-service: %s", query)
	}
	countQuery, err := BuildCountQuery("report", "5xx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(countQuery, "http.statusCode >= 500") {
		t.Fatalf("5xx query missing status filter: %s", countQuery)
	}
	slowQuery, err := BuildSlowQuery("report", 1000, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(slowQuery, "http.latencyMs >= 1000") || !strings.Contains(slowQuery, "limit 20") {
		t.Fatalf("slow query should include threshold and clamp limit: %s", slowQuery)
	}
}

func TestBuildAggregationQueriesAllowAllWithoutRawInput(t *testing.T) {
	countQuery, err := BuildCountQuery("all", "error")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(countQuery, `service.name =`) {
		t.Fatalf("all count query should not force one service filter: %s", countQuery)
	}
	topQuery, err := BuildTopQuery("all", "status", 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(topQuery, "request.body") || strings.Contains(topQuery, "response.data") {
		t.Fatalf("top query leaked forbidden fields: %s", topQuery)
	}
}

func TestBuildTopQueryAppliesLimit(t *testing.T) {
	query, err := BuildTopQuery("report", "path", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "limit 10") {
		t.Fatalf("top query should apply requested limit: %s", query)
	}
	query, err = BuildTopQuery("report", "path", 50)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "limit 20") {
		t.Fatalf("top query should clamp limit to 20: %s", query)
	}
}

func TestRetentionTargetLogGroups(t *testing.T) {
	got := RetentionTargetLogGroups(map[string]string{
		"gateway":      "/custom/gateway",
		"report":       "/a-and-i/prod/report",
		"auth":         "/a-and-i/auth",
		"online-judge": "/a-and-i/online-judge",
		"post":         "/a-and-i/prod/tech-blog",
	})
	expected := []string{
		"/custom/gateway",
		"/a-and-i/prod/monitor-bot",
		"/a-and-i/prod/report",
		"/a-and-i/prod/report-mongodb",
		"/a-and-i/auth",
		"/a-and-i/online-judge",
		"/a-and-i/prod/tech-blog",
	}
	if len(got) != len(expected) {
		t.Fatalf("unexpected retention targets: %#v", got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("target[%d] = %q, want %q; all=%#v", i, got[i], expected[i], got)
		}
	}
}
