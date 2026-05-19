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
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const (
	interactionTypePing               = 1
	interactionTypeApplicationCommand = 2
	maxBodyBytes                      = 64 * 1024
)

var serviceOrder = []string{"gateway", "auth", "report", "post", "online-judge"}

type Handler struct {
	cfg          config.Config
	health       *health.Client
	logs         *cw.LogsClient
	alarms       *cw.AlarmClient
	reportAdmin  *reportadmin.Client
	ops          OpsController
	httpClient   *http.Client
	replayWindow time.Duration
}

type OpsController interface {
	WatchDashboardScope(ctx context.Context, channelID, scope, service string, interval time.Duration) (string, error)
	UnwatchDashboardScope(ctx context.Context, scope, service string) (string, error)
	ListDashboardWatches(ctx context.Context) string
	ConfigureAlert(ctx context.Context, channelID, action, roleID string) (string, error)
	WatchLogFeed(ctx context.Context, channelID, service, mode, since string, interval time.Duration, limit int) (string, error)
	UnwatchLogFeed(ctx context.Context, service, mode string) (string, error)
	ListLogFeeds(ctx context.Context) string
}

type Interaction struct {
	ID            string                 `json:"id"`
	ApplicationID string                 `json:"application_id"`
	ChannelID     string                 `json:"channel_id"`
	Type          int                    `json:"type"`
	Token         string                 `json:"token"`
	GuildID       string                 `json:"guild_id"`
	Member        *Member                `json:"member,omitempty"`
	Data          ApplicationCommandData `json:"data"`
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
		reportAdmin:  reportadmin.NewClientWithRefresh(cfg.ReportServiceURI, cfg.AuthServiceURI, cfg.OpsAdminRefreshToken, cfg.HealthRequestTimeout),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		replayWindow: 5 * time.Minute,
	}
}

func (h *Handler) SetOpsController(controller OpsController) {
	h.ops = controller
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
		return h.dashboardCommand(ctx, interactionForCommand("dashboard", withDefaultOptions(subcommand.Options, map[string]string{"since": "30m"})))
	case "service":
		return h.opsServiceCommand(ctx, subcommand)
	case "logs":
		return h.opsLogsCommand(ctx, subcommand)
	case "watch":
		return h.opsWatchCommand(ctx, interaction, subcommand)
	case "unwatch":
		return h.opsUnwatchCommand(ctx, subcommand)
	case "watches":
		return h.opsWatchesCommand(ctx)
	case "alert":
		return h.opsAlertCommand(ctx, interaction, subcommand)
	case "logs-watch":
		return h.opsLogsWatchCommand(ctx, interaction, subcommand)
	case "logs-unwatch":
		return h.opsLogsUnwatchCommand(ctx, subcommand)
	case "logs-watches":
		return h.opsLogsWatchesCommand(ctx)
	case "assignments":
		return h.assignmentsCommand(ctx, interactionForCommand("assignments", withDefaultOptions(subcommand.Options, map[string]string{"status": "all"})))
	case "assignments-all":
		return h.assignmentsAllCommand(ctx, interactionForCommand("assignments-all", withDefaultOptions(subcommand.Options, map[string]string{"window": "today"})))
	case "assignment":
		return h.assignmentCommand(ctx, interactionForCommand("assignment", subcommand.Options))
	case "assignment-check":
		return h.assignmentCheckCommand(ctx, interactionForCommand("assignment-check", subcommand.Options))
	case "submissions":
		return h.submissionsCommand(ctx, interactionForCommand("submissions", subcommand.Options))
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

func (h *Handler) opsWatchCommand(ctx context.Context, interaction Interaction, subcommand ApplicationCommandOpt) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	scope := optionStringFromOptions(subcommand.Options, "scope")
	service := optionStringFromOptions(subcommand.Options, "service")
	channelID := firstNonEmpty(optionStringFromOptions(subcommand.Options, "channel"), interaction.ChannelID)
	interval, ok := parseOpsInterval(optionStringFromOptions(subcommand.Options, "interval"), 5*time.Minute)
	if !ok {
		return "지원하지 않는 interval입니다. 1m, 3m, 5m, 10m, 15m 중 하나를 사용하세요."
	}
	result, err := h.ops.WatchDashboardScope(ctx, channelID, scope, service, interval)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) opsUnwatchCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	scope := optionStringFromOptions(subcommand.Options, "scope")
	service := optionStringFromOptions(subcommand.Options, "service")
	result, err := h.ops.UnwatchDashboardScope(ctx, scope, service)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) opsWatchesCommand(ctx context.Context) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	return h.ops.ListDashboardWatches(ctx)
}

