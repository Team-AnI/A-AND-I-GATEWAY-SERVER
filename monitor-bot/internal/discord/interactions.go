package discord

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const (
	interactionTypePing               = 1
	interactionTypeApplicationCommand = 2
	maxBodyBytes                      = 64 * 1024
)

var serviceOrder = []string{"gateway", "report", "auth", "online-judge", "post"}

type Handler struct {
	cfg          config.Config
	health       *health.Client
	logs         *cw.LogsClient
	alarms       *cw.AlarmClient
	httpClient   *http.Client
	replayWindow time.Duration
	watcher      Watcher
}

type Watcher interface {
	WatchDashboard(ctx context.Context, channelID string, interval time.Duration) (string, error)
	UnwatchDashboard(ctx context.Context) (string, error)
}

type Interaction struct {
	ID            string                 `json:"id"`
	ApplicationID string                 `json:"application_id"`
	Type          int                    `json:"type"`
	Token         string                 `json:"token"`
	GuildID       string                 `json:"guild_id"`
	Member        *Member                `json:"member,omitempty"`
	Data          ApplicationCommandData `json:"data"`
}

func (h *Handler) SetWatcher(watcher Watcher) {
	h.watcher = watcher
}

type Member struct {
	Roles []string `json:"roles"`
}

type ApplicationCommandData struct {
	Name    string                  `json:"name"`
	Options []ApplicationCommandOpt `json:"options,omitempty"`
}

type ApplicationCommandOpt struct {
	Type    int                     `json:"type,omitempty"`
	Name    string                  `json:"name"`
	Value   json.RawMessage         `json:"value,omitempty"`
	Options []ApplicationCommandOpt `json:"options,omitempty"`
}

func NewHandler(cfg config.Config, healthClient *health.Client, logsClient *cw.LogsClient, alarmClient *cw.AlarmClient) *Handler {
	return &Handler{
		cfg:          cfg,
		health:       healthClient,
		logs:         logsClient,
		alarms:       alarmClient,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		replayWindow: 5 * time.Minute,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := VerifySignature(
		h.cfg.DiscordPublicKey,
		r.Header.Get("X-Signature-Timestamp"),
		body,
		r.Header.Get("X-Signature-Ed25519"),
		time.Now(),
		h.replayWindow,
	); err != nil {
		http.Error(w, "invalid request signature", http.StatusUnauthorized)
		return
	}

	var interaction Interaction
	if err := json.Unmarshal(body, &interaction); err != nil {
		http.Error(w, "bad interaction payload", http.StatusBadRequest)
		return
	}
	if interaction.Type == interactionTypePing {
		writeJSON(w, pongResponse())
		return
	}
	if interaction.Type != interactionTypeApplicationCommand {
		writeJSON(w, messageResponse("지원하지 않는 interaction type입니다.", h.cfg.DiscordEphemeralResponses))
		return
	}
	if err := h.authorize(interaction); err != nil {
		writeJSON(w, messageResponse(err.Error(), true))
		return
	}

	writeJSON(w, deferredResponse(h.cfg.DiscordEphemeralResponses))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), h.commandTimeout(interaction.Data.Name))
		defer cancel()
		content := h.execute(ctx, interaction)
		if err := SendFollowUp(ctx, h.httpClient, h.cfg.DiscordApplicationID, interaction.Token, content, h.cfg.DiscordEphemeralResponses); err != nil {
			log.Printf("discord follow-up failed: %v", err)
		}
	}()
}

func (h *Handler) authorize(interaction Interaction) error {
	if h.cfg.DiscordAllowedGuildID != "" && interaction.GuildID != h.cfg.DiscordAllowedGuildID {
		return errors.New("허용되지 않은 Discord guild입니다.")
	}
	if len(h.cfg.DiscordAllowedRoleIDs) == 0 {
		return nil
	}
	if interaction.Member == nil {
		return errors.New("명령 실행 권한이 없습니다.")
	}
	allowed := make(map[string]struct{}, len(h.cfg.DiscordAllowedRoleIDs))
	for _, role := range h.cfg.DiscordAllowedRoleIDs {
		allowed[role] = struct{}{}
	}
	for _, role := range interaction.Member.Roles {
		if _, ok := allowed[role]; ok {
			return nil
		}
	}
	return errors.New("명령 실행 권한이 없습니다.")
}

