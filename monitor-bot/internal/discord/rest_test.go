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
