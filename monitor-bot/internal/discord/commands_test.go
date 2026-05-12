package discord

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefinitionsPlaceRequiredOptionsBeforeOptionalOptions(t *testing.T) {
	for _, command := range Definitions() {
		assertRequiredOptionsBeforeOptional(t, command.Name, command.Options)
	}
}

func TestErrorsCommandPlacesSinceBeforeOptionalService(t *testing.T) {
	command, err := findCommand("errors")
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Options) != 2 {
		t.Fatalf("unexpected errors option count: %d", len(command.Options))
	}
	if command.Options[0].Name != "since" || !command.Options[0].Required {
		t.Fatalf("first errors option must be required since: %#v", command.Options[0])
	}
	if command.Options[1].Name != "service" || command.Options[1].Required {
		t.Fatalf("second errors option must be optional service: %#v", command.Options[1])
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
	for _, want := range []string{"dashboard", "service", "logs", "copy", "trace", "alarms", "storage", "help"} {
		if !got[want] {
			t.Fatalf("/ops subcommand %q is not registered", want)
		}
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

func TestDefinitionsCanExcludeLegacyCommands(t *testing.T) {
	commands := DefinitionsWithLegacy(false)
	if len(commands) != 1 || commands[0].Name != "ops" {
		t.Fatalf("expected only /ops when legacy registration is disabled: %#v", commands)
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
	want := []string{"summary", "health", "copy"}
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
	want := []string{"recent", "errors", "top", "slow"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("/ops logs mode choices = %#v, want %#v", got, want)
	}
}

func TestLegacyCommandsDelegateToOpsHandlers(t *testing.T) {
	cases := map[string]string{
		"dashboard":   "/ops dashboard",
		"service":     "/ops service",
		"copy-status": "/ops copy",
		"logs":        "/ops logs mode:recent",
		"errors":      "/ops logs mode:errors",
		"trace":       "/ops trace",
		"alarm":       "/ops alarms",
		"disk":        "/ops storage view:usage",
		"retention":   "/ops storage view:retention",
	}
	for legacy, want := range cases {
		got, ok := legacyOpsReplacement(legacy)
		if !ok || got != want {
			t.Fatalf("legacy command %q replacement mismatch: got %q ok=%v want %q", legacy, got, ok, want)
		}
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

func TestRegisterGuildCommandsReturns400BodyWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bot test-bot-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Invalid Form Body","token":"secret-token","errors":{"0":{"name":{"_errors":[{"message":"bad command"}]}}}}`))
	}))
	defer server.Close()

	err := registerGuildCommands(context.Background(), server.Client(), "test-bot-token", "app-id", "guild-id", server.URL, true)
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

	err := registerGuildCommands(context.Background(), server.Client(), "bot-token", "app-id", "guild-id", server.URL, true)
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