func (h *Handler) opsAlertCommand(ctx context.Context, interaction Interaction, subcommand ApplicationCommandOpt) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	action := optionStringFromOptions(subcommand.Options, "action")
	roleID := optionStringFromOptions(subcommand.Options, "role")
	channelID := firstNonEmpty(optionStringFromOptions(subcommand.Options, "channel"), interaction.ChannelID)
	result, err := h.ops.ConfigureAlert(ctx, channelID, action, roleID)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) opsLogsWatchCommand(ctx context.Context, interaction Interaction, subcommand ApplicationCommandOpt) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	service := optionStringFromOptions(subcommand.Options, "service")
	mode := optionStringFromOptions(subcommand.Options, "mode")
	channelID := firstNonEmpty(optionStringFromOptions(subcommand.Options, "channel"), interaction.ChannelID)
	since := optionStringFromOptions(subcommand.Options, "since")
	if since == "" {
		since = "30m"
	}
	interval, ok := parseOpsInterval(optionStringFromOptions(subcommand.Options, "interval"), 5*time.Minute)
	if !ok {
		return "지원하지 않는 interval입니다. 3m, 5m, 10m, 15m 중 하나를 사용하세요."
	}
	limit := parseOpsLimit(optionStringFromOptions(subcommand.Options, "limit"), 10)
	result, err := h.ops.WatchLogFeed(ctx, channelID, service, mode, since, interval, limit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) opsLogsUnwatchCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	service := optionStringFromOptions(subcommand.Options, "service")
	mode := optionStringFromOptions(subcommand.Options, "mode")
	result, err := h.ops.UnwatchLogFeed(ctx, service, mode)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) opsLogsWatchesCommand(ctx context.Context) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	return h.ops.ListLogFeeds(ctx)
}

func (h *Handler) opsServiceCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	service, ok := security.NormalizeService(optionStringFromOptions(subcommand.Options, "service"))
	if !ok {
		return "지원하지 않는 service입니다."
	}
	if !isOpsV2Service(service) {
		return "status: NO_V2_LOG\nservice: " + opsDisplayServiceName(service) + "\nkey findings: V2 로그 연동 전까지 장애 판단 대상이 아닙니다.\nrecommended next commands:\n- `/ops dashboard since:30m`"
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
		return withNext(formatting.FormatStatus([]formatting.ServiceStatus{h.health.Check(ctx, service)}), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors")
	default:
		return "지원하지 않는 service view입니다. 상태는 `/ops service view:summary|health`, 로그 분석은 `/ops logs`를 사용하세요."
	}
}

