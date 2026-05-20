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
	alarmServiceChoices := namedChoices(choice("gateway"), choice("auth"), choice("report"), choiceAlias("blog", "post"), choice("online-judge"))
	reportSinceChoices := choices("15m", "30m", "1h")
	logSinceChoices := choices("15m", "30m", "1h", "24h")
	watchScopeChoices := choices("all", "service")
	watchIntervalChoices := choices("1m", "3m", "5m", "10m", "15m")
	alertActionChoices := choices("channel", "role", "role-clear", "on", "off", "status", "test")
	alertTargetChoices := choices("all", "general", "critical")
	assignmentStatusChoices := choices("all", "published", "draft", "scheduled")
	assignmentWindowChoices := choices("today", "this-week")
	assignmentViewChoices := choices("summary", "diagnosis", "raw")
	assignmentEventChoices := choices("publish-delayed", "draft-past-start", "stale-draft", "invalid-time", "missing-problem", "grading-failed")
	assignmentAckUntilChoices := choices("1h", "6h", "1d", "7d", "forever")
	levelChoices := choices("INFO", "WARN", "ERROR")
	limitChoices := integerChoices(5, 10, 20)
	serviceViewChoices := choices("summary", "health")
	logModeChoices := choices("recent", "errors", "slow", "security", "events")
	logWatchModeChoices := choices("recent", "errors", "slow", "security")
	helpTopicChoices := choices("overview", "dashboard", "logs", "alerts", "assignments", "feeds")
	helpCommandChoices := choices("dashboard", "service", "logs", "trace", "alert", "watch", "logs-watch", "assignments", "assignment", "assignment-check", "assignment-events", "assignment-ack", "submissions")
	alarmStateChoices := choices("ALARM", "OK", "INSUFFICIENT_DATA", "all")
	storageViewChoices := choices("usage", "retention")
	return []commandDefinition{
		{Name: "ops", Description: "A&I 운영 모니터링", Options: []commandOption{
			subcommandOption("dashboard", "전체 서비스 운영 대시보드", []commandOption{
				stringOption("since", "조회 기간", false, reportSinceChoices),
			}),
			subcommandOption("service", "특정 서비스 상세 상태", []commandOption{
				stringOption("service", "조회할 서비스", true, connectedServiceChoices),
				stringOption("view", "상세 보기", false, serviceViewChoices),
				stringOption("since", "조회 기간", false, reportSinceChoices),
			}),
			subcommandOption("logs", "로그 조회와 집계", []commandOption{
				stringOption("service", "조회할 서비스", false, connectedOrAllChoices),
				stringOption("mode", "조회 모드", false, logModeChoices),
				stringOption("level", "로그 레벨", false, levelChoices),
				stringOption("since", "조회 기간", false, logSinceChoices),
				integerOption("limit", "출력 개수", false, limitChoices),
				stringOption("query", "traceId, assignmentId, eventType, errorCode 검색어", false, nil),
			}),
			subcommandOption("watch", "서비스 대시보드 자동 갱신 등록", []commandOption{
				stringOption("scope", "dashboard 범위", true, watchScopeChoices),
				channelOption("channel", "대시보드를 고정할 채널", false),
				stringOption("service", "서비스 범위일 때 대상 서비스", false, connectedServiceChoices),
				stringOption("interval", "업데이트 주기", false, watchIntervalChoices),
			}),
			subcommandOption("unwatch", "서비스 대시보드 자동 갱신 중지", []commandOption{
				stringOption("scope", "dashboard 범위", true, watchScopeChoices),
				stringOption("service", "서비스 범위일 때 대상 서비스", false, connectedServiceChoices),
			}),
			subcommandOption("watches", "등록된 서비스 대시보드 목록", nil),
			subcommandOption("alert", "서비스 알림 설정", []commandOption{
				stringOption("action", "알림 설정 동작", true, alertActionChoices),
				stringOption("target", "general=운영 로그, critical=장애 알림, all=둘 다", false, alertTargetChoices),
				channelOption("channel", "알림 채널", false),
				roleOption("role", "멘션할 운영자 역할", false),
			}),
			subcommandOption("logs-watch", "로그 피드 등록", []commandOption{
				stringOption("service", "피드 서비스", true, connectedServiceChoices),
				stringOption("mode", "피드 모드", true, logWatchModeChoices),
				channelOption("channel", "로그 피드를 보낼 채널", false),
				stringOption("interval", "조회 주기", false, watchIntervalChoices),
				stringOption("since", "조회 기간", false, reportSinceChoices),
				integerOption("limit", "출력 개수", false, limitChoices),
			}),
			subcommandOption("logs-unwatch", "로그 피드 중지", []commandOption{
				stringOption("service", "피드 서비스", true, connectedServiceChoices),
				stringOption("mode", "피드 모드", true, logWatchModeChoices),
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
				stringOption("view", "보기 방식", false, assignmentViewChoices),
			}),
			subcommandOption("assignment-check", "특정 과제 상태 검증", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("id", "assignmentId", true, nil),
			}),
			subcommandOption("assignment-events", "과제 감지 이력과 dedupe 상태 조회", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("id", "assignmentId", true, nil),
			}),
			subcommandOption("assignment-ack", "알고 있는 과제 이슈 알림 중지", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("id", "assignmentId", true, nil),
				stringOption("event", "ack할 이벤트", true, assignmentEventChoices),
				stringOption("until", "ack 유지 기간", true, assignmentAckUntilChoices),
				stringOption("reason", "운영 기록 사유", true, nil),
			}),
			subcommandOption("assignment-unack", "과제 이슈 ack 해제", []commandOption{
				stringOption("course", "courseSlug", true, nil),
				stringOption("id", "assignmentId", true, nil),
				stringOption("event", "ack 해제할 이벤트", true, assignmentEventChoices),
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
				stringOption("service", "서비스 필터", false, alarmServiceChoices),
			}),
			subcommandOption("storage", "CloudWatch log 사용량과 retention", []commandOption{
				stringOption("view", "storage 보기", false, storageViewChoices),
			}),
			subcommandOption("help", "운영 명령어 도움말", []commandOption{
				stringOption("topic", "도움말 주제", false, helpTopicChoices),
				stringOption("command", "상세 설명할 명령어", false, helpCommandChoices),
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