func (h *Handler) execute(ctx context.Context, interaction Interaction) string {
	switch interaction.Data.Name {
	case "ops":
		return h.opsCommand(ctx, interaction)
	case "dashboard":
		return legacyNoticeFor("dashboard") + h.dashboardCommand(ctx, interaction)
	case "watch":
		return h.watchCommand(ctx, interaction)
	case "unwatch":
		return h.unwatchCommand(ctx, interaction)
	case "service":
		return legacyNoticeFor("service") + h.serviceCommand(ctx, interaction)
	case "count":
		return legacyNoticeFor("count") + h.countCommand(ctx, interaction)
	case "top":
		return legacyNoticeFor("top") + h.topCommand(ctx, interaction)
	case "slow":
		return legacyNoticeFor("slow") + h.slowCommand(ctx, interaction)
	case "copy-status":
		return legacyNoticeFor("copy-status") + h.copyStatusCommand(ctx, interaction)
	case "status":
		return legacyNoticeFor("status") + formatting.FormatStatus(h.health.CheckAll(ctx, serviceOrder))
	case "health":
		notice := legacyNoticeFor("health")
		service, ok := security.NormalizeService(optionString(interaction, "service"))
		if !ok {
			return "지원하지 않는 service입니다."
		}
		return notice + formatting.FormatStatus([]formatting.ServiceStatus{h.health.Check(ctx, service)})
	case "logs":
		return legacyNoticeFor("logs") + h.logsCommand(ctx, interaction)
	case "errors":
		return legacyNoticeFor("errors") + h.errorsCommand(ctx, interaction)
	case "trace":
		return legacyNoticeFor("trace") + h.traceCommand(ctx, interaction)
	case "alarm":
		notice := legacyNoticeFor("alarm")
		names, err := h.alarms.AlarmNames(ctx)
		if err != nil {
			return "CloudWatch alarm 조회 실패: " + security.SanitizeText(err.Error())
		}
		return notice + formatting.FormatAlarms(names)
	case "disk":
		return legacyNoticeFor("disk") + h.retentionCommand(ctx, "💽 CloudWatch Log Usage")
	case "retention":
		return legacyNoticeFor("retention") + h.retentionCommand(ctx, "📦 CloudWatch Log Retention")
	case "help":
		return formatting.HelpText()
	default:
		return "지원하지 않는 명령어입니다. /ops help 를 확인하세요."
	}
}

func (h *Handler) opsCommand(ctx context.Context, interaction Interaction) string {
	subcommand, ok := opsSubcommand(interaction)
	if !ok {
		return formatting.HelpText()
	}
	switch subcommand.Name {
	case "dashboard":
		return h.dashboardCommand(ctx, interactionForCommand("dashboard", withDefaultOptions(subcommand.Options, map[string]string{"since": "30m", "view": "summary"})))
	case "service":
		return h.opsServiceCommand(ctx, subcommand)
	case "logs":
		return h.opsLogsCommand(ctx, subcommand)
	case "trace":
		return h.traceCommand(ctx, interactionForCommand("trace", subcommand.Options))
	case "alarms":
		return h.opsAlarmsCommand(ctx, subcommand)
	case "storage":
		view := optionStringFromOptions(subcommand.Options, "view")
		if view == "" {
			view = "usage"
		}
		switch view {
		case "usage":
			return h.retentionCommand(ctx, "💽 CloudWatch Log Usage")
		case "retention":
			return h.retentionCommand(ctx, "📦 CloudWatch Log Retention")
		default:
			return "지원하지 않는 storage view입니다."
		}
	case "help":
		return formatting.HelpText()
	default:
		return "지원하지 않는 /ops subcommand입니다. /ops help 를 확인하세요."
	}
}

func (h *Handler) watchCommand(ctx context.Context, interaction Interaction) string {
	if h.watcher == nil {
		return "dashboard watcher가 설정되어 있지 않습니다."
	}
	channelID := optionString(interaction, "channel")
	interval, ok := parseWatchInterval(optionString(interaction, "interval"))
	if !ok {
		return "지원하지 않는 interval 값입니다."
	}
	message, err := h.watcher.WatchDashboard(ctx, channelID, interval)
	if err != nil {
		return "dashboard watch 설정 실패: " + security.SanitizeText(err.Error())
	}
	return message
}

func (h *Handler) unwatchCommand(ctx context.Context, interaction Interaction) string {
	if h.watcher == nil {
		return "dashboard watcher가 설정되어 있지 않습니다."
	}
	message, err := h.watcher.UnwatchDashboard(ctx)
	if err != nil {
		return "dashboard watch 해제 실패: " + security.SanitizeText(err.Error())
	}
	return message
}