func (h *Handler) opsLogsCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	serviceOption := optionStringFromOptions(subcommand.Options, "service")
	if serviceOption == "" {
		serviceOption = "all"
	}
	service, ok := security.NormalizeServiceOrAll(serviceOption)
	if !ok {
		return "지원하지 않는 service입니다."
	}
	if service != "all" && !isOpsV2Service(service) {
		return "status: NO_V2_LOG\nservice: " + opsDisplayServiceName(service) + "\nkey findings: V2 로그 연동 전까지 장애 판단 대상이 아닙니다.\nrecommended next commands:\n- `/ops logs service:report mode:errors since:30m limit:10`"
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
	limit := parseOpsLimit(optionStringFromOptions(subcommand.Options, "limit"), h.cfg.CloudWatchQueryLimit)
	switch mode {
	case "recent":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.logsCommand(ctx, interactionForCommand("logs", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("level", level),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops service service:"+opsDisplayServiceName(service))
	case "errors":
		if service == "all" && !sinceAllowsAllQuery(since) {
			return allServiceSinceGuardMessage()
		}
		next := []string{"/ops dashboard since:" + since}
		if service != "all" {
			next = append(next, "/ops logs service:"+opsDisplayServiceName(service)+" mode:recent level:ERROR", "/ops service service:"+opsDisplayServiceName(service))
		}
		return withNext(h.errorsCommand(ctx, interactionForCommand("errors", []ApplicationCommandOpt{
			stringInteractionOption("since", since),
			stringInteractionOption("service", service),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), next...)
	case "top":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.topCommand(ctx, interactionForCommand("top", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("by", "path"),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops trace trace_id:<traceId>")
	case "slow":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.slowCommand(ctx, interactionForCommand("slow", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops trace trace_id:<traceId>")
	case "security":
		if service == "all" {
			return allServiceGuardMessage()
		}
		return withNext(h.securityLogsCommand(ctx, interactionForCommand("security", []ApplicationCommandOpt{
			stringInteractionOption("service", service),
			stringInteractionOption("since", since),
			stringInteractionOption("limit", strconv.Itoa(limit)),
		})), "/ops logs service:"+opsDisplayServiceName(service)+" mode:errors", "/ops trace trace_id:<traceId>")
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
			Health:      formatting.ServiceStatus{Service: service.Name, State: "UNKNOWN", Detail: "not connected in service ops phase"},
			Alarm:       serviceHasAlarm(service.Name, alarmNames),
		}
		if !isOpsV2Service(service.Name) {
			input.Health = formatting.ServiceStatus{Service: service.Name, State: "UNKNOWN", Detail: "not connected for V2 logs"}
			input.LogStatus = "NO_V2_LOG"
			inputs = append(inputs, input)
			continue
		}
		input.Health = h.health.Check(ctx, service.Name)
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
			input.LogStatus = "NO_V2_LOG"
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

func (h *Handler) assignmentsCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, "", "올바르지 않은 courseSlug입니다.")
	}
	statusFilter, ok := security.NormalizeAssignmentStatus(optionString(interaction, "status"))
	if !ok {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, "", "지원하지 않는 status 값입니다.")
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	assignments, err := h.reportAdmin.ListAssignments(ctx, courseSlug)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, "", err.Error()), notice)
		}
		return h.assignmentLogFallback(ctx, "Assignments", courseSlug, "", status, err)
	}
	return formatting.WithAdminNotice(formatting.FormatAdminAssignments(courseSlug, statusFilter, reportadmin.FilterAssignments(assignments, statusFilter)), notice)
}

func (h *Handler) assignmentsAllCommand(ctx context.Context, interaction Interaction) string {
	window, ok := security.NormalizeAssignmentWindow(optionString(interaction, "window"))
	if !ok {
		return formatting.FormatAdminError(reportadmin.StatusError, "", "", "지원하지 않는 window 값입니다.")
	}
	courses, err := h.reportAdmin.ListCourses(ctx)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			return formatting.FormatAdminError(status, "", "", err.Error())
		}
		return h.assignmentLogFallback(ctx, "Assignments all", "", "", status, err)
	}
	summaries := make([]formatting.AdminAssignmentsAllSummary, 0, len(courses))
	totalShown := 0
	for _, course := range courses {
		if len(summaries) >= 8 || totalShown >= 20 {
			break
		}
		assignments, err := h.reportAdmin.ListAssignments(ctx, course.Slug)
		summary := formatting.AdminAssignmentsAllSummary{CourseSlug: course.Slug}
		if err != nil {
			summary.Error = reportadmin.StatusOf(err)
			summaries = append(summaries, summary)
			continue
		}
		filtered := filterAssignmentsByWindow(assignments, window)
		counts := assignmentStatusCounts(filtered)
		summary.Total = len(filtered)
		summary.Published = counts["published"]
		summary.Scheduled = counts["scheduled"]
		summary.Draft = counts["draft"]
		for _, assignment := range filtered {
			if len(summary.Shown) >= 3 || totalShown >= 20 {
				break
			}
			summary.Shown = append(summary.Shown, assignment)
			totalShown++
		}
		summaries = append(summaries, summary)
	}
	return formatting.FormatAdminAssignmentsAll(window, summaries)
}

