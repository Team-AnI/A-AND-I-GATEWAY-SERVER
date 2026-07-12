package discord

import (
	"context"
	"strings"
	"testing"
)

func TestOpsLogsQueryModesPreserveRoutingAndNextCommands(t *testing.T) {
	h := newComponentTestHandler(resultRows([]map[string]string{{
		"@timestamp":             "2026-07-13T01:00:00+09:00",
		"count":                  "3",
		"level":                  "ERROR",
		"logType":                "API_ERROR",
		"service.name":           "gateway",
		"service.domain":         "gateway",
		"http.route":             "/v2/test",
		"http.statusCode":        "500",
		"http.latencyMs":         "1200",
		"trace.traceId":          "trace-123",
		"response.error.code":    "18801",
		"response.error.value":   "INTERNAL_ERROR",
		"response.error.message": "internal",
		"message":                "request failed",
	}}))

	cases := []struct {
		name     string
		options  []ApplicationCommandOpt
		contains string
	}{
		{"recent", logsQueryOptions("recent", "gateway"), "/ops logs service:gateway mode:errors"},
		{"errors", logsQueryOptions("errors", "gateway"), "/ops dashboard since:30m"},
		{"critical", logsQueryOptions("critical", "gateway"), "/ops alert action:status"},
		{"top", logsQueryOptions("top", "gateway"), "/ops logs mode:trace query:<traceId>"},
		{"slow", logsQueryOptions("slow", "gateway"), "/ops logs mode:trace query:<traceId>"},
		{"security", logsQueryOptions("security", "gateway"), "/ops logs mode:trace query:<traceId>"},
		{"trace", append(logsQueryOptions("trace", "gateway"), stringInteractionOption("query", "trace-123")), "trace-123"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := h.opsLogsCommand(context.Background(), Interaction{}, ApplicationCommandOpt{Options: tc.options})
			if !strings.Contains(got, tc.contains) {
				t.Fatalf("response does not contain %q: %s", tc.contains, got)
			}
			if strings.Contains(got, "조회 실패") || strings.Contains(got, "지원하지 않는") {
				t.Fatalf("query mode failed: %s", got)
			}
		})
	}
}

func TestOpsLogsQueryPreservesGuardsAndTraceInference(t *testing.T) {
	h := newComponentTestHandler(nil)
	cases := []struct {
		name    string
		options []ApplicationCommandOpt
		want    string
	}{
		{"invalid action", []ApplicationCommandOpt{stringInteractionOption("action", "delete")}, "지원하지 않는 logs action입니다. view, watch, unwatch, watches 중 하나를 사용하세요."},
		{"invalid service", logsQueryOptions("errors", "unknown"), "지원하지 않는 service입니다."},
		{"non v2 service", logsQueryOptions("errors", "online-judge"), "status: NO_V2_LOG"},
		{"all recent guard", logsQueryOptions("recent", "all"), allServiceGuardMessage()},
		{"all top guard", logsQueryOptions("top", "all"), allServiceGuardMessage()},
		{"events service guard", logsQueryOptions("events", "auth"), "mode:events는 현재 report assignment audit EVENT 로그만 지원합니다."},
		{"trace query required", logsQueryOptions("trace", "gateway"), "mode:trace에는 query:<traceId>가 필요합니다."},
		{"invalid mode", logsQueryOptions("unknown", "gateway"), "지원하지 않는 logs mode입니다."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := h.opsLogsCommand(context.Background(), Interaction{}, ApplicationCommandOpt{Options: tc.options})
			if !strings.Contains(got, tc.want) {
				t.Fatalf("unexpected guard response:\ngot:  %s\nwant: %s", got, tc.want)
			}
		})
	}

	traceHandler := newComponentTestHandler(resultRows([]map[string]string{{"trace.traceId": "trace-123"}}))
	got := traceHandler.opsLogsCommand(context.Background(), Interaction{}, ApplicationCommandOpt{Options: []ApplicationCommandOpt{
		stringInteractionOption("service", "gateway"),
		stringInteractionOption("query", "trace-123"),
	}})
	if !strings.Contains(got, "trace-123") {
		t.Fatalf("trace inference changed: %s", got)
	}
}

func logsQueryOptions(mode, service string) []ApplicationCommandOpt {
	return []ApplicationCommandOpt{
		stringInteractionOption("action", "view"),
		stringInteractionOption("mode", mode),
		stringInteractionOption("service", service),
		stringInteractionOption("since", "30m"),
		stringInteractionOption("limit", "10"),
	}
}
