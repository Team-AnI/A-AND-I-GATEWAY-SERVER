package monitor

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/discord"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/opslog"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

type selectiveAlertLogs struct {
	calls    []string
	failures map[string]error
}

func (f *selectiveAlertLogs) Query(_ context.Context, logGroups []string, _ string, _ time.Duration, _ int32) ([]map[string]string, error) {
	logGroup := ""
	if len(logGroups) > 0 {
		logGroup = logGroups[0]
	}
	f.calls = append(f.calls, logGroup)
	if err := f.failures[logGroup]; err != nil {
		return nil, err
	}
	return nil, nil
}

func TestPollAlertsDoesNotResolveServiceWhenCloudWatchQueryFails(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	gatewayAlert := makeV2Alert(map[string]string{
		"service.domain": "gateway", "service.name": "gateway", "logType": "API_ERROR", "http.statusCode": "500", "response.error.code": "18801", "trace.traceId": "trace-gateway",
	})
	authAlert := makeV2Alert(map[string]string{
		"service.domain": "auth", "service.name": "auth-service", "logType": "API_ERROR", "http.statusCode": "500", "response.error.code": "21801", "trace.traceId": "trace-auth",
	})
	setActiveAlerts(t, store, gatewayAlert, authAlert)

	service := newTestService(store, &fakeLogs{})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[:2]
	logs := &selectiveAlertLogs{failures: map[string]error{"/auth": errors.New("cloudwatch unavailable")}}
	service.logs = logs
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	if snapshot.Alerts[alertCooldownKey(gatewayAlert)].Active {
		t.Fatal("successfully queried gateway alert should be resolved")
	}
	if !snapshot.Alerts[alertCooldownKey(authAlert)].Active {
		t.Fatal("failed auth query must not resolve the existing auth alert")
	}
	if fakeDiscord.sends != 1 {
		t.Fatalf("only the observed gateway alert should emit a resolution, got %d sends", fakeDiscord.sends)
	}
}

func TestPollAlertsDoesNotResolveServiceSkippedByQueryBudget(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	gatewayAlert := makeV2Alert(map[string]string{
		"service.domain": "gateway", "service.name": "gateway", "logType": "API_ERROR", "http.statusCode": "500", "response.error.code": "18801", "trace.traceId": "trace-gateway",
	})
	authAlert := makeV2Alert(map[string]string{
		"service.domain": "auth", "service.name": "auth-service", "logType": "API_ERROR", "http.statusCode": "500", "response.error.code": "21801", "trace.traceId": "trace-auth",
	})
	setActiveAlerts(t, store, gatewayAlert, authAlert)

	service := newTestService(store, &fakeLogs{})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[:2]
	service.cfg.Dashboard.MaxCloudWatchQueries = 1
	logs := &selectiveAlertLogs{}
	service.logs = logs
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	if snapshot.Alerts[alertCooldownKey(gatewayAlert)].Active {
		t.Fatal("successfully queried gateway alert should be resolved")
	}
	if !snapshot.Alerts[alertCooldownKey(authAlert)].Active {
		t.Fatal("auth alert skipped by query budget must remain active")
	}
	if len(logs.calls) != 1 || logs.calls[0] != "/gateway" {
		t.Fatalf("expected only the gateway query within budget, got %#v", logs.calls)
	}
	if fakeDiscord.sends != 1 {
		t.Fatalf("only the observed gateway alert should emit a resolution, got %d sends", fakeDiscord.sends)
	}
}

func setActiveAlerts(t *testing.T, store *state.Store, alerts ...Alert) {
	t.Helper()
	if err := store.Update(func(data *state.Data) {
		for _, alert := range alerts {
			data.Alerts[alertCooldownKey(alert)] = state.AlertState{Active: true, LastSentAt: time.Now().Add(-time.Hour)}
		}
	}); err != nil {
		t.Fatal(err)
	}
}

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

