package opslog

import (
	"strings"
	"testing"
)

func TestDecideV2AlertHighForExternalGatewayError(t *testing.T) {
	log := V2OpsLog{
		LogType: "API_ERROR",
		Service: V2Service{Name: "gateway", Domain: "gateway"},
		HTTP:    &V2HTTP{StatusCode: 502},
		Response: &V2Response{Error: &V2Error{
			Code:  17801,
			Value: "DOWNSTREAM_SERVICE_UNAVAILABLE",
		}},
	}
	decision := DecideV2Alert(log)
	if !decision.Alert || decision.Mention || decision.Severity != SeverityHigh || decision.ErrorCode.CategoryCode != 7 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestDecideV2AlertCriticalMentionsOperator(t *testing.T) {
	log := V2OpsLog{
		LogType:  "API_ERROR",
		Service:  V2Service{Name: "gateway", Domain: "gateway"},
		HTTP:     &V2HTTP{StatusCode: 500},
		Response: &V2Response{Error: &V2Error{Code: 18801}},
	}
	decision := DecideV2Alert(log)
	if !decision.Alert || !decision.Mention || decision.Severity != SeverityCrit || decision.ErrorCode.CategoryCode != 8 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestDecideV2AlertCriticalForAuthInternalError(t *testing.T) {
	log := V2OpsLog{
		LogType:  "API_ERROR",
		Service:  V2Service{Name: "auth-service", Domain: "auth", DomainCode: 2},
		HTTP:     &V2HTTP{StatusCode: 500},
		Response: &V2Response{Error: &V2Error{Code: 21801}},
	}
	decision := DecideV2Alert(log)
	if !decision.Alert || !decision.Mention || decision.Severity != SeverityCrit || decision.ErrorCode.CategoryCode != 8 || decision.Domain != "auth" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestDecideV2AlertHighForAuthExternalError(t *testing.T) {
	log := V2OpsLog{
		LogType:  "API_ERROR",
		Service:  V2Service{Name: "auth-service", Domain: "auth", DomainCode: 2},
		HTTP:     &V2HTTP{StatusCode: 400},
		Response: &V2Response{Error: &V2Error{Code: 21701}},
	}
	decision := DecideV2Alert(log)
	if !decision.Alert || decision.Mention || decision.Severity != SeverityHigh || decision.ErrorCode.CategoryCode != 7 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestDecideV2AlertHighForEventError(t *testing.T) {
	log := V2OpsLog{
		LogType: "EVENT_ERROR",
		Service: V2Service{Name: "blog-service", Domain: "blog", DomainCode: 6},
	}
	decision := DecideV2Alert(log)
	if !decision.Alert || decision.Mention || decision.Severity != SeverityHigh || decision.Reason != "EVENT_ERROR" {
		t.Fatalf("unexpected event decision: %#v", decision)
	}
}

func TestDecideV2AlertUsesBlogCommonOverrideCodes(t *testing.T) {
	cases := []struct {
		code     int
		severity string
		mention  bool
		category int
	}{
		{64801, SeverityHigh, false, 4},
		{64805, SeverityHigh, false, 4},
		{68801, SeverityCrit, true, 8},
		{60701, SeverityHigh, false, 7},
		{90701, SeverityHigh, false, 7},
		{90801, SeverityCrit, true, 8},
		{98801, SeverityCrit, true, 8},
	}
	for _, tc := range cases {
		log := V2OpsLog{
			LogType:  "API_ERROR",
			Service:  V2Service{Name: "blog-service", Domain: "blog", DomainCode: 6},
			HTTP:     &V2HTTP{StatusCode: 400},
			Response: &V2Response{Error: &V2Error{Code: tc.code}},
		}
		decision := DecideV2Alert(log)
		if !decision.Alert || decision.Mention != tc.mention || decision.Severity != tc.severity || decision.ErrorCode.CategoryCode != tc.category {
			t.Fatalf("code %d decision = %#v", tc.code, decision)
		}
	}
}

func TestDecideV2AlertUsesExplicitAuthSeverityCodes(t *testing.T) {
	cases := []struct {
		code     int
		severity string
		mention  bool
	}{
		{21701, SeverityHigh, false},
		{21801, SeverityCrit, true},
	}
	for _, tc := range cases {
		log := V2OpsLog{
			LogType:  "API_ERROR",
			Service:  V2Service{Name: "auth-service", Domain: "auth", DomainCode: 2},
			HTTP:     &V2HTTP{StatusCode: 500},
			Response: &V2Response{Error: &V2Error{Code: tc.code}},
		}
		decision := DecideV2Alert(log)
		if !decision.Alert || decision.Mention != tc.mention || decision.Severity != tc.severity {
			t.Fatalf("code %d decision = %#v", tc.code, decision)
		}
	}
}

func TestDecideV2AlertHTTP500WithoutExplicitCriticalIsHighNoMention(t *testing.T) {
	log := V2OpsLog{
		LogType:  "API_ERROR",
		Service:  V2Service{Name: "blog-service", Domain: "blog", DomainCode: 6},
		HTTP:     &V2HTTP{StatusCode: 500},
		Response: &V2Response{Error: &V2Error{Code: 90899}},
	}
	decision := DecideV2Alert(log)
	if !decision.Alert || decision.Mention || decision.Severity != SeverityHigh {
		t.Fatalf("unexpected HTTP 500 decision: %#v", decision)
	}
}

func TestDecideV2AlertAuthClientErrorsAggregateOnly(t *testing.T) {
	for _, log := range []V2OpsLog{
		{
			LogType:  "API_ERROR",
			Service:  V2Service{Name: "auth-service", Domain: "auth"},
			HTTP:     &V2HTTP{StatusCode: 401},
			Response: &V2Response{Error: &V2Error{Code: 21101}},
		},
		{
			LogType:  "API_ERROR",
			Service:  V2Service{Name: "auth-service", Domain: "auth"},
			HTTP:     &V2HTTP{StatusCode: 400},
			Response: &V2Response{Error: &V2Error{Code: 21301}},
		},
		{
			LogType:  "API_ERROR",
			Service:  V2Service{Name: "auth-service", Domain: "auth"},
			HTTP:     &V2HTTP{StatusCode: 403},
			Response: &V2Response{Error: &V2Error{Code: 22101}},
		},
		{
			LogType:  "API_ERROR",
			Service:  V2Service{Name: "auth-service", Domain: "auth"},
			HTTP:     &V2HTTP{StatusCode: 404},
			Response: &V2Response{Error: &V2Error{Code: 35101}},
		},
		{
			LogType:  "API_ERROR",
			Service:  V2Service{Name: "auth-service", Domain: "auth"},
			HTTP:     &V2HTTP{StatusCode: 409},
			Response: &V2Response{Error: &V2Error{Code: 24601}},
		},
	} {
		decision := DecideV2Alert(log)
		if decision.Alert || !decision.AggregateOnly || decision.Severity == SeverityHigh || decision.Severity == SeverityCrit {
			t.Fatalf("client error must aggregate only: %#v", decision)
		}
	}
}

func TestDecideV2AlertLowBusinessErrorAggregatesOnly(t *testing.T) {
	log := V2OpsLog{
		LogType:  "API_ERROR",
		Service:  V2Service{Name: "web-service", Domain: "report"},
		HTTP:     &V2HTTP{StatusCode: 409},
		Response: &V2Response{Error: &V2Error{Code: 44501}},
	}
	decision := DecideV2Alert(log)
	if decision.Alert || !decision.AggregateOnly || decision.Severity != SeverityLow || decision.ErrorCode.CategoryCode != 4 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestResolveServiceDomain(t *testing.T) {
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{Name: "web-service", Domain: "report", DomainCode: 4}}); got != "report" {
		t.Fatalf("domain = %q, want report", got)
	}
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{Name: "gateway", DomainCode: 1}}); got != "gateway" {
		t.Fatalf("domain = %q, want gateway", got)
	}
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{Name: "auth-service"}}); got != "auth" {
		t.Fatalf("domain = %q, want auth", got)
	}
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{Domain: "blog"}}); got != "blog" {
		t.Fatalf("domain = %q, want blog", got)
	}
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{DomainCode: 6}}); got != "blog" {
		t.Fatalf("domain code = %q, want blog", got)
	}
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{Name: "post-service"}}); got != "blog" {
		t.Fatalf("post-service domain = %q, want blog", got)
	}
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{Name: "auth-service"}}); got != "auth" {
		t.Fatalf("auth-service domain = %q, want auth", got)
	}
	if got := ResolveServiceDomain(V2OpsLog{Service: V2Service{DomainCode: 2}}); got != "auth" {
		t.Fatalf("auth domain code = %q, want auth", got)
	}
	if got := ResolveServiceDomainFromFields(map[string]string{"service.domain": "auth"}); got != "auth" {
		t.Fatalf("field domain = %q, want auth", got)
	}
}

