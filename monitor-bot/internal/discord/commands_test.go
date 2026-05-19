package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
)

func TestDefinitionsPlaceRequiredOptionsBeforeOptionalOptions(t *testing.T) {
	for _, command := range Definitions() {
		assertRequiredOptionsBeforeOptional(t, command.Name, command.Options)
	}
}

func TestCommandDefinitionsAreDiscordCompatible(t *testing.T) {
	commandNamePattern := regexp.MustCompile(`^[a-z0-9_-]+$`)
	optionNamePattern := regexp.MustCompile(`^[a-z0-9_-]+$`)
	for _, command := range Definitions() {
		if strings.TrimSpace(command.Name) == "" {
			t.Fatal("command name must not be empty")
		}
		if !commandNamePattern.MatchString(command.Name) {
			t.Fatalf("command %q must be lowercase and Discord-compatible", command.Name)
		}
		for _, option := range command.Options {
			assertOptionNamesAreCompatible(t, command.Name, option, optionNamePattern)
		}
	}
}

func TestOpsCommandSubcommandsRegistered(t *testing.T) {
	command, err := findCommand("ops")
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, option := range command.Options {
		if option.Type != 1 {
			t.Fatalf("/ops option %q must be a subcommand, got type=%d", option.Name, option.Type)
		}
		got[option.Name] = true
	}
	for _, want := range []string{"dashboard", "service", "logs", "watch", "unwatch", "watches", "alert", "logs-watch", "logs-unwatch", "logs-watches", "assignments", "assignments-all", "assignment", "assignment-check", "submissions", "trace", "alarms", "storage", "help"} {
		if !got[want] {
			t.Fatalf("/ops subcommand %q is not registered", want)
		}
	}
	if got["copy"] {
		t.Fatal("copy subcommand should not be registered")
	}
}

func TestOpsConnectedServiceChoices(t *testing.T) {
	command, err := findCommand("ops")
	if err != nil {
		t.Fatal(err)
	}
	dashboard := findSubcommand(t, command, "dashboard")
	if got := choiceValues(findOption(t, dashboard.Options, "since").Choices); strings.Join(got, ",") != "15m,30m,1h" {
		t.Fatalf("dashboard since choices = %#v", got)
	}
	logs := findSubcommand(t, command, "logs")
	if findOption(t, logs.Options, "service").Required {
		t.Fatal("logs service option should be optional and default to all")
	}
	if got := choiceNames(findOption(t, logs.Options, "service").Choices); strings.Join(got, ",") != "all,gateway,auth,report,blog" {
		t.Fatalf("logs service choice names = %#v", got)
	}
	if got := choiceValues(findOption(t, logs.Options, "service").Choices); strings.Join(got, ",") != "all,gateway,auth,report,post" {
		t.Fatalf("logs service choice values = %#v", got)
	}
	service := findSubcommand(t, command, "service")
	if got := choiceNames(findOption(t, service.Options, "service").Choices); strings.Join(got, ",") != "gateway,auth,report,blog" {
		t.Fatalf("service choice names = %#v", got)
	}
	if got := choiceValues(findOption(t, service.Options, "service").Choices); strings.Join(got, ",") != "gateway,auth,report,post" {
		t.Fatalf("service choice values = %#v", got)
	}
	assignments := findSubcommand(t, command, "assignments")
	if course := findOption(t, assignments.Options, "course"); !course.Required {
		t.Fatal("assignments course option must be required")
	}
	if got := choiceValues(findOption(t, assignments.Options, "status").Choices); strings.Join(got, ",") != "all,published,draft,scheduled" {
		t.Fatalf("assignments status choices = %#v", got)
	}
	assignmentsAll := findSubcommand(t, command, "assignments-all")
	if got := choiceValues(findOption(t, assignmentsAll.Options, "window").Choices); strings.Join(got, ",") != "today,this-week" {
		t.Fatalf("assignments-all window choices = %#v", got)
	}
	assignment := findSubcommand(t, command, "assignment")
	if course := findOption(t, assignment.Options, "course"); !course.Required {
		t.Fatal("assignment course option must be required")
	}
	idOption := findOption(t, assignment.Options, "id")
	if !idOption.Required {
		t.Fatal("assignment id option must be required")
	}
	assignmentCheck := findSubcommand(t, command, "assignment-check")
	if !findOption(t, assignmentCheck.Options, "course").Required || !findOption(t, assignmentCheck.Options, "id").Required {
		t.Fatal("assignment-check course/id options must be required")
	}
	submissions := findSubcommand(t, command, "submissions")
	if !findOption(t, submissions.Options, "course").Required || !findOption(t, submissions.Options, "assignment").Required {
		t.Fatal("submissions course/assignment options must be required")
	}
}

