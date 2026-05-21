package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRegisterCommandsFailureDoesNotFatalWhenStrictDisabled(t *testing.T) {
	cfg := config.Config{
		DiscordRegisterCommands: true,
		DiscordCommandScope:     "guild",
		DiscordBotToken:         "bot-token",
		DiscordApplicationID:    "app-id",
		DiscordAllowedGuildID:   "guild-id",
		StrictStartupChecks:     false,
	}
	status := newRuntimeStatus(cfg)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       ioNopCloser(`{"message":"Invalid Form Body","token":"secret-token"}`),
		}, nil
	})}

	if err := registerCommandsIfEnabled(context.Background(), cfg, client, status); err != nil {
		t.Fatalf("registration failure should not stop startup when strict checks are disabled: %v", err)
	}
	snapshot := status.Snapshot()
	if snapshot.DiscordCommandsRegistered {
		t.Fatal("commands should not be marked registered")
	}
	if !strings.Contains(snapshot.DiscordCommandRegistrationError, "status=400") {
		t.Fatalf("registration error was not stored: %#v", snapshot)
	}
	if strings.Contains(snapshot.DiscordCommandRegistrationError, "secret-token") || strings.Contains(snapshot.DiscordCommandRegistrationError, "bot-token") {
		t.Fatalf("registration error leaked token: %s", snapshot.DiscordCommandRegistrationError)
	}
}

func TestHealthStatusIncludesRegistrationState(t *testing.T) {
	cfg := config.Config{
		DiscordPublicKey: "public-key",
		Dashboard:        config.DashboardConfig{Enabled: true},
		Alert:            config.AlertConfig{Enabled: true},
	}
	status := newRuntimeStatus(cfg)
	status.SetHTTPServer(true)
	status.SetAWSSDKConfigured(true)
	status.SetDiscordRegistration(false, assertErr("status=400 body=bad payload"))

	body, err := json.Marshal(status.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	payload := string(body)
	for _, want := range []string{
		`"ok":true`,
		`"httpServer":true`,
		`"awsSdkConfigured":true`,
		`"discordPublicKeyProvided":true`,
		`"discordCommandsRegistered":false`,
		`"discordCommandRegistrationError":"status=400 body=bad payload"`,
		`"dashboardEnabled":true`,
		`"alertEnabled":true`,
	} {
		if !strings.Contains(payload, want) {
			t.Fatalf("health payload missing %s: %s", want, payload)
		}
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

type stringReadCloser struct {
	*strings.Reader
}

func (s stringReadCloser) Close() error { return nil }

func ioNopCloser(value string) stringReadCloser {
	return stringReadCloser{Reader: strings.NewReader(value)}
}
