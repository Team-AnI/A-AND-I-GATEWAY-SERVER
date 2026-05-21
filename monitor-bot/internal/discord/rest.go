package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
)

type Message struct {
	ID string `json:"id"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
	Roles []string `json:"roles,omitempty"`
}

type channelMessagePayload struct {
	Content         string             `json:"content,omitempty"`
	AllowedMentions *allowedMentions   `json:"allowed_mentions,omitempty"`
	Components      []messageComponent `json:"components,omitempty"`
}

func SendChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, content string) (Message, error) {
	return sendChannelMessage(ctx, client, botToken, channelID, channelMessagePayload{
		Content:         formatting.TruncateDiscordMessage(content),
		AllowedMentions: suppressMentions(),
	})
}

func SendChannelMessageWithComponents(ctx context.Context, client *http.Client, botToken, channelID, content string, components []MessageComponent) (Message, error) {
	return sendChannelMessage(ctx, client, botToken, channelID, channelMessagePayload{
		Content:         formatting.TruncateDiscordMessage(content),
		AllowedMentions: suppressMentions(),
		Components:      components,
	})
}

func SendChannelMessageWithRoleMention(ctx context.Context, client *http.Client, botToken, channelID, content, roleID string) (Message, error) {
	roleID = strings.TrimSpace(roleID)
	if !validDiscordRoleID(roleID) {
		return Message{}, fmt.Errorf("valid discord role id is required")
	}
	return sendChannelMessage(ctx, client, botToken, channelID, channelMessagePayload{
		Content:         formatting.TruncateDiscordMessage("<@&" + roleID + ">\n" + content),
		AllowedMentions: roleMention(roleID),
	})
}

func SendChannelMessageWithRoleMentionAndComponents(ctx context.Context, client *http.Client, botToken, channelID, content, roleID string, components []MessageComponent) (Message, error) {
	roleID = strings.TrimSpace(roleID)
	if !validDiscordRoleID(roleID) {
		return Message{}, fmt.Errorf("valid discord role id is required")
	}
	return sendChannelMessage(ctx, client, botToken, channelID, channelMessagePayload{
		Content:         formatting.TruncateDiscordMessage("<@&" + roleID + ">\n" + content),
		AllowedMentions: roleMention(roleID),
		Components:      components,
	})
}

func sendChannelMessage(ctx context.Context, client *http.Client, botToken, channelID string, payloadData channelMessagePayload) (Message, error) {
	if botToken == "" || channelID == "" {
		return Message{}, fmt.Errorf("discord bot token and channel id are required")
	}
	payload, err := json.Marshal(payloadData)
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
	payload, err := json.Marshal(channelMessagePayload{
		Content:         formatting.TruncateDiscordMessage(content),
		AllowedMentions: suppressMentions(),
	})
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

func suppressMentions() *allowedMentions {
	return &allowedMentions{Parse: []string{}}
}

func roleMention(roleID string) *allowedMentions {
	return &allowedMentions{Parse: []string{}, Roles: []string{roleID}}
}

func validDiscordRoleID(roleID string) bool {
	if roleID == "" || strings.EqualFold(roleID, "everyone") || strings.EqualFold(roleID, "here") {
		return false
	}
	for _, r := range roleID {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
