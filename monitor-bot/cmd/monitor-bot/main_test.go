package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
)

var validDiscordPublicKey = strings.Repeat("00", ed25519.PublicKeySize)

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
		DiscordApplicationID: "app-id",
		DiscordPublicKey:     validDiscordPublicKey,
		Dashboard:            config.DashboardConfig{Enabled: true},
		Alert:                config.AlertConfig{Enabled: true},
	}
	status := newRuntimeStatus(cfg)
	status.SetHTTPServer(true)
	status.SetAWSSDKConfigured(true)
	status.SetInteractionReady(true)
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
		`"interactionReady":true`,
		`"discordApplicationIdProvided":true`,
		`"discordPublicKeyProvided":true`,
		`"discordPublicKeyValid":true`,
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

func TestRuntimeStatusRequiresInteractionHandlerForReadiness(t *testing.T) {
	status := newRuntimeStatus(config.Config{DiscordApplicationID: "app-id", DiscordPublicKey: validDiscordPublicKey})
	if status.Snapshot().OK {
		t.Fatal("status should not be ready before startup begins")
	}
	status.SetHTTPServer(true)
	status.SetAWSSDKConfigured(true)

	beforeHandler := status.Snapshot()
	if beforeHandler.OK {
		t.Fatal("status should not be ready before the interaction handler is installed")
	}
	if beforeHandler.InteractionReady {
		t.Fatal("interaction handler should not be marked ready before installation")
	}

	status.SetInteractionReady(true)
	afterHandler := status.Snapshot()
	if !afterHandler.OK {
		t.Fatal("status should be ready after the HTTP server, AWS SDK, and interaction handler are ready")
	}
	if !afterHandler.InteractionReady {
		t.Fatal("interaction handler should be marked ready after installation")
	}
}

func TestRuntimeStatusRequiresAWSForReadiness(t *testing.T) {
	status := newRuntimeStatus(config.Config{DiscordApplicationID: "app-id", DiscordPublicKey: validDiscordPublicKey})
	status.SetHTTPServer(true)
	status.SetInteractionReady(true)

	if status.Snapshot().OK {
		t.Fatal("status should not be ready before the AWS SDK is configured")
	}

	status.SetAWSSDKConfigured(true)
	if !status.Snapshot().OK {
		t.Fatal("status should be ready after the HTTP server, AWS SDK, and interaction handler are ready")
	}
}

func TestRuntimeStatusReadinessDoesNotRequireCommandRegistration(t *testing.T) {
	status := newRuntimeStatus(config.Config{DiscordApplicationID: "app-id", DiscordPublicKey: validDiscordPublicKey})
	status.SetHTTPServer(true)
	status.SetAWSSDKConfigured(true)
	status.SetInteractionReady(true)
	status.SetDiscordRegistration(false, assertErr("registration disabled"))

	snapshot := status.Snapshot()
	if !snapshot.OK {
		t.Fatal("command registration state should not affect readiness")
	}
}

func TestRuntimeStatusRequiresDiscordPublicKeyForReadiness(t *testing.T) {
	status := newRuntimeStatus(config.Config{DiscordApplicationID: "app-id"})
	status.SetHTTPServer(true)
	status.SetAWSSDKConfigured(true)
	status.SetInteractionReady(true)

	if status.Snapshot().OK {
		t.Fatal("status should not be ready without a Discord public key")
	}
}

func TestRuntimeStatusRejectsMalformedDiscordPublicKey(t *testing.T) {
	status := newRuntimeStatus(config.Config{DiscordApplicationID: "app-id", DiscordPublicKey: "not-an-ed25519-key"})
	status.SetHTTPServer(true)
	status.SetAWSSDKConfigured(true)
	status.SetInteractionReady(true)

	snapshot := status.Snapshot()
	if snapshot.OK {
		t.Fatal("status should not be ready with a malformed Discord public key")
	}
	if !snapshot.DiscordPublicKeyProvided {
		t.Fatal("status should distinguish a malformed key from a missing key")
	}
	if snapshot.DiscordPublicKeyValid {
		t.Fatal("malformed Discord public key should not be marked valid")
	}
}

func TestRuntimeStatusRequiresDiscordApplicationIDForReadiness(t *testing.T) {
	status := newRuntimeStatus(config.Config{DiscordPublicKey: validDiscordPublicKey})
	status.SetHTTPServer(true)
	status.SetAWSSDKConfigured(true)
	status.SetInteractionReady(true)

	snapshot := status.Snapshot()
	if snapshot.OK {
		t.Fatal("status should not be ready without a Discord application ID")
	}
	if snapshot.DiscordApplicationIDProvided {
		t.Fatal("missing Discord application ID should be reported")
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