func TestOpsDashboardHasNoViewOption(t *testing.T) {
	command, err := findCommand("ops")
	if err != nil {
		t.Fatal(err)
	}
	dashboardCommand := findSubcommand(t, command, "dashboard")
	if _, ok := findOptionByName(dashboardCommand.Options, "view"); ok {
		t.Fatal("/ops dashboard should not register a view option")
	}
}

func TestOpsLogsLimitChoices(t *testing.T) {
	command, err := findCommand("ops")
	if err != nil {
		t.Fatal(err)
	}
	logsCommand := findSubcommand(t, command, "logs")
	limitOption := findOption(t, logsCommand.Options, "limit")
	got := choiceValues(limitOption.Choices)
	want := []string{"5", "10", "20"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("/ops logs limit choices = %#v, want %#v", got, want)
	}
}

func TestDefinitionsOnlyRegisterOpsCommand(t *testing.T) {
	commands := Definitions()
	if len(commands) != 1 || commands[0].Name != "ops" {
		t.Fatalf("expected only /ops to be registered: %#v", commands)
	}
}

func TestOpsServiceViewsStayStateFocused(t *testing.T) {
	command, err := findCommand("ops")
	if err != nil {
		t.Fatal(err)
	}
	serviceCommand := findSubcommand(t, command, "service")
	viewOption := findOption(t, serviceCommand.Options, "view")
	got := choiceValues(viewOption.Choices)
	want := []string{"summary", "health"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("/ops service view choices = %#v, want %#v", got, want)
	}
}

func TestOpsLogsModesStayLogFocused(t *testing.T) {
	command, err := findCommand("ops")
	if err != nil {
		t.Fatal(err)
	}
	logsCommand := findSubcommand(t, command, "logs")
	modeOption := findOption(t, logsCommand.Options, "mode")
	got := choiceValues(modeOption.Choices)
	want := []string{"recent", "errors", "slow", "security"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("/ops logs mode choices = %#v, want %#v", got, want)
	}
}

func TestServiceOpsAutomationCommandsRegistered(t *testing.T) {
	command, err := findCommand("ops")
	if err != nil {
		t.Fatal(err)
	}
	watch := findSubcommand(t, command, "watch")
	if !findOption(t, watch.Options, "scope").Required {
		t.Fatal("watch scope must be required")
	}
	if got := choiceValues(findOption(t, watch.Options, "interval").Choices); strings.Join(got, ",") != "1m,3m,5m,10m,15m" {
		t.Fatalf("watch interval choices = %#v", got)
	}
	if channel := findOption(t, watch.Options, "channel"); channel.Type != 7 || channel.Required {
		t.Fatalf("watch channel option must be optional channel type, got type=%d required=%t", channel.Type, channel.Required)
	}
	if got := choiceNames(findOption(t, watch.Options, "service").Choices); strings.Join(got, ",") != "gateway,auth,report,blog" {
		t.Fatalf("watch service choices = %#v", got)
	}
	alert := findSubcommand(t, command, "alert")
	if !findOption(t, alert.Options, "action").Required {
		t.Fatal("alert action must be required")
	}
	if channel := findOption(t, alert.Options, "channel"); channel.Type != 7 || channel.Required {
		t.Fatalf("alert channel option must be optional channel type, got type=%d required=%t", channel.Type, channel.Required)
	}
	if role := findOption(t, alert.Options, "role"); role.Type != 8 {
		t.Fatalf("alert role option must be role type, got %d", role.Type)
	}
	logsWatch := findSubcommand(t, command, "logs-watch")
	if !findOption(t, logsWatch.Options, "service").Required || !findOption(t, logsWatch.Options, "mode").Required {
		t.Fatal("logs-watch service/mode options must be required")
	}
	if got := choiceNames(findOption(t, logsWatch.Options, "service").Choices); strings.Join(got, ",") != "gateway,auth,report,blog" {
		t.Fatalf("logs-watch service choices = %#v", got)
	}
	if channel := findOption(t, logsWatch.Options, "channel"); channel.Type != 7 || channel.Required {
		t.Fatalf("logs-watch channel option must be optional channel type, got type=%d required=%t", channel.Type, channel.Required)
	}
	logsUnwatch := findSubcommand(t, command, "logs-unwatch")
	if got := choiceNames(findOption(t, logsUnwatch.Options, "service").Choices); strings.Join(got, ",") != "gateway,auth,report,blog" {
		t.Fatalf("logs-unwatch service choices = %#v", got)
	}
	alarms := findSubcommand(t, command, "alarms")
	if got := choiceNames(findOption(t, alarms.Options, "service").Choices); strings.Join(got, ",") != "gateway,auth,report,blog,online-judge" {
		t.Fatalf("alarms service choices = %#v", got)
	}
}

