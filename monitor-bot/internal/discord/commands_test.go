package discord

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefinitionsPlaceRequiredOptionsBeforeOptionalOptions(t *testing.T) {
	for _, command := range Definitions() {
		seenOptional := false
		for _, option := range command.Options {
			if !option.Required {
				seenOptional = true
				continue
			}
			if seenOptional {
				t.Fatalf("command %q has required option %q after an optional option", command.Name, option.Name)
			}
		}
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