func TestV2AlertKeepsTraceFingerprintWhileIncidentKeyExcludesTrace(t *testing.T) {
	row1 := map[string]string{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "gateway", "service.name": "gateway", "logType": "API_ERROR", "http.statusCode": "500", "http.route": "/v2/reports", "response.error.code": "18801", "trace.traceId": "trace-1"}
	row2 := map[string]string{"@timestamp": "2026-05-14T10:00:01+09:00", "service.domain": "gateway", "service.name": "gateway", "logType": "API_ERROR", "http.statusCode": "500", "http.route": "/v2/lectures", "response.error.code": "18801", "trace.traceId": "trace-2"}
	alert1 := makeV2Alert(row1)
	alert2 := makeV2Alert(row2)
	if alert1.Fingerprint == "" || alert2.Fingerprint == "" || alert1.Fingerprint == alert2.Fingerprint {
		t.Fatalf("trace-scoped alert fingerprint behavior should remain unchanged: %q %q", alert1.Fingerprint, alert2.Fingerprint)
	}
	if alertCooldownKey(alert1) != alertCooldownKey(alert2) {
		t.Fatalf("send cooldown key should group same incident with different trace/path: %q %q", alertCooldownKey(alert1), alertCooldownKey(alert2))
	}
	if serviceAlertIncidentKey(alert1) != serviceAlertIncidentKey(alert2) {
		t.Fatalf("incident key should group same incident with different trace IDs: %q %q", serviceAlertIncidentKey(alert1), serviceAlertIncidentKey(alert2))
	}
}

func TestV2AlertCooldownKeyKeepsDistinctIncidentsSeparate(t *testing.T) {
	base := map[string]string{"service.domain": "gateway", "service.name": "gateway", "logType": "API_ERROR", "http.statusCode": "500", "http.route": "/v2/test", "response.error.code": "18801", "trace.traceId": "trace-1"}
	gateway18801 := makeV2Alert(base)
	auth18801 := makeV2Alert(withAlertRow(base, "service.domain", "auth", "service.name", "auth-service", "trace.traceId", "trace-2"))
	gateway17801 := makeV2Alert(withAlertRow(base, "response.error.code", "17801", "trace.traceId", "trace-3"))
	gateway60701 := makeV2Alert(withAlertRow(base, "response.error.code", "60701", "trace.traceId", "trace-4"))
	gatewayEvent18801 := makeV2Alert(withAlertRow(base, "logType", "EVENT_ERROR", "trace.traceId", "trace-5"))

	cases := []Alert{auth18801, gateway17801, gateway60701, gatewayEvent18801}
	for _, other := range cases {
		if alertCooldownKey(gateway18801) == alertCooldownKey(other) {
			t.Fatalf("cooldown key should keep distinct incident separate: base=%#v other=%#v key=%q", gateway18801, other, alertCooldownKey(other))
		}
	}
}

func TestRepeatedGatewayCriticalAlertsDedupesByCooldownKey(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-14T10:00:00+09:00", "service.domain": "gateway", "service.name": "gateway", "logType": "API_ERROR", "http.statusCode": "401", "http.route": "/v2/lectures", "response.error.code": "18801", "trace.traceId": "trace-a"},
		{"@timestamp": "2026-05-14T10:00:01+09:00", "service.domain": "gateway", "service.name": "gateway", "logType": "API_ERROR", "http.statusCode": "401", "http.route": "/v2/blogs", "response.error.code": "18801", "trace.traceId": "trace-b"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[:1]
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	if fakeDiscord.roleSends != 1 {
		t.Fatalf("same gateway 18801 incident should role-mention once, got %d", fakeDiscord.roleSends)
	}
	if got := len(store.Snapshot().ServiceAlerts.LastSent); got != 1 {
		t.Fatalf("same incident should have one cooldown key, got %d", got)
	}
	recent := store.Snapshot().RecentServiceAlerts
	if len(recent) != 2 {
		t.Fatalf("unique duplicate evidence should be recorded once per fingerprint, got %#v", recent)
	}
	lines := recentServiceAlertLines(recent, 5, "30m", 30*time.Minute, time.Now())
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "×2") || !strings.Contains(joined, "trace-a") || !strings.Contains(joined, "trace-b") {
		t.Fatalf("recent dashboard should group duplicate evidence with traces: %s", joined)
	}
}