func (h *Handler) assignmentCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	assignmentID := strings.TrimSpace(optionString(interaction, "id"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	assignment, err := h.reportAdmin.GetAssignment(ctx, courseSlug, assignmentID)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			finding := err.Error()
			if status == reportadmin.StatusNoData {
				finding = "no matching records"
			}
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, assignmentID, finding), notice)
		}
		return h.assignmentLogFallback(ctx, "Assignment", courseSlug, assignmentID, status, err)
	}
	return formatting.WithAdminNotice(formatting.FormatAdminAssignment(courseSlug, assignment), notice)
}

func (h *Handler) assignmentCheckCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	assignmentID := strings.TrimSpace(optionString(interaction, "id"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	assignment, err := h.reportAdmin.GetAssignment(ctx, courseSlug, assignmentID)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			finding := err.Error()
			if status == reportadmin.StatusNoData {
				finding = "no matching records"
			}
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, assignmentID, finding), notice)
		}
		return h.assignmentLogFallback(ctx, "Assignment check", courseSlug, assignmentID, status, err)
	}
	return formatting.WithAdminNotice(formatting.FormatAdminAssignmentCheck(courseSlug, assignment, reportadmin.CheckAssignment(assignment)), notice)
}

func (h *Handler) submissionsCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	assignmentID := strings.TrimSpace(optionString(interaction, "assignment"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	summary, err := h.reportAdmin.SubmissionStatuses(ctx, courseSlug, assignmentID)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			finding := err.Error()
			if status == reportadmin.StatusNoData {
				finding = "no matching records"
			}
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, assignmentID, finding), notice)
		}
		return h.assignmentLogFallback(ctx, "Submissions", courseSlug, assignmentID, status, err)
	}
	return formatting.WithAdminNotice(formatting.FormatAdminSubmissions(courseSlug, assignmentID, summary), notice)
}

func (h *Handler) assignmentLogFallback(ctx context.Context, title, courseSlug, assignmentID, status string, cause error) string {
	if assignmentID != "" && security.ValidateAssignmentID(assignmentID) {
		groups, groupErr := cw.LogGroupsForService(h.cfg.LogGroups, "report")
		if groupErr == nil {
			query, queryErr := cw.BuildAssignmentQuery(assignmentID)
			if queryErr == nil {
				rows, queryRunErr := h.logs.Query(ctx, groups, query, 3*time.Hour, 20)
				if queryRunErr == nil {
					return formatting.FormatCloudWatchFallback(title, rows)
				}
			}
		}
	}
	prefix := "WEB_ADMIN_API " + status + ": " + security.SanitizeText(cause.Error()) + ". "
	return formatting.FormatAdminError(status, courseSlug, assignmentID, prefix+"CloudWatch fallback result, not authoritative; 상세 로그는 `/ops logs service:report mode:errors since:30m limit:10`로 확인하세요.")
}

func shouldUseCloudWatchFallback(status string) bool {
	switch status {
	case reportadmin.StatusUpstreamError, reportadmin.StatusTimeout, reportadmin.StatusInvalidResponse:
		return true
	default:
		return false
	}
}

func (h *Handler) courseManualNotice(ctx context.Context, courseSlug string) string {
	courses, err := h.reportAdmin.ListCourses(ctx)
	if err != nil {
		return ""
	}
	now := time.Now()
	for _, course := range courses {
		if course.Slug != courseSlug {
			continue
		}
		switch classifyManualCourse(course, now) {
		case "LEGACY":
			return "참고: 이 코스는 레거시/종료 코스로 보입니다. 자동 feed 대상은 아니며 수동 조회 결과입니다."
		case "UNKNOWN":
			return "참고: 이 코스는 운영 상태 판단 필드가 부족해 UNKNOWN으로 보입니다. 자동 이벤트 발송은 제한됩니다."
		default:
			return ""
		}
	}
	return ""
}

