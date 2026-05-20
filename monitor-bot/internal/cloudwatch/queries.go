package cloudwatch

import (
	"fmt"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const (
	recentFields          = `fields @timestamp, level, logType, env, service.name, service.domain, service.domainCode, service.version, service.instanceId, trace.traceId, trace.requestId, event.eventType, assignmentId, request.pathVariables.assignmentId, http.method, http.path, http.route, http.statusCode, http.latencyMs, actor.userId, actor.role, actor.isAuthenticated, response.success, response.error.code, response.error.value, response.error.message, response.error.alert, message, tags`
	traceFields           = `fields @timestamp, level, logType, service.name, service.domain, service.domainCode, service.version, trace.traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, response.success, response.error.code, response.error.value, response.error.message, message, tags`
	errorFields           = `fields service.domain, service.name, http.path, http.route, http.statusCode, response.error.code, response.error.value, response.error.message, trace.traceId, logType, message`
	assignmentFields      = `fields service.name, http.method, http.path, http.route, http.statusCode, response.error.code, response.error.value, response.error.message, tags`
	assignmentAuditFields = `fields @timestamp, logType, service.name, service.domain, service.domainCode, event.eventType, event.resourceType, event.resourceId, event.assignmentId, event.courseSlug, event.title, event.occurredAt, assignmentId, courseSlug, assignment.assignmentId, assignment.courseSlug, assignment.title, assignment.status, request.pathVariables.assignmentId, request.pathVariables.courseSlug, request.pathVariables.course, actor.userId, actor.id, actor.role, actor.name, actor.displayName, actor.loginId, trace.traceId, trace.requestId, http.path, http.route, changedFields, changes, tags`
	countFields           = `fields service.domain, service.name, service.domainCode, logType, http.statusCode`
	slowFields            = `fields @timestamp, level, logType, service.domain, service.name, service.domainCode, trace.traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, response.error.code, response.error.value, response.error.message, response.error.alert, message`
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
	order := []string{"gateway", "auth", "report", "post"}
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
	return BuildRecentLogsQueryWithSearch(service, level, "", limit)
}

func BuildRecentLogsQueryWithSearch(service, level, search string, limit int) (string, error) {
	normalizedService, ok := security.NormalizeService(service)
	if !ok {
		return "", fmt.Errorf("unsupported service: %s", service)
	}
	normalized, ok := security.NormalizeLevel(level)
	if !ok {
		return "", fmt.Errorf("unsupported level: %s", level)
	}
	searchFilter := ""
	search = strings.TrimSpace(search)
	if search != "" {
		if !security.ValidateLogSearchQuery(search) {
			return "", fmt.Errorf("invalid query")
		}
		quoted := regexpQuoteForLogsInsights(search)
		searchFilter = fmt.Sprintf(`
| filter trace.traceId = "%[1]s" or trace.requestId = "%[1]s" or event.eventType = "%[1]s" or assignmentId = "%[1]s" or request.pathVariables.assignmentId = "%[1]s" or response.error.code = "%[1]s" or response.error.value = "%[1]s" or http.path like /%[2]s/ or http.route like /%[2]s/ or @message like /%[2]s/`, search, quoted)
	}
	limit32 := security.ClampLimit(limit, 20, 100)
	return fmt.Sprintf(`%s
%s
| filter logType != "" and service.name != ""
| filter level = "%s"
%s
| sort @timestamp desc
| limit %d`, recentFields, serviceDomainFilter(normalizedService), normalized, searchFilter, limit32), nil
}

func BuildTraceQuery(traceID string) (string, error) {
	traceID = strings.TrimSpace(traceID)
	if !security.ValidateTraceID(traceID) {
		return "", fmt.Errorf("invalid trace_id")
	}
	return fmt.Sprintf(`%s
| filter trace.traceId = "%s"
| sort @timestamp asc
| limit 100`, traceFields, traceID), nil
}

func BuildErrorsQuery(service string, limit int) string {
	limit32 := security.ClampLimit(limit, 20, 100)
	return fmt.Sprintf(`%s
%s
| filter logType in ["API_ERROR", "EVENT_ERROR"]
| stats count(*) as count by service.domain, service.name, http.route, http.path, http.statusCode, response.error.code, response.error.value, response.error.message, trace.traceId, logType, message
| sort count desc
| limit %d`, errorFields, serviceDomainFilter(service), limit32)
}

func BuildDashboardSummaryQuery(service string) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	filter := serviceDomainFilter(normalized)
	return fmt.Sprintf(`fields service.domain, service.name, service.domainCode, logType, http.route, http.path, http.statusCode, http.latencyMs, response.error.code, response.error.value
%s
| filter logType in ["API", "API_ERROR", "API_SLOW", "EVENT_ERROR", "SECURITY"]
| stats count(*) as count, max(@timestamp) as lastLog, max(http.latencyMs) as maxLatency, pct(http.latencyMs, 95) as p95 by service.domain, service.name, service.domainCode, logType, http.route, http.path, http.statusCode, response.error.code, response.error.value
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
	filter := strings.TrimSpace(strings.Join([]string{serviceDomainFilter(normalized), countTypeFilter(normalizedType)}, "\n"))
	return fmt.Sprintf(`%s
%s
| stats count(*) as count by service.domain, service.name, service.domainCode, logType, http.statusCode
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
| filter logType in ["API_ERROR", "EVENT_ERROR"]
| stats count(*) as count by %s
| sort count desc
| limit %d`, fields, serviceDomainFilter(normalized), groupBy, limit32), nil
}

func BuildSlowQuery(service string, thresholdMs, limit int) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	limit32 := security.ClampLimit(limit, 10, 20)
	thresholdFilter := `| filter logType = "API_SLOW"`
	if thresholdMs > 0 {
		thresholdFilter = fmt.Sprintf(`| filter logType = "API_SLOW" or http.latencyMs >= %d`, thresholdMs)
	}
	return fmt.Sprintf(`%s
%s
%s
| sort http.latencyMs desc
| limit %d`, slowFields, serviceDomainFilter(normalized), thresholdFilter, limit32), nil
}

func BuildSecurityQuery(service string, limit int) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	limit32 := security.ClampLimit(limit, 10, 20)
	return fmt.Sprintf(`%s
%s
| filter logType = "SECURITY"
| sort @timestamp desc
| limit %d`, recentFields, serviceDomainFilter(normalized), limit32), nil
}

func BuildAssignmentsQuery() string {
	return fmt.Sprintf(`%s
| filter service.name = "report-service"
| filter http.path like /\/assignments/ or http.route like /assignments/ or tags like /assignment/
| stats count(*) as count by http.method, http.path, http.statusCode, response.error.code, response.error.value, response.error.message
| sort count desc
| limit 20`, assignmentFields)
}

func BuildAssignmentQuery(assignmentID string) (string, error) {
	assignmentID = strings.TrimSpace(assignmentID)
	if !security.ValidateAssignmentID(assignmentID) {
		return "", fmt.Errorf("invalid assignment id")
	}
	quoted := regexpQuoteForLogsInsights(assignmentID)
	return fmt.Sprintf(`%s
%s
| filter http.path like /%s/ or http.route like /%s/ or tags like /%s/
| sort @timestamp desc
| limit 20`, recentFields, serviceDomainFilter("report"), quoted, quoted, quoted), nil
}

func BuildAssignmentAuditEventsQuery(search string, limit int) (string, error) {
	searchFilter := ""
	search = strings.TrimSpace(search)
	if search != "" {
		if !security.ValidateLogSearchQuery(search) {
			return "", fmt.Errorf("invalid query")
		}
		quoted := regexpQuoteForLogsInsights(search)
		searchFilter = fmt.Sprintf(`
| filter trace.traceId = "%[1]s" or trace.requestId = "%[1]s" or event.eventType = "%[1]s" or actor.userId = "%[1]s" or actor.id = "%[1]s" or event.assignmentId = "%[1]s" or event.resourceId = "%[1]s" or assignmentId = "%[1]s" or assignment.assignmentId = "%[1]s" or request.pathVariables.assignmentId = "%[1]s" or event.courseSlug = "%[1]s" or assignment.courseSlug = "%[1]s" or courseSlug = "%[1]s" or request.pathVariables.courseSlug = "%[1]s" or request.pathVariables.course = "%[1]s" or http.path like /%[2]s/ or http.route like /%[2]s/`, search, quoted)
	}
	limit32 := security.ClampLimit(limit, 20, 100)
	return fmt.Sprintf(`%s
%s
| filter logType = "EVENT"
| filter event.eventType in ["ASSIGNMENT_CREATED", "ASSIGNMENT_UPDATED", "ASSIGNMENT_DELETED", "ASSIGNMENT_PUBLISHED", "ASSIGNMENT_UNPUBLISHED"]
%s
| sort @timestamp desc
| limit %d`, assignmentAuditFields, serviceDomainFilter("report"), searchFilter, limit32), nil
}

func BuildAlertQuery(service string) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`%s
%s
| filter logType = "EVENT_ERROR" or (logType = "API_ERROR" and http.statusCode >= 500) or response.error.code like /^[0-9][78][0-9]{3}$/ or response.error.code like /^21[78][0-9]{2}$/ or response.error.code in [60701, 90701, 90801]
| sort @timestamp desc
| limit 50`, slowFields, serviceDomainFilter(normalized)), nil
}

func regexpQuoteForLogsInsights(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`.`, `\.`,
		`+`, `\+`,
		`*`, `\*`,
		`?`, `\?`,
		`^`, `\^`,
		`$`, `\$`,
		`(`, `\(`,
		`)`, `\)`,
		`[`, `\[`,
		`]`, `\]`,
		`{`, `\{`,
		`}`, `\}`,
		`|`, `\|`,
		`/`, `\/`,
	)
	return replacer.Replace(value)
}

func BuildLastLogQuery(service string) (string, error) {
	normalized, err := normalizeQueryService(service)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`fields @timestamp, service.name, level, logType
%s
| sort @timestamp desc
| limit 1`, serviceDomainFilter(normalized)), nil
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

func serviceDomainFilter(service string) string {
	if service == "all" {
		return ""
	}
	switch service {
	case "gateway":
		return `| filter service.domain = "gateway" or service.domainCode = 1 or service.name = "gateway"`
	case "report":
		return `| filter service.domain = "report" or service.domainCode = 4 or service.name = "web-service" or service.name = "report-service"`
	case "auth":
		return `| filter service.domain = "auth" or service.domainCode = 2 or service.name = "auth-service"`
	case "online-judge":
		return `| filter service.domain = "judge" or service.domainCode = 5 or service.name = "online-judge-service" or service.name = "judge-service"`
	case "post":
		return `| filter service.domain = "blog" or service.domainCode = 6 or service.name = "post-service" or service.name = "blog-service"`
	}
	return ""
}

func countTypeFilter(countType string) string {
	switch countType {
	case "api":
		return `| filter logType = "API" or logType = "API_ERROR"`
	case "error":
		return `| filter logType in ["API_ERROR", "EVENT_ERROR"]`
	case "4xx":
		return `| filter http.statusCode >= 400 and http.statusCode < 500`
	case "5xx":
		return `| filter http.statusCode >= 500`
	default:
		return ""
	}
}