func TestDefinitionsPayloadMarshals(t *testing.T) {
	payload, err := json.Marshal(Definitions())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"name":"blog","value":"post"`) {
		t.Fatalf("blog display/post value choice missing from payload: %s", payload)
	}
}

func TestAllServiceQueryGuard(t *testing.T) {
	if !isAllServiceQuery("") || !isAllServiceQuery("all") {
		t.Fatal("blank or all service should be treated as all-service query")
	}
	if !sinceAllowsAllQuery("30m") {
		t.Fatal("30m should be allowed for service=all")
	}
	if sinceAllowsAllQuery("1h") || sinceAllowsAllQuery("3h") {
		t.Fatal("service=all should be capped at 30m")
	}
	if !strings.Contains(allServiceGuardMessage(), "errors/dashboard") {
		t.Fatalf("all-service guard should explain allowed modes: %s", allServiceGuardMessage())
	}
}

func TestParseOpsInterval(t *testing.T) {
	cases := map[string]time.Duration{
		"":    5 * time.Minute,
		"1m":  time.Minute,
		"3m":  3 * time.Minute,
		"5m":  5 * time.Minute,
		"10m": 10 * time.Minute,
		"15m": 15 * time.Minute,
	}
	for input, want := range cases {
		got, ok := parseOpsInterval(input, 5*time.Minute)
		if !ok || got != want {
			t.Fatalf("parseOpsInterval(%q) = %s %t, want %s true", input, got, ok, want)
		}
	}
	if _, ok := parseOpsInterval("30s", 5*time.Minute); ok {
		t.Fatal("30s should not be accepted")
	}
}

func TestAppendTraceNextOnlyWhenRowsContainTraceID(t *testing.T) {
	withoutTrace := appendTraceNext("content", []map[string]string{{"message": "no trace"}})
	if strings.Contains(withoutTrace, "/ops trace") {
		t.Fatalf("trace next should be hidden without trace id: %s", withoutTrace)
	}
	withTrace := appendTraceNext("content", []map[string]string{{"trace.traceId": "abc-123"}})
	if !strings.Contains(withTrace, "/ops trace trace_id:abc-123") {
		t.Fatalf("trace next should be shown when trace exists: %s", withTrace)
	}
}

func TestParseOpsLimit(t *testing.T) {
	cases := map[string]int{
		"5":  5,
		"10": 10,
		"20": 20,
		"":   20,
		"7":  20,
	}
	for input, want := range cases {
		if got := parseOpsLimit(input, 20); got != want {
			t.Fatalf("parseOpsLimit(%q) = %d, want %d", input, got, want)
		}
	}
	if got := parseOpsLimit("", 10); got != 10 {
		t.Fatalf("parseOpsLimit should keep safe fallback choice, got %d", got)
	}
}

func TestFilterAssignmentsByWindow(t *testing.T) {
	assignments := []reportadmin.Assignment{
		{ID: "today", StartAt: time.Now().Format(time.RFC3339)},
		{ID: "future", StartAt: time.Now().Add(48 * time.Hour).Format(time.RFC3339)},
	}
	today := filterAssignmentsByWindow(assignments, "today")
	if len(today) != 1 || today[0].ID != "today" {
		t.Fatalf("today window mismatch: %#v", today)
	}
	week := filterAssignmentsByWindow(assignments, "this-week")
	if len(week) != 2 {
		t.Fatalf("this-week window mismatch: %#v", week)
	}
}

func TestClassifyManualCourseNoticeStates(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	if got := classifyManualCourse(reportadmin.Course{Status: "ARCHIVED"}, now); got != "LEGACY" {
		t.Fatalf("archived manual course = %s", got)
	}
	if got := classifyManualCourse(reportadmin.Course{}, now); got != "UNKNOWN" {
		t.Fatalf("empty manual course = %s", got)
	}
	if got := classifyManualCourse(reportadmin.Course{Status: "OPEN"}, now); got != "ACTIVE" {
		t.Fatalf("open manual course = %s", got)
	}
}

func TestRegisterGuildCommandsReturns400BodyWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bot test-bot-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Invalid Form Body","token":"secret-token","errors":{"0":{"name":{"_errors":[{"message":"bad command"}]}}}}`))
	}))
	defer server.Close()

	err := registerGuildCommands(context.Background(), server.Client(), "test-bot-token", "app-id", "guild-id", server.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	message := err.Error()
	if !strings.Contains(message, "status=400") || !strings.Contains(message, "Invalid Form Body") || !strings.Contains(message, "bad command") {
		t.Fatalf("400 response body was not included: %s", message)
	}
	if strings.Contains(message, "test-bot-token") || strings.Contains(message, "secret-token") {
		t.Fatalf("sensitive token leaked in error: %s", message)
	}
}

