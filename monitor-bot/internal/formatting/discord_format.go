package formatting

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const DiscordMessageLimit = 2000

type ServiceStatus struct {
	Service string
	State   string
	Detail  string
}

func TruncateDiscordMessage(message string) string {
	const suffix = "\n...(truncated)"
	if len([]rune(message)) <= DiscordMessageLimit {
		return message
	}
	runes := []rune(message)
	return string(runes[:DiscordMessageLimit-len([]rune(suffix))]) + suffix
}

func FormatStatus(statuses []ServiceStatus) string {
	var b strings.Builder
	b.WriteString("A&I 서비스 상태\n")
	for _, status := range statuses {
		icon := "🟡"
		switch strings.ToUpper(status.State) {
		case "UP":
			icon = "🟢"
		case "DOWN":
			icon = "🔴"
		}
		detail := strings.TrimSpace(security.SanitizeText(status.Detail))
		if detail == "" {
			detail = status.State
		}
		fmt.Fprintf(&b, "%s `%s` %s - %s\n", icon, status.Service, strings.ToUpper(status.State), detail)
	}
	return TruncateDiscordMessage(b.String())
}

func FormatLogRows(title string, rows []map[string]string) string {
	if len(rows) == 0 {
		return title + "\n조회 결과가 없습니다."
	}
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	for i, row := range rows {
		fmt.Fprintf(&b, "\n%d. ", i+1)
		writeCompactRow(&b, row)
	}
	return TruncateDiscordMessage(b.String())
}

func FormatErrors(rows []map[string]string) string {
	return FormatLogRows("상위 에러", rows)
}

func FormatTrace(rows []map[string]string) string {
	return FormatLogRows("Trace 조회 결과", rows)
}

func FormatAlarms(names []string) string {
	if len(names) == 0 {
		return "현재 ALARM 상태 없음"
	}
	sort.Strings(names)
	return TruncateDiscordMessage("ALARM 상태 알람\n- " + strings.Join(names, "\n- "))
}

func HelpText() string {
	return strings.TrimSpace(`/status - 전체 서비스 상태 요약
/health service:<service> - 특정 서비스 health 조회
/logs service:<service> since:<5m|15m|30m|1h|3h> level:<INFO|WARN|ERROR> - 최근 로그 조회
/errors service:<optional> since:<duration> - 에러 집계
/trace trace_id:<traceId> - traceId 기준 시간순 조회
/alarm - CloudWatch ALARM 상태 조회
/help - 명령어 도움말`)
}

func writeCompactRow(b *strings.Builder, row map[string]string) {
	pairs := security.FilterDisplayPairs(row)
	if len(pairs) == 0 {
		b.WriteString("표시 가능한 필드 없음")
		return
	}
	for i, pair := range pairs {
		if i > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(b, "%s=%s", pair[0], pair[1])
	}
}