func TestLegacyV2FingerprintStateDoesNotSendResolvedNoise(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *state.Data) {
		data.Alerts["prod:gateway:v2:API_ERROR:trace-old"] = state.AlertState{Active: true, LastSentAt: time.Now().Add(-time.Hour)}
	}); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	if fakeDiscord.sends != 0 || fakeDiscord.roleSends != 0 {
		t.Fatalf("legacy V2 trace fingerprint should be resolved silently, sends=%d roleSends=%d", fakeDiscord.sends, fakeDiscord.roleSends)
	}
	if store.Snapshot().Alerts["prod:gateway:v2:API_ERROR:trace-old"].Active {
		t.Fatal("legacy V2 trace fingerprint should be marked inactive")
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

func TestMarkAlertSentPersistsRecentIncidentEvidence(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	alert := Alert{
		Fingerprint: "prod:gateway:v2:API_ERROR:trace-1",
		Service:     "gateway",
		Severity:    "CRITICAL",
		AlertType:   "v2-log",
		Reason:      "critical",
		Path:        "/v2/reports",
		ErrorCode:   "18801",
		Traces:      []string{"trace-1", "trace-1", "bad trace with space"},
	}
	if err := service.markAlertSent(alert, true); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot().RecentServiceAlerts
	if len(got) != 1 {
		t.Fatalf("expected one recent service alert, got %#v", got)
	}
	event := got[0]
	if event.IncidentKey == "" || strings.Contains(event.IncidentKey, "trace-1") {
		t.Fatalf("incident key should exist and exclude trace id: %#v", event)
	}
	if event.Reason != "critical" || event.Path != "/v2/reports" || event.ErrorCode != "18801" {
		t.Fatalf("incident metadata not persisted: %#v", event)
	}
	if len(event.TraceIDs) != 1 || event.TraceIDs[0] != "trace-1" {
		t.Fatalf("trace ids should be sanitized and deduped: %#v", event.TraceIDs)
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
	if fakeDiscord.roleSends != 1 || fakeDiscord.sends != 0 {
		t.Fatalf("expected one role alert message, sends=%d roleSends=%d", fakeDiscord.sends, fakeDiscord.roleSends)
	}
	if got := strings.Join(fakeDiscord.roleIDs, ","); got != "1234567890" {
		t.Fatalf("critical alert should use fallback role id, got %q", got)
	}
	buttons := alertButtons(t, fakeDiscord.roleComponents[0])
	if len(buttons) != 2 {
		t.Fatalf("expected trace and service buttons: %#v", buttons)
	}
	if buttons[0].Label != "Trace 상세" || buttons[0].CustomID != "ops:v1:trace:trace-1" {
		t.Fatalf("trace button mismatch: %#v", buttons[0])
	}
	if buttons[1].Label != "report 오류 30m" || buttons[1].CustomID != "ops:v1:logs:report:errors:30m:10" {
		t.Fatalf("service button mismatch: %#v", buttons[1])
	}
	content := fakeDiscord.roleContents[0]
	for _, want := range []string{"<@&1234567890>", "API_ERROR | report", "Code      18801", "버튼으로 상세 로그를 확인하세요.", "fallback:", "/ops logs service:report mode:errors", "/ops logs mode:trace query:trace-1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("alert missing %q: %s", want, content)
		}
	}
}

func TestV2AlertWithoutTraceOmitsTraceButtonAndEmptyFallback(t *testing.T) {
	alert := alertForErrorCode(18801)
	alert.V2Log.Trace = nil

	components := alertDrilldownComponents(alert)
	buttons := alertButtons(t, components)
	if len(buttons) != 1 {
		t.Fatalf("expected only service button: %#v", buttons)
	}
	if buttons[0].Label != "gateway 오류 30m" || buttons[0].CustomID != "ops:v1:logs:gateway:errors:30m:10" {
		t.Fatalf("service button mismatch: %#v", buttons[0])
	}
	content := formatAlert(alert, "")
	if !strings.Contains(content, "버튼") || !strings.Contains(content, "fallback:") {
		t.Fatalf("alert should keep button-first fallback text: %s", content)
	}
	if strings.Contains(content, "/ops logs mode:trace query:") {
		t.Fatalf("alert must not print empty trace command: %s", content)
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
	if fakeDiscord.roleSends != 0 {
		t.Fatalf("HIGH alert must not use role mention send path, got %d", fakeDiscord.roleSends)
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
	if fakeDiscord.roleSends != 1 || fakeDiscord.sends != 0 {
		t.Fatalf("critical alert should use role send path, sends=%d roleSends=%d", fakeDiscord.sends, fakeDiscord.roleSends)
	}
	content := fakeDiscord.roleContents[0]
	if !strings.Contains(content, "<@&9999999999>") || strings.Contains(content, "<@&1234567890>") {
		t.Fatalf("critical alert should use state role first: %s", content)
	}
	if got := strings.Join(fakeDiscord.roleIDs, ","); got != "9999999999" {
		t.Fatalf("critical alert should send only state role id, got %q", got)
	}
}

func TestExplicitCriticalCodesUseConfiguredStateRoleMention(t *testing.T) {
	for _, code := range []int{18801, 21801, 68801, 98801} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
			if err := store.Load(); err != nil {
				t.Fatal(err)
			}
			if err := store.Update(func(data *state.Data) {
				data.ServiceAlerts.RoleID = "9999999999"
			}); err != nil {
				t.Fatal(err)
			}
			service := newTestService(store, &fakeLogs{})
			fakeDiscord := &fakeDiscord{}
			service.discord = fakeDiscord

			if err := service.sendAlert(context.Background(), alertForErrorCode(code)); err != nil {
				t.Fatal(err)
			}
			if fakeDiscord.roleSends != 1 || fakeDiscord.sends != 0 {
				t.Fatalf("CRITICAL should use role send path, sends=%d roleSends=%d", fakeDiscord.sends, fakeDiscord.roleSends)
			}
			if got := strings.Join(fakeDiscord.roleIDs, ","); got != "9999999999" {
				t.Fatalf("CRITICAL should allow only configured role id, got %q", got)
			}
			if !strings.Contains(fakeDiscord.roleContents[0], "<@&9999999999>") {
				t.Fatalf("CRITICAL should include configured role mention: %s", fakeDiscord.roleContents[0])
			}
		})
	}
}

