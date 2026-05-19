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
	if !strings.Contains(query, `service.domain = "report"`) {
		t.Fatalf("report query should filter report domain: %s", query)
	}
	traceQuery, err := BuildTraceQuery("trace_123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(traceQuery, `trace.traceId = "trace_123"`) {
		t.Fatalf("trace id not included after validation: %s", traceQuery)
	}
	if strings.Contains(traceQuery, ` or traceId = "trace_123"`) {
		t.Fatalf("trace query should not use raw traceId fallback: %s", traceQuery)
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
	groups := map[string]string{"gateway": "/a-and-i/gateway", "auth": "/a-and-i/auth", "post": "/a-and-i/prod/tech-blog"}
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
	got, err = LogGroupsForService(groups, "blog")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/a-and-i/prod/tech-blog" {
		t.Fatalf("blog alias should use post log group: %#v", got)
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

func TestBuildErrorsQueryUsesV2ErrorTypes(t *testing.T) {
	query := BuildErrorsQuery("report", 20)
	for _, forbidden := range []string{`level = "WARN"`, `level = "ERROR"`, `http.statusCode >= 400`} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("errors query should not use legacy predicate %s: %s", forbidden, query)
		}
	}
	if !strings.Contains(query, `logType in ["API_ERROR", "EVENT_ERROR"]`) {
		t.Fatalf("errors query should filter V2 error logTypes: %s", query)
	}
	if !strings.Contains(query, `service.domain = "report"`) {
		t.Fatalf("report errors query should filter report domain: %s", query)
	}
}

func TestBuildAuthQueriesUseV2ServiceFields(t *testing.T) {
	errorsQuery := BuildErrorsQuery("auth", 20)
	for _, want := range []string{`logType in ["API_ERROR", "EVENT_ERROR"]`, `service.domain = "auth"`, `service.domainCode = 2`} {
		if !strings.Contains(errorsQuery, want) {
			t.Fatalf("auth errors query missing %q: %s", want, errorsQuery)
		}
	}
	if strings.Contains(errorsQuery, "request.body") || strings.Contains(errorsQuery, "response.data") || strings.Contains(errorsQuery, "@message") {
		t.Fatalf("auth errors query selected raw sensitive fields: %s", errorsQuery)
	}
	securityQuery, err := BuildSecurityQuery("auth", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`logType = "SECURITY"`, `service.domain = "auth"`, `service.domainCode = 2`} {
		if !strings.Contains(securityQuery, want) {
			t.Fatalf("auth security query missing %q: %s", want, securityQuery)
		}
	}
}

func TestServiceDomainFiltersUseV2ServiceFields(t *testing.T) {
	auth := serviceDomainFilter("auth")
	for _, want := range []string{`service.domain = "auth"`, `service.domainCode = 2`, `service.name = "auth-service"`} {
		if !strings.Contains(auth, want) {
			t.Fatalf("auth filter missing %q: %s", want, auth)
		}
	}
	post := serviceDomainFilter("post")
	for _, want := range []string{`service.domain = "blog"`, `service.domainCode = 6`, `service.name = "post-service"`, `service.name = "blog-service"`} {
		if !strings.Contains(post, want) {
			t.Fatalf("post filter missing %q: %s", want, post)
		}
	}
}

func TestBuildAlertQueryIncludesImmediateV2Candidates(t *testing.T) {
	query, err := BuildAlertQuery("auth")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`service.domain = "auth"`, `logType = "EVENT_ERROR"`, `response.error.code like /^[0-9][78][0-9]{3}$/`, `response.error.code like /^21[78][0-9]{2}$/`, `http.statusCode >= 500`} {
		if !strings.Contains(query, want) {
			t.Fatalf("auth alert query missing %q: %s", want, query)
		}
	}
	for _, want := range []string{"60701", "90701", "90801"} {
		if !strings.Contains(query, want) {
			t.Fatalf("alert query missing override code %s: %s", want, query)
		}
	}
	if strings.Contains(query, "request.body") || strings.Contains(query, "response.data") || strings.Contains(query, "@message") {
		t.Fatalf("auth alert query selected raw sensitive fields: %s", query)
	}
}

func TestBuildBlogQueriesUseV2ServiceFields(t *testing.T) {
	recentQuery, err := BuildRecentLogsQuery("blog", "ERROR", 20)
	if err != nil {
		t.Fatal(err)
	}
	slowQuery, err := BuildSlowQuery("post", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	securityQuery, err := BuildSecurityQuery("blog", 20)
	if err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]string{"recent": recentQuery, "slow": slowQuery, "security": securityQuery} {
		for _, want := range []string{`service.domain = "blog"`, `service.domainCode = 6`, `service.name = "post-service"`, `service.name = "blog-service"`} {
			if !strings.Contains(query, want) {
				t.Fatalf("%s blog query missing %q: %s", name, want, query)
			}
		}
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
	if !strings.Contains(query, `service.domain = "report"`) {
		t.Fatalf("report dashboard query should filter report domain: %s", query)
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

func TestBuildAssignmentQueriesUseSafeReportFields(t *testing.T) {
	query := BuildAssignmentsQuery()
	for _, forbidden := range []string{"request.body", "response.data"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("assignments query selected forbidden field %s: %s", forbidden, query)
		}
	}
	if !strings.Contains(query, `service.name = "report-service"`) || !strings.Contains(query, "/assignments") {
		t.Fatalf("assignments query should target report assignment events: %s", query)
	}
	if !strings.Contains(query, "http.method") {
		t.Fatalf("assignments query should keep method for operator context: %s", query)
	}

	detailQuery, err := BuildAssignmentQuery("8f7f8a47-3f5e-4f59-9f2d-a9a9e7b6f111")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detailQuery, `service.domain = "report"`) || !strings.Contains(detailQuery, "8f7f8a47-3f5e-4f59-9f2d-a9a9e7b6f111") {
		t.Fatalf("assignment query should target validated assignment id: %s", detailQuery)
	}
	if strings.Contains(detailQuery, "@message") {
		t.Fatalf("assignment query should not use raw message fallback: %s", detailQuery)
	}
	if _, err := BuildAssignmentQuery("bad/id"); err == nil {
		t.Fatal("unsafe assignment id accepted")
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
