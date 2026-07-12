package discord

import (
	"context"
	"strconv"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

func (h *Handler) opsLogsCommand(ctx context.Context, interaction Interaction, subcommand ApplicationCommandOpt) string {
	action := optionStringFromOptions(subcommand.Options, "action")
	if action == "" {
		action = "view"
	}
	serviceOption := optionStringFromOptions(subcommand.Options, "service")
	if serviceOption == "" {
		serviceOption = "all"
	}
	mode := optionStringFromOptions(subcommand.Options, "mode")
	if mode == "" {
		mode = "errors"
	}
	defaults := map[string]string{"service": serviceOption, "mode": mode}
	switch action {
	case "watch":
		if serviceOption == "all" {
			defaults["service"] = "report"
		}
		return h.opsLogsWatchCommand(ctx, interaction, ApplicationCommandOpt{Options: withDefaultOptions(subcommand.Options, defaults)})
	case "unwatch":
		if serviceOption == "all" {
			defaults["service"] = "report"
		}
		return h.opsLogsUnwatchCommand(ctx, ApplicationCommandOpt{Options: withDefaultOptions(subcommand.Options, defaults)})
	case "watches":
		return h.opsLogsWatchesCommand(ctx)
	case "view":
	default:
		return "지원하지 않는 logs action입니다. view, watch, unwatch, watches 중 하나를 사용하세요."
	}
	service, ok := security.NormalizeServiceOrAll(serviceOption)
	if !ok {
		return "지원하지 않는 service입니다."
	}
	if service != "all" && !isOpsV2Service(service) {
		return "status: NO_V2_LOG\nservice: " + opsDisplayServiceName(service) + "\nkey findings: V2 로그 연동 전까지 장애 판단 대상이 아닙니다.\nrecommended next commands:\n- `/ops logs service:report mode:errors since:30m limit:10`"
	}
	since := optionStringFromOptions(subcommand.Options, "since")
	if since == "" {
		since = "30m"
	}
	level := optionStringFromOptions(subcommand.Options, "level")
	if level == "" {
		level = "ERROR"
	}
	query := optionStringFromOptions(subcommand.Options, "query")
	if optionStringFromOptions(subcommand.Options, "mode") == "" && looksLikeTraceQuery(query) {
		mode = "trace"
	}
	limit := parseOpsLimit(optionStringFromOptions(subcommand.Options, "limit"), 10)
	switch mode {
	case "recent":
		if service == "all" {
			return allServiceGuardMessage()
		}
		options := []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("level", level),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		}
		if query != "" {
			options = append(options, stringInteractionOption("query", query))
		}
		return withNext(h.logsCommand(ctx, interactionForCommand("logs", options)), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops dashboard service:"+opsDisplayServiceName(service))
	case "errors":
		if service == "all" && !sinceAllowsAllQuery(since) {
			return allServiceSinceGuardMessage()
		}
		next := []string{"/ops dashboard since:" + since}
		if service != "all" {
			next = append(next, "/ops logs service:"+opsDisplayServiceName(service)+" mode:recent level:ERROR", "/ops dashboard service:"+opsDisplayServiceName(service))
		}
		return withNext(h.errorsCommand(ctx, interactionForCommand("errors", []ApplicationCommandOpt{
			stringInteractionOption("since", since),
			stringInteractionOption("service", service),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), next...)
	case "critical":
		options := []ApplicationCommandOpt{
			stringInteractionOption("since", since),
			stringInteractionOption("service", service),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		}
		return withNext(h.errorsCommand(ctx, interactionForCommand("errors", options)), "/ops alert action:status", "/ops dashboard since:"+since)
	case "top":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.topCommand(ctx, interactionForCommand("top", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("by", "path"),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops logs mode:trace query:<traceId>")
	case "slow":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.slowCommand(ctx, interactionForCommand("slow", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops logs mode:trace query:<traceId>")
	case "security":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.securityLogsCommand(ctx, interactionForCommand("security", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops logs mode:trace query:<traceId>")
	case "events":
		if service != "all" && service != "report" {
			return "mode:events는 현재 report assignment audit EVENT 로그만 지원합니다."
		}
		return h.assignmentAuditLogsCommand(ctx, since, query, limit)
	case "trace":
		if strings.TrimSpace(query) == "" {
			return "mode:trace에는 query:<traceId>가 필요합니다."
		}
		return h.traceCommand(ctx, interactionForCommand("trace", []ApplicationCommandOpt{
			stringInteractionOption("trace_id", query),
		}))
	default:
		return "지원하지 않는 logs mode입니다."
	}
}

func (h *Handler) serviceCommand(ctx context.Context, interaction Interaction) string {
	service, ok := security.NormalizeService(optionString(interaction, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	groups, err := cw.LogGroupsForOptionalService(h.cfg.LogGroups, service, h.cfg.CloudWatchMaxLogGroups)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	countQuery, err := cw.BuildCountQuery(service, "all")
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	topQuery, err := cw.BuildTopQuery(service, "path", 10)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	countRows, err := h.logs.Query(ctx, groups, countQuery, since, 50)
	if err != nil {
		return "CloudWatch count 조회 실패: " + security.SanitizeText(err.Error())
	}
	topRows, err := h.logs.Query(ctx, groups, topQuery, since, 10)
	if err != nil {
		return "CloudWatch top 조회 실패: " + security.SanitizeText(err.Error())
	}
	errorRows, err := h.logs.Query(ctx, groups, cw.BuildErrorsQuery(service, 10), since, 10)
	if err != nil {
		return "CloudWatch errors 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatServiceDetail(formatting.ServiceDetailInput{
		Service:   service,
		LogGroup:  groups[0],
		Since:     sinceLabel,
		Health:    h.health.Check(ctx, service),
		CountRows: countRows,
		TopRows:   topRows,
		ErrorRows: errorRows,
	})
}

func (h *Handler) topCommand(ctx context.Context, interaction Interaction) string {
	service, ok := security.NormalizeServiceOrAll(optionString(interaction, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	by, ok := security.NormalizeTopBy(optionString(interaction, "by"))
	if !ok {
		return "지원하지 않는 by 값입니다."
	}
	if service == "all" {
		return allServiceGuardMessage()
	}
	groups, err := cw.LogGroupsForOptionalService(h.cfg.LogGroups, service, h.cfg.CloudWatchMaxLogGroups)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	limit := parseOpsLimit(optionString(interaction, "limit"), 10)
	query, err := cw.BuildTopQuery(service, by, limit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, int32(limit))
	if err != nil {
		return "CloudWatch top 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatTopSummaryWithLimit(service, sinceLabel, by, rows, limit)
}

func (h *Handler) slowCommand(ctx context.Context, interaction Interaction) string {
	service, ok := security.NormalizeService(optionString(interaction, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	limit := security.ParsePositiveInt(optionString(interaction, "limit"), 10)
	if limit > 20 {
		limit = 20
	}
	threshold := security.ParsePositiveInt(optionString(interaction, "threshold_ms"), 0)
	groups, err := cw.LogGroupsForService(h.cfg.LogGroups, service)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	query, err := cw.BuildSlowQuery(service, threshold, limit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, int32(limit))
	if err != nil {
		return "CloudWatch slow 조회 실패: " + security.SanitizeText(err.Error())
	}
	return appendTraceNext(formatting.FormatSlowSummary(service, sinceLabel, rows), rows)
}

func (h *Handler) securityLogsCommand(ctx context.Context, interaction Interaction) string {
	service, ok := security.NormalizeService(optionString(interaction, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	limit := security.ParsePositiveInt(optionString(interaction, "limit"), 10)
	if limit > 20 {
		limit = 20
	}
	groups, err := cw.LogGroupsForService(h.cfg.LogGroups, service)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	query, err := cw.BuildSecurityQuery(service, limit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, int32(limit))
	if err != nil {
		return "CloudWatch security 조회 실패: " + security.SanitizeText(err.Error())
	}
	return appendTraceNext(formatting.FormatLogRows("Security 로그", rows), rows)
}

func (h *Handler) logsCommand(ctx context.Context, interaction Interaction) string {
	service, ok := security.NormalizeService(optionString(interaction, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	since, ok := security.ParseSince(optionString(interaction, "since"))
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	level, ok := security.NormalizeLevel(optionString(interaction, "level"))
	if !ok {
		return "지원하지 않는 level 값입니다."
	}
	groups, err := cw.LogGroupsForService(h.cfg.LogGroups, service)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	limit := parseOpsLimit(optionString(interaction, "limit"), h.cfg.CloudWatchQueryLimit)
	search := optionString(interaction, "query")
	query, err := cw.BuildRecentLogsQueryWithSearch(service, level, search, limit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, int32(limit))
	if err != nil {
		return "CloudWatch logs 조회 실패: " + security.SanitizeText(err.Error())
	}
	title := "최근 로그"
	if search != "" {
		title = "최근 로그 query=" + security.SanitizeText(search)
	}
	return appendTraceNext(formatting.FormatLogRows(title, rows), rows)
}

func (h *Handler) assignmentAuditLogsCommand(ctx context.Context, sinceLabel, search string, limit int) string {
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	groups, err := cw.LogGroupsForService(h.cfg.LogGroups, "report")
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	query, err := cw.BuildAssignmentAuditEventsQuery(search, limit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, int32(limit))
	if err != nil {
		return "CloudWatch assignment events 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatAssignmentAuditRows(rows, search)
}

func (h *Handler) errorsCommand(ctx context.Context, interaction Interaction) string {
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	service := optionString(interaction, "service")
	if isAllServiceQuery(service) && !sinceAllowsAllQuery(sinceLabel) {
		return allServiceSinceGuardMessage()
	}
	groups, err := cw.LogGroupsForOptionalService(h.cfg.LogGroups, service, h.cfg.CloudWatchMaxLogGroups)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	limit := parseOpsLimit(optionString(interaction, "limit"), h.cfg.CloudWatchQueryLimit)
	rows, err := h.logs.Query(ctx, groups, cw.BuildErrorsQuery(service, limit), since, int32(limit))
	if err != nil {
		return "CloudWatch errors 조회 실패: " + security.SanitizeText(err.Error())
	}
	return appendTraceNext(formatting.FormatErrors(rows), rows)
}

func (h *Handler) traceCommand(ctx context.Context, interaction Interaction) string {
	traceID := optionString(interaction, "trace_id")
	query, err := cw.BuildTraceQuery(traceID)
	if err != nil {
		return "올바르지 않은 trace_id입니다."
	}
	groups, err := cw.LogGroupsForOptionalService(h.cfg.LogGroups, "", h.cfg.CloudWatchMaxLogGroups)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, 3*time.Hour, 100)
	if err != nil {
		return "CloudWatch trace 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatTrace(rows)
}

func appendTraceNext(content string, rows []map[string]string) string {
	traceID := firstTraceID(rows)
	if traceID == "" {
		return content
	}
	return withNext(content, "/ops logs mode:trace query:"+traceID)
}

func looksLikeTraceQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" || strings.Contains(trimmed, "-") || strings.Contains(trimmed, " ") {
		return false
	}
	return len(trimmed) >= 16 && security.ValidateTraceID(trimmed)
}

func firstTraceID(rows []map[string]string) string {
	for _, row := range rows {
		traceID := strings.TrimSpace(row["trace.traceId"])
		if traceID != "" && security.ValidateTraceID(traceID) {
			return traceID
		}
	}
	return ""
}

func isAllServiceQuery(service string) bool {
	normalized := strings.ToLower(strings.TrimSpace(service))
	return normalized == "" || normalized == "all"
}

func sinceAllowsAllQuery(since string) bool {
	switch strings.TrimSpace(since) {
	case "5m", "15m", "30m":
		return true
	default:
		return false
	}
}

func allServiceGuardMessage() string {
	return "service:all은 비용 보호를 위해 errors/dashboard 계열에서만 지원합니다. `/ops logs service:all mode:errors since:30m` 또는 `/ops dashboard`를 사용하세요."
}

func allServiceSinceGuardMessage() string {
	return "service:all 조회는 CloudWatch 비용 보호를 위해 since 30m 이하만 허용합니다."
}
