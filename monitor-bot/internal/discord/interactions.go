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
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value,omitempty"`
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
	case "dashboard":
		return h.dashboardCommand(ctx, interaction)
	case "watch":
		return h.watchCommand(ctx, interaction)
	case "unwatch":
		return h.unwatchCommand(ctx, interaction)
	case "service":
		return h.serviceCommand(ctx, interaction)
	case "count":
		return h.countCommand(ctx, interaction)
	case "top":
		return h.topCommand(ctx, interaction)
	case "slow":
		return h.slowCommand(ctx, interaction)
	case "copy-status":
		return h.copyStatusCommand(ctx, interaction)
	case "status":
		return formatting.FormatStatus(h.health.CheckAll(ctx, serviceOrder))
	case "health":
		service, ok := security.NormalizeService(optionString(interaction, "service"))
		if !ok {
			return "지원하지 않는 service입니다."
		}
		return formatting.FormatStatus([]formatting.ServiceStatus{h.health.Check(ctx, service)})
	case "logs":
		return h.logsCommand(ctx, interaction)
	case "errors":
		return h.errorsCommand(ctx, interaction)
	case "trace":
		return h.traceCommand(ctx, interaction)
	case "alarm":
		names, err := h.alarms.AlarmNames(ctx)
		if err != nil {
			return "CloudWatch alarm 조회 실패: " + security.SanitizeText(err.Error())
		}
		return formatting.FormatAlarms(names)
	case "disk":
		return h.retentionCommand(ctx, "💽 CloudWatch Log Usage")
	case "retention":
		return h.retentionCommand(ctx, "📦 CloudWatch Log Retention")
	case "help":
		return formatting.HelpText()
	default:
		return "지원하지 않는 명령어입니다. /help 를 확인하세요."
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
	since, ok := security.ParseSince(optionString(interaction, "since"))
	if !ok {
		return "지원하지 않는 since 값입니다."
	}
	groups, err := cw.LogGroupsForOptionalService(h.cfg.LogGroups, optionString(interaction, "service"), h.cfg.CloudWatchMaxLogGroups)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	rows, err := h.logs.Query(ctx, groups, cw.BuildErrorsQuery(optionString(interaction, "service"), h.cfg.CloudWatchQueryLimit), since, security.ClampLimit(h.cfg.CloudWatchQueryLimit, 20, 100))
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
	case "dashboard":
		return h.cfg.CloudWatchQueryTimeout*time.Duration(len(serviceOrder)) + 8*time.Second
	case "service":
		return h.cfg.CloudWatchQueryTimeout*3 + 5*time.Second
	default:
		return h.cfg.CloudWatchQueryTimeout + 3*time.Second
	}
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
