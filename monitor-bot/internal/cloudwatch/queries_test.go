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