func TestParseV2LogUnparsedLogsDoNotAlert(t *testing.T) {
	for _, raw := range []string{
		"plain failed text",
		`{"message":"failed","service":{"name":"gateway"}}`,
	} {
		parsed := ParseV2Log(raw)
		if parsed.Kind != UnparsedText && parsed.Kind != UnparsedJSON {
			t.Fatalf("expected unparsed kind for %q: %#v", raw, parsed)
		}
		if parsed.Log != nil && DecideV2Alert(*parsed.Log).Alert {
			t.Fatalf("unparsed log must not alert: %#v", parsed)
		}
	}
}

func TestMessageFailedDoesNotDriveAlert(t *testing.T) {
	log := V2OpsLog{
		LogType: "API",
		Message: "payment failed",
		Service: V2Service{Name: "gateway", Domain: "gateway"},
		HTTP:    &V2HTTP{StatusCode: 200},
	}
	decision := DecideV2Alert(log)
	if decision.Alert || decision.Severity != SeverityUnk {
		t.Fatalf("message text must not drive alert: %#v", decision)
	}
}

func TestTraceRowUsesTraceField(t *testing.T) {
	log := RowToV2OpsLog(map[string]string{
		"trace.traceId": "trace-123",
		"message":       "trace-123 in message is display-only",
	})
	if log.Trace == nil || log.Trace.TraceID != "trace-123" {
		t.Fatalf("trace field not mapped: %#v", log.Trace)
	}
}

