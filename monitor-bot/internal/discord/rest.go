package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
)

type Message struct {
	ID string `json:"id"`
}

func SendChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, content string) (Message, error) {
	if botToken == "" || channelID == "" {
		return Message{}, fmt.Errorf("discord bot token and channel id are required")
	}
	payload, err := json.Marshal(interactionCallback{Content: formatting.TruncateDiscordMessage(content)})
	if err != nil {
		return Message{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID), bytes.NewReader(payload))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Message{}, fmt.Errorf("discord send message failed: HTTP %d", resp.StatusCode)
	}
	var msg Message
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func EditChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, messageID, content string) error {
	if botToken == "" || channelID == "" || messageID == "" {
		return fmt.Errorf("discord bot token, channel id, and message id are required")
	}
	payload, err := json.Marshal(interactionCallback{Content: formatting.TruncateDiscordMessage(content)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages/%s", channelID, messageID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord edit message failed: HTTP %d", resp.StatusCode)
	}
	return nil
}
