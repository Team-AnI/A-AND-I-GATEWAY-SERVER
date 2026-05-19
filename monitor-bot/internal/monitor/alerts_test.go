package monitor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/opslog"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func TestAlertFingerprintDedupeAndCooldown(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "report", "service.name": "web-service", "logType": "API_ERROR", "http.statusCode": "502", "http.route": "/v2/report", "response.error.code": "17801", "trace.traceId": "trace-1"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[2:3]
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	if fakeDiscord.sends != 1 {
		t.Fatalf("expected one V2 alert within cooldown, got %d", fakeDiscord.sends)
	}
}

func TestServiceAlertsSkipUnconnectedServices(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"service.domain": "report", "service.name": "web-service", "logType": "API_ERROR", "http.statusCode": "500", "response.error.code": "18801"},
	}})

	alerts := service.collectAlerts(context.Background())
	for _, alert := range alerts {
		if alert.Service != "report" {
			t.Fatalf("unconnected service should not create alert: %#v", alert)
		}
	}
}

func TestServiceAlertsCollectAuthV2Alert(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "auth", "service.name": "auth-service", "logType": "API_ERROR", "http.statusCode": "500", "http.route": "/api/v2/auth/login", "response.error.code": "21801", "trace.traceId": "trace-auth"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[1:2]

	alerts := service.collectAlerts(context.Background())
	if len(alerts) != 1 {
		t.Fatalf("expected one auth alert, got %#v", alerts)
	}
	alert := alerts[0]
	if alert.Service != "auth" || alert.Severity != "CRITICAL" || alert.V2Decision.ErrorCode.CategoryCode != 8 {
		t.Fatalf("unexpected auth alert: %#v", alert)
	}
}

func TestServiceAlertMentionsOperatorRoleAndUsesOpsCommands(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "report", "service.name": "web-service", "logType": "API_ERROR", "http.statusCode": "500", "http.route": "/v2/report", "response.error.code": "18801", "trace.traceId": "trace-1"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[2:3]
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends == 0 {
		t.Fatal("expected service alert message")
	}
	content := fakeDiscord.sentContents[0]
	for _, want := range []string{"<@&1234567890>", "API_ERROR | report", "Code      18801", "/ops logs service:report mode:errors", "/ops trace trace_id:trace-1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("alert missing %q: %s", want, content)
		}
	}
}

func TestHighServiceAlertDoesNotMentionOperatorRole(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "blog", "service.name": "blog-service", "logType": "API_ERROR", "http.statusCode": "502", "http.route": "/v2/blogs", "response.error.code": "60701", "trace.traceId": "trace-blog"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[4:5]
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends != 1 {
		t.Fatalf("expected one alert, got %d", fakeDiscord.sends)
	}
	content := fakeDiscord.sentContents[0]
	if strings.Contains(content, "<@&1234567890>") {
		t.Fatalf("HIGH alert must not mention role: %s", content)
	}
	if !strings.Contains(content, "API_ERROR | blog") || !strings.Contains(content, "external_system") {
		t.Fatalf("unexpected HIGH alert content: %s", content)
	}
}

func TestCriticalAlertUsesStateRoleBeforeAllowedRole(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *state.Data) {
		data.ServiceAlerts.RoleID = "9999999999"
	}); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "auth", "service.name": "auth-service", "logType": "API_ERROR", "http.statusCode": "500", "http.route": "/v2/auth/login", "response.error.code": "28101", "trace.traceId": "trace-auth"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[1:2]
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	content := fakeDiscord.sentContents[0]
	if !strings.Contains(content, "<@&9999999999>") || strings.Contains(content, "<@&1234567890>") {
		t.Fatalf("critical alert should use state role first: %s", content)
	}
}

func TestAllowedRoleFallbackOnlyAppliesToCritical(t *testing.T) {
	critical := Alert{
		AlertType: "v2-log",
		V2Log: &opslog.V2OpsLog{
			LogType:  "API_ERROR",
			Service:  opslog.V2Service{Name: "auth-service", Domain: "auth"},
			HTTP:     &opslog.V2HTTP{StatusCode: 500},
			Response: &opslog.V2Response{Error: &opslog.V2Error{Code: 28101}},
		},
		V2Decision: opslog.AlertDecision{Alert: true, Mention: true, Severity: opslog.SeverityCrit, Domain: "auth"},
	}
	high := critical
	high.V2Log = &opslog.V2OpsLog{
		LogType:  "API_ERROR",
		Service:  opslog.V2Service{Name: "blog-service", Domain: "blog"},
		HTTP:     &opslog.V2HTTP{StatusCode: 502},
		Response: &opslog.V2Response{Error: &opslog.V2Error{Code: 60701}},
	}
	high.V2Decision = opslog.AlertDecision{Alert: true, Mention: false, Severity: opslog.SeverityHigh, Domain: "blog"}

	if content := formatAlert(critical, "<@&1234567890>\n"); !strings.Contains(content, "<@&1234567890>") {
		t.Fatalf("critical alert should include fallback mention: %s", content)
	}
	if content := formatAlert(high, "<@&1234567890>\n"); strings.Contains(content, "<@&1234567890>") {
		t.Fatalf("HIGH alert should not include fallback mention: %s", content)
	}
}

func TestConfigureAlertUsesStateRoleAndBlocksUnsafeRole(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})

	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "role", "everyone"); err == nil {
		t.Fatal("@everyone-like role should be rejected")
	}
	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "channel", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "role", "1234567890"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "on", ""); err != nil {
		t.Fatal(err)
	}
	status := service.FormatAlertStatus()
	for _, want := range []string{"enabled: true", "<#channel-1>", "<@&1234567890>"} {
		if !strings.Contains(status, want) {
			t.Fatalf("alert status missing %q: %s", want, status)
		}
	}
	if got := service.alertRoleMention(); !strings.Contains(got, "<@&1234567890>") {
		t.Fatalf("state role should be used for mention: %q", got)
	}
}

func TestAlertTestSendsWithoutConfiguredRole(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.DiscordAllowedRoleIDs = nil
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "test", ""); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends != 1 {
		t.Fatalf("expected one test alert, got %d", fakeDiscord.sends)
	}
	if strings.Contains(fakeDiscord.sentContents[0], "@everyone") || strings.Contains(fakeDiscord.sentContents[0], "@here") {
		t.Fatalf("unsafe mention leaked: %s", fakeDiscord.sentContents[0])
	}
}

func TestResolvedAlertStateTransition(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "report", "service.name": "web-service", "logType": "API_ERROR", "http.statusCode": "500", "http.path": "/v2/report", "response.error.code": "18801"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[2:3]
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.logs = &fakeLogs{}
	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	resolved := false
	for _, alert := range snapshot.Alerts {
		if !alert.Active && !alert.ResolvedAt.IsZero() && time.Since(alert.ResolvedAt) < time.Minute {
			resolved = true
		}
	}
	if !resolved {
		t.Fatalf("expected resolved alert state, got %#v", snapshot.Alerts)
	}
}