func assertRequiredOptionsBeforeOptional(t *testing.T, commandPath string, options []commandOption) {
	t.Helper()
	seenOptional := false
	for _, option := range options {
		if option.Type == 1 {
			assertRequiredOptionsBeforeOptional(t, commandPath+" "+option.Name, option.Options)
			continue
		}
		if !option.Required {
			seenOptional = true
			continue
		}
		if seenOptional {
			t.Fatalf("command %q has required option %q after an optional option", commandPath, option.Name)
		}
	}
}

func assertOptionNamesAreCompatible(t *testing.T, commandPath string, option commandOption, pattern *regexp.Regexp) {
	t.Helper()
	if strings.TrimSpace(option.Name) == "" {
		t.Fatalf("command %q has an empty option name", commandPath)
	}
	if !pattern.MatchString(option.Name) {
		t.Fatalf("command %q option %q must be lowercase and Discord-compatible", commandPath, option.Name)
	}
	for _, child := range option.Options {
		assertOptionNamesAreCompatible(t, commandPath+" "+option.Name, child, pattern)
	}
}

func TestRegisterGuildCommands429DoesNotRetryImmediately(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited","retry_after":2}`))
	}))
	defer server.Close()

	err := registerGuildCommands(context.Background(), server.Client(), "bot-token", "app-id", "guild-id", server.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("registration should not retry during startup, calls=%d", got)
	}
	var registrationErr *RegistrationError
	if !errors.As(err, &registrationErr) {
		t.Fatalf("expected RegistrationError: %T", err)
	}
	if registrationErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status: %d", registrationErr.StatusCode)
	}
	if registrationErr.RetryAfter != 2*time.Second {
		t.Fatalf("unexpected retry_after: %s", registrationErr.RetryAfter)
	}
}

func findCommand(name string) (commandDefinition, error) {
	for _, command := range Definitions() {
		if command.Name == name {
			return command, nil
		}
	}
	return commandDefinition{}, fmt.Errorf("command %q not found", name)
}

func findSubcommand(t *testing.T, command commandDefinition, name string) commandOption {
	t.Helper()
	for _, option := range command.Options {
		if option.Type == 1 && option.Name == name {
			return option
		}
	}
	t.Fatalf("subcommand %q not found in %q", name, command.Name)
	return commandOption{}
}

func findOption(t *testing.T, options []commandOption, name string) commandOption {
	t.Helper()
	option, ok := findOptionByName(options, name)
	if ok {
		return option
	}
	t.Fatalf("option %q not found", name)
	return commandOption{}
}

func findOptionByName(options []commandOption, name string) (commandOption, bool) {
	for _, option := range options {
		if option.Name == name {
			return option, true
		}
	}
	return commandOption{}, false
}

func choiceValues(choices []commandChoice) []string {
	values := make([]string, 0, len(choices))
	for _, choice := range choices {
		values = append(values, fmt.Sprint(choice.Value))
	}
	return values
}

func choiceNames(choices []commandChoice) []string {
	values := make([]string, 0, len(choices))
	for _, choice := range choices {
		values = append(values, choice.Name)
	}
	return values
}