func TestFormatV2AlertSanitizesSecrets(t *testing.T) {
	log := V2OpsLog{
		LogType: "API_ERROR",
		Message: "Authenticate=abc password=secret salt=raw Authorization: Bearer token-value",
		Service: V2Service{Name: "gateway", Domain: "gateway"},
		Trace:   &V2Trace{TraceID: "trace-1"},
		HTTP:    &V2HTTP{Method: "GET", Route: "/api/v2/test", StatusCode: 502, LatencyMs: 241},
		Response: &V2Response{Error: &V2Error{
			Code:    17801,
			Value:   "DOWNSTREAM_SERVICE_UNAVAILABLE",
			Message: "token=secret",
			Alert:   "서버 연결이 불안정합니다.",
		}},
	}
	content := FormatV2Alert(log, DecideV2Alert(log), "<@&123>\n")
	for _, forbidden := range []string{"abc", "password=secret", "salt=raw", "Bearer token-value", "token=secret"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("alert leaked %q: %s", forbidden, content)
		}
	}
	for _, want := range []string{"API_ERROR | gateway", "Code      17801", "external_system", "/ops logs mode:trace query:trace-1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("alert missing %q: %s", want, content)
		}
	}
}

func TestFormatV2AlertSanitizesAuthSecrets(t *testing.T) {
	log := V2OpsLog{
		LogType: "API_ERROR",
		Message: "Authenticate=abc password=secret Authorization: Bearer token-value",
		Service: V2Service{Name: "auth-service", Domain: "auth"},
		Trace:   &V2Trace{TraceID: "trace-auth"},
		HTTP:    &V2HTTP{Method: "POST", Route: "/api/v2/auth/login", StatusCode: 500},
		Response: &V2Response{Error: &V2Error{
			Code:    21801,
			Message: "token=secret",
		}},
	}
	content := FormatV2Alert(log, DecideV2Alert(log), "<@&123>\n")
	for _, forbidden := range []string{"abc", "password=secret", "Bearer token-value", "token=secret"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("auth alert leaked %q: %s", forbidden, content)
		}
	}
	for _, want := range []string{"API_ERROR | auth", "Code      21801", "internal_error", "/ops logs service:auth mode:errors"} {
		if !strings.Contains(content, want) {
			t.Fatalf("auth alert missing %q: %s", want, content)
		}
	}
}
