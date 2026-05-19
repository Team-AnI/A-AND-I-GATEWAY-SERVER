package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
)

const (
	InteractionResponsePong                   = 1
	InteractionResponseChannelMessage         = 4
	InteractionResponseDeferredChannelMessage = 5
	MessageFlagEphemeral                      = 1 << 6
)

type interactionResponse struct {
	Type int                  `json:"type"`
	Data *interactionCallback `json:"data,omitempty"`
}

type interactionCallback struct {
	Content string `json:"content,omitempty"`
	Flags   int    `json:"flags,omitempty"`
}

func pongResponse() interactionResponse {
	return interactionResponse{Type: InteractionResponsePong}
}

func messageResponse(content string, ephemeral bool) interactionResponse {
	flags := 0
	if ephemeral {
		flags = MessageFlagEphemeral
	}
	return interactionResponse{
		Type: InteractionResponseChannelMessage,
		Data: &interactionCallback{Content: formatting.TruncateDiscordMessage(content), Flags: flags},
	}
}

func deferredResponse(ephemeral bool) interactionResponse {
	flags := 0
	if ephemeral {
		flags = MessageFlagEphemeral
	}
	return interactionResponse{
		Type: InteractionResponseDeferredChannelMessage,
		Data: &interactionCallback{Flags: flags},
	}
}

func SendFollowUp(ctx context.Context, client *http.Client, applicationID, token, content string, ephemeral bool) error {
	if applicationID == "" || token == "" {
		return fmt.Errorf("discord application id and interaction token are required")
	}
	flags := 0
	if ephemeral {
		flags = MessageFlagEphemeral
	}
	payload, err := json.Marshal(interactionCallback{Content: formatting.TruncateDiscordMessage(content), Flags: flags})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://discord.com/api/v10/webhooks/%s/%s", applicationID, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord follow-up failed: HTTP %d", resp.StatusCode)
	}
	return nil
}