func classifyManualCourse(course reportadmin.Course, now time.Time) string {
	status := strings.ToUpper(strings.TrimSpace(course.Status))
	switch status {
	case "CLOSED", "ARCHIVED", "ENDED", "LEGACY", "INACTIVE":
		return "LEGACY"
	}
	if end, ok := parseRFC3339(course.EndAt); ok && now.After(end) {
		return "LEGACY"
	}
	if strings.TrimSpace(course.Status) == "" && strings.TrimSpace(course.StartAt) == "" && strings.TrimSpace(course.EndAt) == "" {
		return "UNKNOWN"
	}
	return "ACTIVE"
}

func filterAssignmentsByWindow(assignments []reportadmin.Assignment, window string) []reportadmin.Assignment {
	normalized := strings.TrimSpace(window)
	if normalized == "" {
		return assignments
	}
	now := time.Now()
	var start, end time.Time
	switch normalized {
	case "today":
		kst := time.FixedZone("KST", 9*60*60)
		nowKST := now.In(kst)
		start = time.Date(nowKST.Year(), nowKST.Month(), nowKST.Day(), 0, 0, 0, 0, kst)
		end = start.Add(24 * time.Hour)
	case "this-week":
		kst := time.FixedZone("KST", 9*60*60)
		nowKST := now.In(kst)
		start = time.Date(nowKST.Year(), nowKST.Month(), nowKST.Day(), 0, 0, 0, 0, kst)
		end = now.Add(7 * 24 * time.Hour)
	default:
		return assignments
	}
	filtered := make([]reportadmin.Assignment, 0, len(assignments))
	for _, assignment := range assignments {
		for _, candidate := range []string{assignment.StartAt, assignment.EndAt, assignment.PublishedAt, assignment.UpdatedAt} {
			parsed, ok := parseRFC3339(candidate)
			if ok && (parsed.Equal(start) || parsed.After(start)) && parsed.Before(end) {
				filtered = append(filtered, assignment)
				break
			}
		}
	}
	return filtered
}

func assignmentStatusCounts(assignments []reportadmin.Assignment) map[string]int {
	counts := map[string]int{"published": 0, "scheduled": 0, "draft": 0}
	for _, assignment := range assignments {
		normalized := strings.ToLower(strings.TrimSpace(assignment.Status))
		if _, ok := counts[normalized]; ok {
			counts[normalized]++
		}
	}
	return counts
}

func parseRFC3339(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
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
	query, err := cw.BuildRecentLogsQuery(service, level, limit)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, query, since, int32(limit))
	if err != nil {
		return "CloudWatch logs 조회 실패: " + security.SanitizeText(err.Error())
	}
	return appendTraceNext(formatting.FormatLogRows("최근 로그", rows), rows)
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

func isOpsV2Service(service string) bool {
	return service == "gateway" || service == "auth" || service == "report" || service == "post"
}

func opsDisplayServiceName(service string) string {
	if service == "post" {
		return "blog"
	}
	return service
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
	trimmed := strings.TrimRight(content, "\n")
	b.WriteString(trimmed)
	if strings.Contains(trimmed, "\n\nNext:\n") {
		b.WriteByte('\n')
	} else {
		b.WriteString("\n\nNext:\n")
	}
	for _, command := range visible {
		b.WriteString("- `")
		b.WriteString(command)
		b.WriteString("`\n")
	}
	return formatting.TruncateDiscordMessage(b.String())
}

func appendTraceNext(content string, rows []map[string]string) string {
	traceID := firstTraceID(rows)
	if traceID == "" {
		return content
	}
	return withNext(content, "/ops trace trace_id:"+traceID)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseOpsLimit(value string, fallback int) int {
	switch strings.TrimSpace(value) {
	case "5":
		return 5
	case "10":
		return 10
	case "20":
		return 20
	case "":
		switch fallback {
		case 5, 10, 20:
			return fallback
		default:
			return 20
		}
	default:
		return 20
	}
}

func parseOpsInterval(value string, fallback time.Duration) (time.Duration, bool) {
	switch strings.TrimSpace(value) {
	case "":
		if fallback <= 0 {
			return 5 * time.Minute, true
		}
		return fallback, true
	case "1m":
		return time.Minute, true
	case "3m":
		return 3 * time.Minute, true
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