func TestHighCodesSendAlertWithoutRoleMention(t *testing.T) {
	for _, code := range []int{60701, 90701, 64801, 64805} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
			if err := store.Load(); err != nil {
				t.Fatal(err)
			}
			if err := store.Update(func(data *state.Data) {
				data.ServiceAlerts.RoleID = "9999999999"
			}); err != nil {
				t.Fatal(err)
			}
			service := newTestService(store, &fakeLogs{})
			service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
			fakeDiscord := &fakeDiscord{}
			service.discord = fakeDiscord

			if err := service.sendAlert(context.Background(), alertForErrorCode(code)); err != nil {
				t.Fatal(err)
			}
			if fakeDiscord.sends != 1 || fakeDiscord.roleSends != 0 {
				t.Fatalf("HIGH should use non-mention send path, sends=%d roleSends=%d", fakeDiscord.sends, fakeDiscord.roleSends)
			}
			if strings.Contains(fakeDiscord.sentContents[0], "<@&") {
				t.Fatalf("HIGH alert must not mention role: %s", fakeDiscord.sentContents[0])
			}
		})
	}
}

func TestAllowedRoleFallbackOnlyAppliesToCritical(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	fd := &fakeDiscord{}
	service.discord = fd

	if err := service.sendAlert(context.Background(), alertForErrorCode(21801)); err != nil {
		t.Fatal(err)
	}
	if fd.roleSends != 1 || strings.Join(fd.roleIDs, ",") != "1234567890" {
		t.Fatalf("CRITICAL should use fallback role, sends=%d roleIDs=%#v", fd.roleSends, fd.roleIDs)
	}

	fd = &fakeDiscord{}
	service.discord = fd
	if err := service.sendAlert(context.Background(), alertForErrorCode(60701)); err != nil {
		t.Fatal(err)
	}
	if fd.sends != 1 || fd.roleSends != 0 {
		t.Fatalf("HIGH should not use fallback role, sends=%d roleSends=%d", fd.sends, fd.roleSends)
	}
	if strings.Contains(fd.sentContents[0], "<@&1234567890>") {
		t.Fatalf("HIGH alert should not include fallback mention: %s", fd.sentContents[0])
	}
}

