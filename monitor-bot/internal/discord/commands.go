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
	Options     []commandOption `json:"options,omitempty"`
}

type commandChoice struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
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
	connectedServiceChoices := namedChoices(choice("gateway"), choice("auth"), choice("report"), choiceAlias("blog", "post"))
	connectedOrAllChoices := namedChoices(choice("all"), choice("gateway"), choice("auth"), choice("report"), choiceAlias("blog", "post"))
	reportSinceChoices := choices("15m", "30m", "1h")
	logSinceChoices := choices("15m", "30m", "1h", "24h")
	watchIntervalChoices := choices("1m", "3m", "5m", "10m", "15m")
	dashboardActionChoices := choices("view", "watch", "unwatch", "status")
	alertActionChoices := choices("channel", "role", "role-clear", "on", "off", "status", "test")
	alertTargetChoices := choices("all", "general", "critical")
	assignmentStatusChoices := choices("all", "published", "draft", "scheduled")
	assignmentWindowChoices := choices("today", "this-week")
	assignmentViewChoices := choices("summary", "diagnosis", "raw", "events")
	assignmentActionChoices := choices("list", "check", "submissions", "ack", "unack")
	assignmentScopeChoices := choices("course", "all")
	assignmentEventChoices := choices("publish-delayed", "draft-past-start", "stale-draft", "invalid-time", "missing-problem", "grading-failed")
	assignmentAckUntilChoices := choices("1h", "6h", "1d", "7d", "forever")
	levelChoices := choices("INFO", "WARN", "ERROR")
	limitChoices := integerChoices(5, 10, 20)
	logModeChoices := choices("recent", "errors", "critical", "slow", "security", "events", "trace")
	logActionChoices := choices("view", "watch", "unwatch", "watches")
	helpTopicChoices := choices("overview", "dashboard", "logs", "alerts", "assignments")
	helpCommandChoices := choices("dashboard", "logs", "alert", "assignment")
	return []commandDefinition{
		{Name: "ops", Description: "A&I 운영 모니터링", Options: []commandOption{
			subcommandOption("dashboard", "서비스 상태와 dashboard watch", []commandOption{
				stringOption("action", "view/watch/unwatch/status", false, dashboardActionChoices),
				stringOption("service", "조회할 서비스", false, connectedServiceChoices),
				stringOption("since", "조회 기간", false, reportSinceChoices),
				channelOption("channel", "대시보드를 고정할 채널", false),
				stringOption("interval", "업데이트 주기", false, watchIntervalChoices),
			}),
			subcommandOption("logs", "로그 조회와 집계", []commandOption{
				stringOption("action", "view/watch/unwatch/watches", false, logActionChoices),
				stringOption("service", "조회할 서비스", false, connectedOrAllChoices),
				stringOption("mode", "조회 모드", false, logModeChoices),
				stringOption("level", "로그 레벨", false, levelChoices),
				stringOption("since", "조회 기간", false, logSinceChoices),
				integerOption("limit", "출력 개수", false, limitChoices),
				stringOption("query", "traceId, assignmentId, eventType, errorCode 검색어", false, nil),
				channelOption("channel", "로그 피드를 보낼 채널", false),
				stringOption("interval", "조회 주기", false, watchIntervalChoices),
			}),
			subcommandOption("alert", "서비스 알림 설정", []commandOption{
				stringOption("action", "알림 설정 동작", true, alertActionChoices),
				stringOption("target", "general=운영 로그, critical=장애 알림, all=둘 다", false, alertTargetChoices),
				channelOption("channel", "알림 채널", false),
				roleOption("role", "멘션할 운영자 역할", false),
			}),
			subcommandOption("assignment", "특정 과제 이벤트 조회", []commandOption{
				stringOption("action", "list/check/submissions/ack/unack", false, assignmentActionChoices),
				stringOption("scope", "조회 범위", false, assignmentScopeChoices),
				stringOption("course", "courseSlug", false, nil),
				stringOption("id", "assignmentId", false, nil),
				stringOption("view", "보기 방식", false, assignmentViewChoices),
				stringOption("status", "과제 상태", false, assignmentStatusChoices),
				stringOption("window", "조회 창", false, assignmentWindowChoices),
				stringOption("event", "ack/unack 대상 이벤트", false, assignmentEventChoices),
				stringOption("until", "ack 유지 기간", false, assignmentAckUntilChoices),
				stringOption("reason", "ack 사유", false, nil),
			}),
			subcommandOption("help", "운영 명령어 도움말", []commandOption{
				stringOption("topic", "도움말 주제", false, helpTopicChoices),
				stringOption("command", "상세 설명할 명령어", false, helpCommandChoices),
				stringOption("query", "상황 검색어", false, nil),
			}),
		}},
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

func integerOption(name, description string, required bool, choices []commandChoice) commandOption {
	return commandOption{Type: 4, Name: name, Description: description, Required: required, Choices: choices}
}

func roleOption(name, description string, required bool) commandOption {
	return commandOption{Type: 8, Name: name, Description: description, Required: required}
}

func channelOption(name, description string, required bool) commandOption {
	return commandOption{Type: 7, Name: name, Description: description, Required: required}
}

func subcommandOption(name, description string, options []commandOption) commandOption {
	return commandOption{Type: 1, Name: name, Description: description, Options: options}
}

func choices(values ...string) []commandChoice {
	result := make([]commandChoice, 0, len(values))
	for _, value := range values {
		result = append(result, choice(value))
	}
	return result
}

func namedChoices(values ...commandChoice) []commandChoice {
	return values
}

func choice(value string) commandChoice {
	return commandChoice{Name: value, Value: value}
}

func choiceAlias(name, value string) commandChoice {
	return commandChoice{Name: name, Value: value}
}

func integerChoices(values ...int) []commandChoice {
	result := make([]commandChoice, 0, len(values))
	for _, value := range values {
		text := strconv.Itoa(value)
		result = append(result, commandChoice{Name: text, Value: value})
	}
	return result
}