func (h *Handler) opsServiceCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	service, ok := security.NormalizeService(optionStringFromOptions(subcommand.Options, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	view := optionStringFromOptions(subcommand.Options, "view")
	if view == "" {
		view = "summary"
	}
	since := optionStringFromOptions(subcommand.Options, "since")
	if since == "" {
		since = "30m"
	}
	switch view {
	case "summary":
		return h.serviceCommand(ctx, interactionForCommand("service", withDefaultOptions(subcommand.Options, map[string]string{"since": since})))
	case "health":
		return withNext(formatting.FormatStatus([]formatting.ServiceStatus{h.health.Check(ctx, service)}), "/ops logs service:"+service+" mode:errors")
	case "copy":
		if service != "report" {
			return "copy view는 report service에서만 지원합니다."
		}
		return withNext(h.copyStatusCommand(ctx, interactionForCommand("copy-status", []ApplicationCommandOpt{stringInteractionOption("since", since)})), "/ops logs service:report mode:errors", "/ops service service:report view:summary")
	default:
		return "지원하지 않는 service view입니다. 상태는 `/ops service view:summary|health|copy`, 로그 분석은 `/ops logs`를 사용하세요."
	}
}

func (h *Handler) opsLogsCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	service, ok := security.NormalizeServiceOrAll(optionStringFromOptions(subcommand.Options, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	mode := optionStringFromOptions(subcommand.Options, "mode")
	if mode == "" {
		mode = "recent"
	}
	since := optionStringFromOptions(subcommand.Options, "since")
	if since == "" {
		since = "30m"
	}
	level := optionStringFromOptions(subcommand.Options, "level")
	if level == "" {
		level = "ERROR"
	}
	limit := optionStringFromOptions(subcommand.Options, "limit")
	if limit == "" {
		limit = "20"
	}
	switch mode {
	case "recent":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.logsCommand(ctx, interactionForCommand("logs", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("level", level),
		})), "/ops logs service:"+service+" mode:errors", "/ops service service:"+service)
	case "errors":
		if service == "all" && !sinceAllowsAllQuery(since) {
			return allServiceSinceGuardMessage()
		}
		next := []string{"/ops dashboard since:" + since}
		if service != "all" {
			next = append(next, "/ops logs service:"+service+" mode:recent level:ERROR", "/ops service service:"+service)
		}
		return withNext(h.errorsCommand(ctx, interactionForCommand("errors", []ApplicationCommandOpt{
			stringInteractionOption("since", since),
			stringInteractionOption("service", service),
		})), next...)
	case "top":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.topCommand(ctx, interactionForCommand("top", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("by", "path"),
		})), "/ops logs service:"+service+" mode:errors", "/ops trace trace_id:<traceId>")
	case "slow":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.slowCommand(ctx, interactionForCommand("slow", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("limit", limit),
		})), "/ops logs service:"+service+" mode:errors", "/ops trace trace_id:<traceId>")
	default:
		return "지원하지 않는 logs mode입니다."
	}
}

func (h *Handler) opsAlarmsCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	state := optionStringFromOptions(subcommand.Options, "state")
	if state == "" {
		state = "ALARM"
	}
	names, err := h.alarms.AlarmNamesByState(ctx, state)
	if err != nil {
		return "CloudWatch alarm 조회 실패: " + security.SanitizeText(err.Error())
	}
	service := optionStringFromOptions(subcommand.Options, "service")
	if service != "" {
		normalized, ok := security.NormalizeService(service)
		if !ok {
			return "지원하지 않는 service입니다."
		}
		names = filterAlarmNames(normalized, names)
	}
	return formatting.FormatAlarms(names)
}