func TestConfigureAlertTargetsSetRouteChannels(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.Alert.ChannelID = ""
	service.cfg.Dashboard.ChannelID = ""

	if _, err := service.ConfigureAlert(context.Background(), "all-channel", "channel", "", "all"); err != nil {
		t.Fatal(err)
	}
	snapshot := store.Snapshot()
	if snapshot.ServiceAlerts.ChannelID != "all-channel" || snapshot.ServiceAlerts.GeneralChannelID != "all-channel" || snapshot.ServiceAlerts.CriticalChannelID != "all-channel" {
		t.Fatalf("target=all should set legacy/general/critical channels: %#v", snapshot.ServiceAlerts)
	}
	if got := service.generalAlertChannelID(); got != "all-channel" {
		t.Fatalf("general all fallback = %q", got)
	}
	if got := service.criticalAlertChannelID(); got != "all-channel" {
		t.Fatalf("critical all fallback = %q", got)
	}

	if _, err := service.ConfigureAlert(context.Background(), "general-channel", "channel", "", "general"); err != nil {
		t.Fatal(err)
	}
	if got := service.generalAlertChannelID(); got != "general-channel" {
		t.Fatalf("general target should update general channel, got %q", got)
	}
	if got := service.criticalAlertChannelID(); got != "all-channel" {
		t.Fatalf("general target should not update critical channel, got %q", got)
	}

	if _, err := service.ConfigureAlert(context.Background(), "critical-channel", "channel", "", "critical"); err != nil {
		t.Fatal(err)
	}
	if got := service.criticalAlertChannelID(); got != "critical-channel" {
		t.Fatalf("critical target should update critical channel, got %q", got)
	}
	if got := service.generalAlertChannelID(); got != "general-channel" {
		t.Fatalf("critical target should not update general channel, got %q", got)
	}
}

func TestLegacyAlertChannelRoutesGeneralAndCritical(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *state.Data) {
		data.ServiceAlerts.ChannelID = "legacy-channel"
	}); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.Alert.ChannelID = ""
	service.cfg.Dashboard.ChannelID = ""

	if got := service.generalAlertChannelID(); got != "legacy-channel" {
		t.Fatalf("legacy channel should route general alerts, got %q", got)
	}
	if got := service.criticalAlertChannelID(); got != "legacy-channel" {
		t.Fatalf("legacy channel should route critical alerts, got %q", got)
	}
}

func TestAlertRoutingUsesGeneralAndCriticalChannels(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *state.Data) {
		data.ServiceAlerts.GeneralChannelID = "general-channel"
		data.ServiceAlerts.CriticalChannelID = "critical-channel"
		data.ServiceAlerts.RoleID = "9999999999"
	}); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.Alert.ChannelID = ""
	service.cfg.Dashboard.ChannelID = ""
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.sendAlert(context.Background(), alertForErrorCode(60701)); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends != 1 || fakeDiscord.sentChannels[0] != "general-channel" || fakeDiscord.roleSends != 0 {
		t.Fatalf("HIGH should use general channel without role mention, sends=%d roleSends=%d channels=%#v", fakeDiscord.sends, fakeDiscord.roleSends, fakeDiscord.sentChannels)
	}

	if err := service.sendAlert(context.Background(), alertForErrorCode(21801)); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.roleSends != 1 || fakeDiscord.roleChannels[0] != "critical-channel" {
		t.Fatalf("CRITICAL should use critical channel with role mention, roleSends=%d channels=%#v", fakeDiscord.roleSends, fakeDiscord.roleChannels)
	}
	if got := strings.Join(fakeDiscord.roleIDs, ","); got != "9999999999" {
		t.Fatalf("critical route should use configured state role, got %q", got)
	}
}

func TestAlertTestTargetsDoNotMentionRole(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *state.Data) {
		data.ServiceAlerts.GeneralChannelID = "general-channel"
		data.ServiceAlerts.CriticalChannelID = "critical-channel"
		data.ServiceAlerts.RoleID = "9999999999"
	}); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.Alert.ChannelID = ""
	service.cfg.Dashboard.ChannelID = ""
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if _, err := service.ConfigureAlert(context.Background(), "", "test", "", "general"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureAlert(context.Background(), "", "test", "", "critical"); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.roleSends != 0 {
		t.Fatalf("alert tests must not mention role, roleSends=%d", fakeDiscord.roleSends)
	}
	if strings.Join(fakeDiscord.sentChannels, ",") != "general-channel,critical-channel" {
		t.Fatalf("test target routing channels = %#v", fakeDiscord.sentChannels)
	}
}

