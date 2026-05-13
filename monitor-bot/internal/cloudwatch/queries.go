package cloudwatch

import (
	"fmt"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const (
	recentFields = `fields @timestamp, level, logType, env, service.name, service.domainCode, service.version, trace.traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, actor.userId, actor.role, actor.isAuthenticated, response.success, response.error.code, response.error.value, response.error.message, response.error.alert, message, tags`
	traceFields  = `fields @timestamp, level, logType, service.name, service.version, trace.traceId, traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, response.success, response.error.code, response.error.value, response.error.message, message, tags`
	errorFields  = `fields service.name, http.path, http.statusCode, response.error.code, response.error.value, response.error.message`
	countFields  = `fields service.name, logType, level, http.statusCode`
	slowFields   = `fields @timestamp, service.name, trace.traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, response.error.code, response.error.value, response.error.message, message`
)

func LogGroupsForService(logGroups map[string]string, service string) ([]string, error) {
	normalized, ok := security.NormalizeService(service)
	if !ok {
		return nil, fmt.Errorf("unsupported service: %s", service)
	}
	group := strings.TrimSpace(logGroups[normalized])
	if group == "" {
		return nil, fmt.Errorf("log group is not configured for service: %s", normalized)
	}
	return []string{group}, nil
}

func LogGroupsForOptionalService(logGroups map[string]string, service string, maxGroups int) ([]string, error) {
	if strings.TrimSpace(service) != "" && strings.TrimSpace(service) != "all" {
		return LogGroupsForService(logGroups, service)
	}
	if maxGroups <= 0 {
		maxGroups = 5
	}
	order := []string{"gateway", "auth", "report", "online-judge", "post"}
	groups := make([]string, 0, len(order))
	for _, name := range order {
		if group := strings.TrimSpace(logGroups[name]); group != "" {
			groups = append(groups, group)
			if len(groups) >= maxGroups {
				break
			}
		}
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no log groups are configured")
	}
	return groups, nil
}

func BuildRecentLogsQuery(service, level string, limit int) (string, error) {
	normalizedService, ok := security.NormalizeService(service)
	if !ok {
		return "", fmt.Errorf("unsupported service: %s", service)
	}
	normalized, ok := security.NormalizeLevel(level)
	if !ok {
		return "", fmt.Errorf("unsupported level: %s", level)
	}
	limit32 := security.ClampLimit(limit, 20, 100)
	if normalizedService == "report" {
		return fmt.Sprintf(`%s
| filter service.name = "report-service"
| filter level = "%s"
| sort @timestamp desc
| limit %d`, recentFields, normalized, limit32), nil
	}
	return fmt.Sprintf(`%s
| filter level = "%s"
| sort @timestamp desc
| limit %d`, recentFields, normalized, limit32), nil
}

func BuildTraceQuery(traceID string) (string, error) {
	traceID = strings.TrimSpace(traceID)
	if !security.ValidateTraceID(traceID) {
		return "", fmt.Errorf("invalid trace_id")
	}
	return fmt.Sprintf(`%s
| filter trace.traceId = "%s" or traceId = "%s"
| sort @timestamp asc
| limit 100`, traceFields, traceID, traceID), nil
}

func BuildErrorsQuery(service string, limit int) string {
	limit32 := security.ClampLimit(limit, 20, 100)
	if normalized, ok := security.NormalizeService(service); ok && normalized == "report" {
		return fmt.Sprintf(`%s
| filter service.name = "report-service"
| filter level = "ERROR" or level = "WARN" or logType = "API_ERROR" or http.statusCode >= 400
| stats count(*) as count by http.path, http.statusCode, response.error.code, response.error.value, response.error.message
| sort count desc
| limit %d`, errorFields, limit32)
	}
	return fmt.Sprintf(`%s
| filter level = "ERROR" or level = "WARN" or logType = "API_ERROR" or http.statusCode >= 400
| stats count(*) as count by service.name, http.path, http.statusCode, response.error.code, response.error.value
| sort count desc
| limit %d`, errorFields, limit32)
}

func BuildDashboardSummaryQuery(service string) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	filter := serviceNameFilter(normalized)
	return fmt.Sprintf(`fields service.name, logType, level, http.path, http.statusCode, http.latencyMs, response.error.code, response.error.value
%s
| filter logType = "API" or logType = "API_ERROR" or level = "WARN" or level = "ERROR"
| stats count(*) as count, max(@timestamp) as lastLog, max(http.latencyMs) as maxLatency, pct(http.latencyMs, 95) as p95 by service.name, logType, level, http.path, http.statusCode, response.error.code, response.error.value
| sort count desc
| limit 100`, filter), nil
}

