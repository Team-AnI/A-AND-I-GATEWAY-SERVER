package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type commandDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Options     []commandOption `json:"options,omitempty"`
}

type commandOption struct {
	Type        int             `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Required    bool            `json:"required,omitempty"`
	Choices     []commandChoice `json:"choices,omitempty"`
}

type commandChoice struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func Definitions() []commandDefinition {
	serviceChoices := choices("gateway", "auth", "report", "online-judge", "post")
	sinceChoices := choices("5m", "15m", "30m", "1h", "3h")
	levelChoices := choices("INFO", "WARN", "ERROR")
	return []commandDefinition{
		{Name: "status", Description: "A&I 서비스 상태 요약"},
		{Name: "health", Description: "특정 서비스 health 조회", Options: []commandOption{
			stringOption("service", "조회할 서비스", true, serviceChoices),
		}},
		{Name: "logs", Description: "CloudWatch 최근 로그 조회", Options: []commandOption{
			stringOption("service", "조회할 서비스", true, serviceChoices),
			stringOption("since", "조회 기간", true, sinceChoices),
			stringOption("level", "로그 레벨", true, levelChoices),
		}},
		{Name: "errors", Description: "CloudWatch 에러 집계", Options: []commandOption{
			stringOption("service", "조회할 서비스", false, serviceChoices),
			stringOption("since", "조회 기간", true, sinceChoices),
		}},
		{Name: "trace", Description: "traceId 기준 로그 조회", Options: []commandOption{
			stringOption("trace_id", "조회할 traceId", true, nil),
		}},
		{Name: "alarm", Description: "CloudWatch ALARM 상태 조회"},
		{Name: "help", Description: "명령어 도움말"},
	}
}

func RegisterGuildCommands(ctx context.Context, client *http.Client, botToken, applicationID, guildID string) error {
	if botToken == "" {
		return fmt.Errorf("DISCORD_BOT_TOKEN is required to register commands")
	}
	if applicationID == "" || guildID == "" {
		return fmt.Errorf("DISCORD_APPLICATION_ID and DISCORD_ALLOWED_GUILD_ID are required for guild command registration")
	}
	payload, err := json.Marshal(Definitions())
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://discord.com/api/v10/applications/%s/guilds/%s/commands", applicationID, guildID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
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
		return fmt.Errorf("discord command registration failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func stringOption(name, description string, required bool, choices []commandChoice) commandOption {
	return commandOption{Type: 3, Name: name, Description: description, Required: required, Choices: choices}
}

func choices(values ...string) []commandChoice {
	result := make([]commandChoice, 0, len(values))
	for _, value := range values {
		result = append(result, commandChoice{Name: value, Value: value})
	}
	return result
}
