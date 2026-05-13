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
	serviceChoices := choices("gateway", "auth", "report", "online-judge", "post")
	reportServiceChoices := choices("report")
	reportOrAllChoices := choices("all", "report")
	reportSinceChoices := choices("15m", "30m", "1h")
	watchScopeChoices := choices("all", "service")
	watchIntervalChoices := choices("1m", "3m", "5m", "10m", "15m")
	alertActionChoices := choices("channel", "role", "role-clear", "on", "off", "status", "test")
	assignmentStatusChoices := choices("all", "published", "draft", "scheduled")
	assignmentWindowChoices := choices("today", "this-week")
	levelChoices := choices("INFO", "WARN", "ERROR")
	limitChoices := integerChoices(5, 10, 20)
	serviceViewChoices := choices("summary", "health")
	logModeChoices := choices("recent", "errors", "slow")
	alarmStateChoices := choices("ALARM", "OK", "INSUFFICIENT_DATA", "all")
	storageViewChoices := choices("usage", "retention")
	return []commandDefinition{
		{Name: "ops", Description: "A&I 운영 모니터링", Options: []commandOption{
			subcommandOption("dashboard", "전체 서비스 운영 대시보드", []commandOption{
				stringOption("since", "조회 기간", false, reportSinceChoices),
			}),
			subcommandOption("service", "특정 서비스 상세 상태", []commandOption{
				stringOption("service", "조회할 서비스", true, reportServiceChoices),
				stringOption("view", "상세 보기", false, serviceViewChoices),
				stringOption("since", "조회 기간", false, reportSinceChoices),
			}),
			subcommandOption("logs", "로그 조회와 집계", []commandOption{
				stringOption("service", "조회할 서비스", true, reportOrAllChoices),
				stringOption("mode", "조회 모드", false, logModeChoices),
				stringOption("level", "로그 레벨", false, levelChoices),
				stringOption("since", "조회 기간", false, reportSinceChoices),
				integerOption("limit", "출력 개수", false, limitChoices),
			}),
			subcommandOption("watch", "서비스 대시보드 자동 갱신 등록", []commandOption{
				stringOption("scope", "dashboard 범위", true, watchScopeChoices),
				stringOption("service", "서비스 범위일 때 대상 서비스", false, serviceChoices),
				stringOption("interval", "업데이트 주기", false, watchIntervalChoices),
			}),
			subcommandOption("unwatch", "서비스 대시보드 자동 갱신 중지", []commandOption{
				stringOption("scope", "dashboard 범위", true, watchScopeChoices),
				stringOption("service", "서비스 범위일 때 대상 서비스", false, serviceChoices),
			}),
			subcommandOption("watches", "등록된 서비스 대시보드 목록", nil),
			subcommandOption("alert", "서비스 알림 설정", []commandOption{
				stringOption("action", "알림 설정 동작", true, alertActionChoices),
				roleOption("role", "멘션할 운영자 역할", false),
			}),
			subcommandOption("logs-watch", "Report 로그 피드 등록", []commandOption{
				stringOption("service", "피드 서비스", true, serviceChoices),
				stringOption("mode", "피드 모드", true, logModeChoices),
				stringOption("interval", "조회 주기", false, watchIntervalChoices),
				stringOption("since", "조회 기간", false, reportSinceChoices),
				integerOption("limit", "출력 개수", false, limitChoices),
			}),
			subcommandOption("logs-unwatch", "Report 로그 피드 중지", []commandOption{
				stringOption("service", "피드 서비스", true, serviceChoices),
				stringOption("mode", "피드 모드", true, logModeChoices),
			}),
			subcommandOption("logs-watches", "등록된 로그 피드 목록", nil),
			subcommandOption("assignments", "Report 과제 이벤트 요약", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("status", "과제 상태", false, assignmentStatusChoices),
			}),
			subcommandOption("assignments-all", "Report 전체 코스 과제 요약", []commandOption{
				stringOption("window", "조회 창", false, assignmentWindowChoices),
			}),
			subcommandOption("assignment", "특정 과제 이벤트 조회", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("id", "assignmentId", true, nil),
			}),
			subcommandOption("assignment-check", "특정 과제 상태 검증", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("id", "assignmentId", true, nil),
			}),
			subcommandOption("submissions", "과제 제출/채점 상태 요약", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("assignment", "assignmentId", true, nil),
			}),
			subcommandOption("trace", "traceId 기준 로그 조회", []commandOption{
				stringOption("trace_id", "조회할 traceId", true, nil),
			}),
			subcommandOption("alarms", "CloudWatch alarm 조회", []commandOption{
				stringOption("state", "alarm 상태", false, alarmStateChoices),
				stringOption("service", "서비스 필터", false, serviceChoices),
			}),
			subcommandOption("storage", "CloudWatch log 사용량과 retention", []commandOption{
				stringOption("view", "storage 보기", false, storageViewChoices),
			}),
			subcommandOption("help", "운영 명령어 도움말", nil),
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

func subcommandOption(name, description string, options []commandOption) commandOption {
	return commandOption{Type: 1, Name: name, Description: description, Options: options}
}

func choices(values ...string) []commandChoice {
	result := make([]commandChoice, 0, len(values))
	for _, value := range values {
		result = append(result, commandChoice{Name: value, Value: value})
	}
	return result
}

func integerChoices(values ...int) []commandChoice {
	result := make([]commandChoice, 0, len(values))
	for _, value := range values {
		text := strconv.Itoa(value)
		result = append(result, commandChoice{Name: text, Value: value})
	}
	return result
}