func BuildCountQuery(service, countType string) (string, error) {
	normalized, ok := security.NormalizeServiceOrAll(service)
	if !ok {
		return "", fmt.Errorf("unsupported service: %s", service)
	}
	normalizedType, ok := security.NormalizeCountType(countType)
	if !ok {
		return "", fmt.Errorf("unsupported count type: %s", countType)
	}
	filter := strings.TrimSpace(strings.Join([]string{serviceNameFilter(normalized), countTypeFilter(normalizedType)}, "\n"))
	return fmt.Sprintf(`%s
%s
| stats count(*) as count by logType, level, http.statusCode
| sort count desc
| limit 50`, countFields, filter), nil
}

func BuildTopQuery(service, by string, limit int) (string, error) {
	normalized, ok := security.NormalizeServiceOrAll(service)
	if !ok {
		return "", fmt.Errorf("unsupported service: %s", service)
	}
	normalizedBy, ok := security.NormalizeTopBy(by)
	if !ok {
		return "", fmt.Errorf("unsupported top by: %s", by)
	}
	groupBy := "http.path"
	fields := `fields service.name, http.path, http.statusCode, response.error.code, response.error.value, response.error.message`
	switch normalizedBy {
	case "error":
		groupBy = "http.path, http.statusCode, response.error.code, response.error.value, response.error.message"
	case "status":
		groupBy = "http.statusCode"
		fields = `fields service.name, http.statusCode`
	}
	limit32 := security.ClampLimit(limit, 10, 20)
	return fmt.Sprintf(`%s
%s
| filter logType = "API_ERROR" or level = "ERROR" or http.statusCode >= 400
| stats count(*) as count by %s
| sort count desc
| limit %d`, fields, serviceNameFilter(normalized), groupBy, limit32), nil
}

func BuildSlowQuery(service string, thresholdMs, limit int) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	limit32 := security.ClampLimit(limit, 10, 20)
	thresholdFilter := ""
	if thresholdMs > 0 {
		thresholdFilter = fmt.Sprintf("\n| filter http.latencyMs >= %d", thresholdMs)
	}
	return fmt.Sprintf(`%s
%s
| filter logType = "API" or logType = "API_ERROR"%s
| sort http.latencyMs desc
| limit %d`, slowFields, serviceNameFilter(normalized), thresholdFilter, limit32), nil
}

func BuildAlertQuery(service string) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`%s
%s
| filter logType = "API_ERROR" or level = "ERROR" or http.statusCode >= 500
| sort @timestamp desc
| limit 50`, slowFields, serviceNameFilter(normalized)), nil
}

func BuildLastLogQuery(service string) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`fields @timestamp, service.name, level, logType
%s
| sort @timestamp desc
| limit 1`, serviceNameFilter(normalized)), nil
}

func TimeRange(since time.Duration) (int64, int64) {
	now := time.Now()
	return now.Add(-since).Unix(), now.Unix()
}

func normalizeQueryService(service string) (string, error) {
	normalized, ok := security.NormalizeService(service)
	if !ok {
		return "", fmt.Errorf("unsupported service: %s", service)
	}
	return normalized, nil
}

func serviceNameFilter(service string) string {
	if service == "all" {
		return ""
	}
	if service == "report" {
		return `| filter service.name = "report-service"`
	}
	return ""
}

func countTypeFilter(countType string) string {
	switch countType {
	case "api":
		return `| filter logType = "API" or logType = "API_ERROR"`
	case "error":
		return `| filter logType = "API_ERROR" or level = "WARN" or level = "ERROR"`
	case "4xx":
		return `| filter http.statusCode >= 400 and http.statusCode < 500`
	case "5xx":
		return `| filter http.statusCode >= 500`
	default:
		return ""
	}
}