func (h *Handler) dashboardCommand(ctx context.Context, interaction Interaction) string {
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	healthRows := h.health.CheckAll(ctx, serviceOrder)
	healthByService := make(map[string]formatting.ServiceStatus, len(healthRows))
	for _, row := range healthRows {
		healthByService[row.Service] = row
	}
	alarmNames, err := h.alarms.AlarmNames(ctx)
	if err != nil {
		alarmNames = nil
	}
	registry := h.cfg.ServiceRegistry
	if len(registry) == 0 {
		registry = config.BuildServiceRegistry(h.cfg.LogGroups, h.cfg.HealthURLs)
	}
	inputs := make([]formatting.DashboardServiceInput, 0, len(registry))
	for _, service := range registry {
		input := formatting.DashboardServiceInput{
			Service:     service.Name,
			DisplayName: service.DisplayName,
			Health:      healthByService[service.Name],
			Alarm:       serviceHasAlarm(service.Name, alarmNames),
		}
		if !service.Enabled {
			input.Health = formatting.ServiceStatus{Service: service.Name, State: "NOT_CONFIGURED", Detail: "health URL and log group are not configured"}
			input.LogStatus = "NOT_CONFIGURED"
			inputs = append(inputs, input)
			continue
		}
		if strings.TrimSpace(service.LogGroup) == "" {
			input.LogStatus = "NOT_CONFIGURED"
			inputs = append(inputs, input)
			continue
		}
		query, err := cw.BuildDashboardSummaryQuery(service.Name)
		if err != nil {
			input.LogStatus = "LOG_QUERY_FAILED"
			inputs = append(inputs, input)
			continue
		}
		rows, err := h.logs.Query(ctx, []string{service.LogGroup}, query, since, 100)
		if err != nil {
			input.LogStatus = "LOG_QUERY_FAILED"
			inputs = append(inputs, input)
			continue
		}
		input.Rows = rows
		if len(rows) == 0 {
			input.LogStatus = "NO_LOGS"
		} else {
			input.LogStatus = "OK"
		}
		inputs = append(inputs, input)
	}
	return formatting.FormatDashboard(sinceLabel, inputs, alarmNames)
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
	topQuery, err := cw.BuildTopQuery(service, "path")
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

func (h *Handler) countCommand(ctx context.Context, interaction Interaction) string {
	service, ok := security.NormalizeServiceOrAll(optionString(interaction, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	countType, ok := security.NormalizeCountType(optionString(interaction, "type"))
	if !ok {
		return "지원하지 않는 type 값입니다."
	}
	if service == "all" {
		return allServiceGuardMessage()
	}
	groups, err := cw.LogGroupsForOptionalService(h.cfg.LogGroups, service, h.cfg.CloudWatchMaxLogGroups)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	query, err := cw.BuildCountQuery(service, countType)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, 50)
	if err != nil {
		return "CloudWatch count 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatCountSummary(service, sinceLabel, countType, rows)
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
	query, err := cw.BuildTopQuery(service, by)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, 10)
	if err != nil {
		return "CloudWatch top 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatTopSummary(service, sinceLabel, by, rows)
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
	return formatting.FormatSlowSummary(service, sinceLabel, rows)
}

func (h *Handler) copyStatusCommand(ctx context.Context, interaction Interaction) string {
	sinceLabel := optionString(interaction, "since")
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	groups, err := cw.LogGroupsForService(h.cfg.LogGroups, "report")
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, cw.BuildCopyStatusQuery(), since, 100)
	if err != nil {
		return "CloudWatch copy-status 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatCopyStatus(sinceLabel, rows)
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
	query, err := cw.BuildRecentLogsQuery(service, level, h.cfg.CloudWatchQueryLimit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, security.ClampLimit(h.cfg.CloudWatchQueryLimit, 20, 100))
	if err != nil {
		return "CloudWatch logs 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatLogRows("최근 로그", rows)
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
	rows, err := h.logs.Query(ctx, groups, cw.BuildErrorsQuery(service, h.cfg.CloudWatchQueryLimit), since, security.ClampLimit(h.cfg.CloudWatchQueryLimit, 20, 100))
	if err != nil {
		return "CloudWatch errors 조회 실패: " + security.SanitizeText(err.Error())
	}
	return formatting.FormatErrors(rows)
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

func (h *Handler) retentionCommand(ctx context.Context, title string) string {
	groups, err := h.logs.DescribeGroups(ctx, cw.RetentionTargetLogGroups(h.cfg.LogGroups))
	if err != nil {
		return "CloudWatch log group 조회 실패: " + security.SanitizeText(err.Error())
	}
	rows := make([]formatting.LogGroupRetention, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, formatting.LogGroupRetention{
			Name:          group.Name,
			RetentionDays: group.RetentionDays,
			StoredBytes:   group.StoredBytes,
		})
	}
	return formatting.FormatRetention(title, rows)
}

func (h *Handler) commandTimeout(command string) time.Duration {
	switch command {
	case "dashboard", "ops":
		return h.cfg.CloudWatchQueryTimeout*time.Duration(len(serviceOrder)) + 8*time.Second
	case "service":
		return h.cfg.CloudWatchQueryTimeout*3 + 5*time.Second
	default:
		return h.cfg.CloudWatchQueryTimeout + 3*time.Second
	}
}

func opsSubcommand(interaction Interaction) (ApplicationCommandOpt, bool) {
	if len(interaction.Data.Options) == 0 {
		return ApplicationCommandOpt{}, false
	}
	return interaction.Data.Options[0], true
}

func interactionForCommand(name string, options []ApplicationCommandOpt) Interaction {
	return Interaction{Data: ApplicationCommandData{Name: name, Options: options}}
}

func withDefaultOptions(options []ApplicationCommandOpt, defaults map[string]string) []ApplicationCommandOpt {
	result := make([]ApplicationCommandOpt, 0, len(options)+len(defaults))
	seen := make(map[string]struct{}, len(options))
	for _, option := range options {
		result = append(result, option)
		seen[option.Name] = struct{}{}
	}
	for name, value := range defaults {
		if _, ok := seen[name]; ok {
			continue
		}
		result = append(result, stringInteractionOption(name, value))
	}
	return result
}

func stringInteractionOption(name, value string) ApplicationCommandOpt {
	encoded, _ := json.Marshal(value)
	return ApplicationCommandOpt{Name: name, Value: encoded}
}

func optionStringFromOptions(options []ApplicationCommandOpt, name string) string {
	return optionString(Interaction{Data: ApplicationCommandData{Options: options}}, name)
}

func legacyNotice(replacement string) string {
	return "Tip: use `" + replacement + "`\n\n"
}

func legacyNoticeFor(command string) string {
	replacement, ok := legacyOpsReplacement(command)
	if !ok {
		return ""
	}
	return legacyNotice(replacement)
}

func legacyOpsReplacement(command string) (string, bool) {
	replacements := map[string]string{
		"dashboard":   "/ops dashboard",
		"service":     "/ops service",
		"count":       "/ops logs mode:errors",
		"top":         "/ops logs mode:top",
		"slow":        "/ops logs mode:slow",
		"copy-status": "/ops service service:report view:copy",
		"status":      "/ops dashboard",
		"health":      "/ops service view:health",
		"logs":        "/ops logs mode:recent",
		"errors":      "/ops logs mode:errors",
		"trace":       "/ops trace",
		"alarm":       "/ops alarms",
		"disk":        "/ops storage view:usage",
		"retention":   "/ops storage view:retention",
	}
	replacement, ok := replacements[command]
	return replacement, ok
}

func withNext(content string, commands ...string) string {
	visible := make([]string, 0, 3)
	for _, command := range commands {
		if trimmed := strings.TrimSpace(command); trimmed != "" {
			visible = append(visible, trimmed)
		}
		if len(visible) == 3 {
			break
		}
	}
	if len(visible) == 0 {
		return content
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(content, "\n"))
	b.WriteString("\n\nNext:\n")
	for _, command := range visible {
		b.WriteString("- `")
		b.WriteString(command)
		b.WriteString("`\n")
	}
	return formatting.TruncateDiscordMessage(b.String())
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

func serviceHasAlarm(service string, alarmNames []string) bool {
	for _, name := range alarmNames {
		normalized := strings.ToLower(name)
		if strings.Contains(normalized, service) || (service == "post" && strings.Contains(normalized, "blog")) {
			return true
		}
	}
	return false
}

func filterAlarmNames(service string, alarmNames []string) []string {
	filtered := make([]string, 0, len(alarmNames))
	for _, name := range alarmNames {
		if serviceHasAlarm(service, []string{name}) {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func parseWatchInterval(value string) (time.Duration, bool) {
	switch strings.TrimSpace(value) {
	case "5m":
		return 5 * time.Minute, true
	case "10m":
		return 10 * time.Minute, true
	case "15m":
		return 15 * time.Minute, true
	default:
		return 0, false
	}
}

func optionString(interaction Interaction, name string) string {
	for _, option := range interaction.Data.Options {
		if option.Name != name {
			continue
		}
		var value string
		if err := json.Unmarshal(option.Value, &value); err == nil {
			return strings.TrimSpace(value)
		}
		var number int
		if err := json.Unmarshal(option.Value, &number); err == nil {
			return strconv.Itoa(number)
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
