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
	if strings.TrimSpace(service) != "" {
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

func TimeRange(since time.Duration) (int64, int64) {
	now := time.Now()
	return now.Add(-since).Unix(), now.Unix()
}
