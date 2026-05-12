package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
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

type RegistrationError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *RegistrationError) Error() string {
	if e == nil {
		return ""
	}
	message := fmt.Sprintf("discord command registration failed: status=%d", e.StatusCode)
	if e.RetryAfter > 0 {
		message += fmt.Sprintf(" retry_after=%s", e.RetryAfter)
	}
	if strings.TrimSpace(e.Body) != "" {
		message += " body=" + e.Body
	}
	return message
}

func Definitions() []commandDefinition {
	serviceChoices := choices("gateway", "auth", "report", "online-judge", "post")
	serviceOrAllChoices := choices("all", "gateway", "auth", "report", "online-judge", "post")
	sinceChoices := choices("5m", "15m", "30m", "1h", "3h")
	watchIntervalChoices := choices("5m", "10m", "15m")
	levelChoices := choices("INFO", "WARN", "ERROR")
	countTypeChoices := choices("all", "api", "error", "4xx", "5xx")
	topByChoices := choices("path", "error", "status")
	return []commandDefinition{
		{Name: "dashboard", Description: "전체 서비스 운영 대시보드", Options: []commandOption{
			stringOption("since", "조회 기간", true, sinceChoices),
		}},
		{Name: "watch", Description: "지속 dashboard message 갱신 설정", Options: []commandOption{
			channelOption("channel", "dashboard를 표시할 채널", true),
			stringOption("interval", "갱신 주기", true, watchIntervalChoices),
		}},
		{Name: "unwatch", Description: "지속 dashboard message 갱신 중지"},
		{Name: "service", Description: "특정 서비스 상세 운영 상태", Options: []commandOption{
			stringOption("service", "조회할 서비스", true, serviceChoices),
			stringOption("since", "조회 기간", true, sinceChoices),
		}},
		{Name: "count", Description: "서비스 로그 숫자 집계", Options: []commandOption{
			stringOption("service", "조회할 서비스", true, serviceOrAllChoices),
			stringOption("since", "조회 기간", true, sinceChoices),
			stringOption("type", "집계 타입", true, countTypeChoices),
		}},
		{Name: "top", Description: "서비스 상위 문제 항목", Options: []commandOption{
			stringOption("service", "조회할 서비스", true, serviceOrAllChoices),
			stringOption("since", "조회 기간", true, sinceChoices),
			stringOption("by", "집계 기준", true, topByChoices),
		}},
		{Name: "slow", Description: "느린 API 조회", Options: []commandOption{
			stringOption("service", "조회할 서비스", true, serviceChoices),
			stringOption("since", "조회 기간", true, sinceChoices),
			integerOption("limit", "출력 개수(1..20)", false),
			integerOption("threshold_ms", "최소 latency ms", false),
		}},
		{Name: "copy-status", Description: "Report 과제 복사 API 상태", Options: []commandOption{
			stringOption("since", "조회 기간", true, sinceChoices),
		}},
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
			stringOption("since", "조회 기간", true, sinceChoices),
			stringOption("service", "조회할 서비스", false, serviceChoices),
		}},
		{Name: "trace", Description: "traceId 기준 로그 조회", Options: []commandOption{
			stringOption("trace_id", "조회할 traceId", true, nil),
		}},
		{Name: "alarm", Description: "CloudWatch ALARM 상태 조회"},
		{Name: "disk", Description: "CloudWatch log group 사용량 조회"},
		{Name: "retention", Description: "CloudWatch log retention 조회"},
		{Name: "help", Description: "명령어 도움말"},
	}
}

func RegisterGuildCommands(ctx context.Context, client *http.Client, botToken, applicationID, guildID string) error {
	return registerGuildCommands(ctx, client, botToken, applicationID, guildID, "https://discord.com/api/v10")
}

func registerGuildCommands(ctx context.Context, client *http.Client, botToken, applicationID, guildID, baseURL string) error {
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
	url := fmt.Sprintf("%s/applications/%s/guilds/%s/commands", strings.TrimRight(baseURL, "/"), applicationID, guildID)
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
		return registrationHTTPError(resp)
	}
	return nil
}

func registrationHTTPError(resp *http.Response) error {
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := security.SanitizeText(strings.TrimSpace(string(bodyBytes)))
	return &RegistrationError{
		StatusCode: resp.StatusCode,
		Body:       body,
		RetryAfter: retryAfterDuration(resp, bodyBytes),
	}
}

func retryAfterDuration(resp *http.Response, body []byte) time.Duration {
	if value := strings.TrimSpace(resp.Header.Get("Retry-After")); value != "" {
		if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
			return time.Duration(seconds * float64(time.Second))
		}
	}
	var payload struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.RetryAfter > 0 {
		return time.Duration(payload.RetryAfter * float64(time.Second))
	}
	return 0
}

func stringOption(name, description string, required bool, choices []commandChoice) commandOption {
	return commandOption{Type: 3, Name: name, Description: description, Required: required, Choices: choices}
}

func integerOption(name, description string, required bool) commandOption {
	return commandOption{Type: 4, Name: name, Description: description, Required: required}
}

func channelOption(name, description string, required bool) commandOption {
	return commandOption{Type: 7, Name: name, Description: description, Required: required}
}

func choices(values ...string) []commandChoice {
	result := make([]commandChoice, 0, len(values))
	for _, value := range values {
		result = append(result, commandChoice{Name: value, Value: value})
	}
	return result
}
