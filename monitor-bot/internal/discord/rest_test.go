package discord

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureTransport struct {
	calls int
	body  []byte
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	t.body = body
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"id":"message-1"}`)),
	}, nil
}

func TestSendChannelMessageSuppressesMentionsByDefault(t *testing.T) {
	transport := &captureTransport{}
	client := &http.Client{Transport: transport}

	if _, err := SendChannelMessage(context.Background(), client, "bot-token", "channel-1", "hello @everyone @here <@&999999>"); err != nil {
		t.Fatal(err)
	}

	payload := decodeMessagePayload(t, transport.body)
	if payload.AllowedMentions == nil {
		t.Fatal("allowed_mentions must be set")
	}
	if len(payload.AllowedMentions.Parse) != 0 {
		t.Fatalf("default parse must be empty, got %#v", payload.AllowedMentions.Parse)
	}
	if len(payload.AllowedMentions.Roles) != 0 {
		t.Fatalf("default role mentions must be empty, got %#v", payload.AllowedMentions.Roles)
	}
	assertEveryoneHereDisabled(t, payload.AllowedMentions)
}

func TestSendChannelMessageWithRoleMentionAllowsOnlyConfiguredRole(t *testing.T) {
	transport := &captureTransport{}
	client := &http.Client{Transport: transport}

	if _, err := SendChannelMessageWithRoleMention(context.Background(), client, "bot-token", "channel-1", "critical <@&999999> @everyone", "1234567890"); err != nil {
		t.Fatal(err)
	}

	payload := decodeMessagePayload(t, transport.body)
	if !strings.HasPrefix(payload.Content, "<@&1234567890>\n") {
		t.Fatalf("critical message must prefix configured role mention: %s", payload.Content)
	}
	if payload.AllowedMentions == nil {
		t.Fatal("allowed_mentions must be set")
	}
	if len(payload.AllowedMentions.Parse) != 0 {
		t.Fatalf("role mention parse must be empty, got %#v", payload.AllowedMentions.Parse)
	}
	if got := strings.Join(payload.AllowedMentions.Roles, ","); got != "1234567890" {
		t.Fatalf("allowed role mentions = %#v", payload.AllowedMentions.Roles)
	}
	assertEveryoneHereDisabled(t, payload.AllowedMentions)
}

func TestSendChannelMessageWithComponentsMarshalsLegacyButtons(t *testing.T) {
	transport := &captureTransport{}
	client := &http.Client{Transport: transport}

	components := []MessageComponent{ActionRow(
		PrimaryButton("Trace 상세", "ops:v1:trace:trace-123"),
		SecondaryButton("gateway 오류 30m", "ops:v1:logs:gateway:errors:30m:10"),
	)}
	if _, err := SendChannelMessageWithComponents(context.Background(), client, "bot-token", "channel-1", "content", components); err != nil {
		t.Fatal(err)
	}

	payload := decodeMessagePayload(t, transport.body)
	if payload.Content != "content" {
		t.Fatalf("content mismatch: %s", payload.Content)
	}
	if payload.AllowedMentions == nil || len(payload.AllowedMentions.Parse) != 0 || len(payload.AllowedMentions.Roles) != 0 {
		t.Fatalf("component message must suppress mentions: %#v", payload.AllowedMentions)
	}
	if len(payload.Components) != 1 || payload.Components[0].Type != componentTypeActionRow {
		t.Fatalf("expected action row component: %#v", payload.Components)
	}
	buttons := payload.Components[0].Components
	if len(buttons) != 2 {
		t.Fatalf("expected two buttons: %#v", buttons)
	}
	if buttons[0].Type != componentTypeButton || buttons[0].Style != buttonStylePrimary || buttons[0].Label != "Trace 상세" || buttons[0].CustomID != "ops:v1:trace:trace-123" {
		t.Fatalf("trace button mismatch: %#v", buttons[0])
	}
	if buttons[1].Type != componentTypeButton || buttons[1].Style != buttonStyleSecondary || buttons[1].Label != "gateway 오류 30m" || buttons[1].CustomID != "ops:v1:logs:gateway:errors:30m:10" {
		t.Fatalf("service button mismatch: %#v", buttons[1])
	}
}

func TestSendChannelMessageWithRoleMentionAndComponentsAllowsOnlyConfiguredRole(t *testing.T) {
	transport := &captureTransport{}
	client := &http.Client{Transport: transport}

	components := []MessageComponent{ActionRow(PrimaryButton("Trace 상세", "ops:v1:trace:trace-123"))}
	if _, err := SendChannelMessageWithRoleMentionAndComponents(context.Background(), client, "bot-token", "channel-1", "critical @everyone", "1234567890", components); err != nil {
		t.Fatal(err)
	}

	payload := decodeMessagePayload(t, transport.body)
	if !strings.HasPrefix(payload.Content, "<@&1234567890>\n") {
		t.Fatalf("critical message must prefix configured role mention: %s", payload.Content)
	}
	if payload.AllowedMentions == nil || len(payload.AllowedMentions.Parse) != 0 || strings.Join(payload.AllowedMentions.Roles, ",") != "1234567890" {
		t.Fatalf("role mention policy mismatch: %#v", payload.AllowedMentions)
	}
	if len(payload.Components) != 1 || len(payload.Components[0].Components) != 1 {
		t.Fatalf("components missing: %#v", payload.Components)
	}
	assertEveryoneHereDisabled(t, payload.AllowedMentions)
}

func TestSendChannelMessageWithRoleMentionRejectsInvalidRoleID(t *testing.T) {
	for _, roleID := range []string{"", "abc", "123x", "everyone", "here", "@everyone", "@here", "<@&1234567890>"} {
		transport := &captureTransport{}
		client := &http.Client{Transport: transport}
		if _, err := SendChannelMessageWithRoleMention(context.Background(), client, "bot-token", "channel-1", "critical", roleID); err == nil {
			t.Fatalf("expected invalid role id %q to be rejected", roleID)
		}
		if transport.calls != 0 {
			t.Fatalf("invalid role id %q should not send request", roleID)
		}
	}
}

func decodeMessagePayload(t *testing.T, body []byte) channelMessagePayload {
	t.Helper()
	var payload channelMessagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, body)
	}
	return payload
}

func assertEveryoneHereDisabled(t *testing.T, mentions *allowedMentions) {
	t.Helper()
	for _, value := range mentions.Parse {
		if value == "everyone" || value == "here" {
			t.Fatalf("@everyone/@here must never be enabled: %#v", mentions.Parse)
		}
	}
}