func TestConfigureAlertUsesStateRoleAndBlocksUnsafeRole(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})

	for _, roleID := range []string{"everyone", "here", "@everyone", "@here"} {
		if _, err := service.ConfigureAlert(context.Background(), "channel-1", "role", roleID, ""); err == nil {
			t.Fatalf("%q role should be rejected", roleID)
		}
	}
	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "channel", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "role", "1234567890", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "on", "", ""); err != nil {
		t.Fatal(err)
	}
	status := service.FormatAlertStatus()
	for _, want := range []string{"enabled: true", "<#channel-1>", "<@&1234567890>"} {
		if !strings.Contains(status, want) {
			t.Fatalf("alert status missing %q: %s", want, status)
		}
	}
	if got := service.alertMentionRoleID(); got != "1234567890" {
		t.Fatalf("state role should be used for mention: %q", got)
	}
}

func TestAlertTestSendsWithoutConfiguredRole(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	if err := store.Update(func(data *state.Data) {
		data.ServiceAlerts.RoleID = "9999999999"
	}); err != nil {
		t.Fatal(err)
	}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if _, err := service.ConfigureAlert(context.Background(), "channel-1", "test", "", ""); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends != 1 {
		t.Fatalf("expected one test alert, got %d", fakeDiscord.sends)
	}
	if fakeDiscord.roleSends != 0 {
		t.Fatalf("test alert must not use role mention send path, got %d", fakeDiscord.roleSends)
	}
	if strings.Contains(fakeDiscord.sentContents[0], "<@&") || strings.Contains(fakeDiscord.sentContents[0], "@everyone") || strings.Contains(fakeDiscord.sentContents[0], "@here") {
		t.Fatalf("unsafe mention leaked: %s", fakeDiscord.sentContents[0])
	}
}

func TestCriticalWithoutValidRoleSendsWithoutMention(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.DiscordAllowedRoleIDs = []string{"@everyone"}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.sendAlert(context.Background(), alertForErrorCode(21801)); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends != 1 || fakeDiscord.roleSends != 0 {
		t.Fatalf("invalid fallback role should send without mention, sends=%d roleSends=%d", fakeDiscord.sends, fakeDiscord.roleSends)
	}
	if strings.Contains(fakeDiscord.sentContents[0], "<@&") {
		t.Fatalf("invalid fallback role should not mention: %s", fakeDiscord.sentContents[0])
	}
}

func alertForErrorCode(code int) Alert {
	domain, name := serviceForErrorCode(code)
	log := opslog.V2OpsLog{
		Timestamp: "2026-05-19T10:00:00+09:00",
		Level:     "ERROR",
		LogType:   "API_ERROR",
		Service:   opslog.V2Service{Name: name, Domain: domain},
		HTTP:      &opslog.V2HTTP{StatusCode: 500, Route: "/test"},
		Response:  &opslog.V2Response{Error: &opslog.V2Error{Code: code}},
		Trace:     &opslog.V2Trace{TraceID: "trace-" + strconv.Itoa(code)},
	}
	decision := opslog.DecideV2Alert(log)
	return Alert{
		Fingerprint: "test-" + strconv.Itoa(code),
		Service:     decision.Domain,
		AlertType:   "v2-log",
		Severity:    decision.Severity,
		Reason:      decision.Reason,
		Path:        "/test",
		ErrorCode:   strconv.Itoa(code),
		V2Log:       &log,
		V2Decision:  decision,
	}
}

func serviceForErrorCode(code int) (domain string, name string) {
	switch code / 10000 {
	case 2:
		return "auth", "auth-service"
	case 6:
		return "blog", "blog-service"
	case 9:
		return "common", "common-service"
	default:
		return "gateway", "gateway"
	}
}

func withAlertRow(row map[string]string, pairs ...string) map[string]string {
	cloned := make(map[string]string, len(row)+len(pairs)/2)
	for key, value := range row {
		cloned[key] = value
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		cloned[pairs[i]] = pairs[i+1]
	}
	return cloned
}

func alertButtons(t *testing.T, components []discord.MessageComponent) []discord.MessageComponent {
	t.Helper()
	if len(components) == 0 {
		return nil
	}
	if len(components) != 1 || len(components[0].Components) == 0 {
		t.Fatalf("expected one action row with buttons: %#v", components)
	}
	return components[0].Components
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
