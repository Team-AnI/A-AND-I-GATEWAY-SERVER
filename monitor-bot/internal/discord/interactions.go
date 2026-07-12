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
	ConfigureAlert(ctx context.Context, channelID, action, roleID, target string) (string, error)
	WatchLogFeed(ctx context.Context, channelID, service, mode, since string, interval time.Duration, limit int) (string, error)
	UnwatchLogFeed(ctx context.Context, service, mode string) (string, error)
	ListLogFeeds(ctx context.Context) string
	DescribeAssignmentDiagnosis(courseSlug string, assignment reportadmin.Assignment) string
	AssignmentIssueStatus(courseSlug, assignmentID string) string
	AssignmentIssueHistory(courseSlug, assignmentID string) string
	AcknowledgeAssignmentIssue(courseSlug, assignmentID, eventSlug, until, reason, actor string) (string, error)
	UnacknowledgeAssignmentIssue(courseSlug, assignmentID, eventSlug string) (string, error)
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
	Name          string                  `json:"name"`
	Options       []ApplicationCommandOpt `json:"options,omitempty"`
	CustomID      string                  `json:"custom_id,omitempty"`
	ComponentType int                     `json:"component_type,omitempty"`
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
	if interaction.Type != interactionTypeApplicationCommand && interaction.Type != interactionTypeMessageComponent {
		writeJSON(w, messageResponse("지원하지 않는 interaction type입니다.", h.cfg.DiscordEphemeralResponses))
		return
	}
	if err := h.authorize(interaction); err != nil {
		writeJSON(w, messageResponse(err.Error(), true))
		return
	}
	if interaction.Type == interactionTypeMessageComponent && interaction.Data.ComponentType != componentTypeButton {
		writeJSON(w, messageResponse("지원하지 않는 버튼입니다. 메시지의 fallback 명령어를 사용하세요.", true))
		return
	}

	ephemeral := h.cfg.DiscordEphemeralResponses
	if interaction.Type == interactionTypeMessageComponent {
		ephemeral = true
	}
	writeJSON(w, deferredResponse(ephemeral))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), h.interactionTimeout(interaction))
		defer cancel()
		content := h.executeInteraction(ctx, interaction)
		if err := SendFollowUp(ctx, h.httpClient, h.cfg.DiscordApplicationID, interaction.Token, content, ephemeral); err != nil {
			log.Printf("discord follow-up failed: %v", err)
		}
	}()
}

func (h *Handler) executeInteraction(ctx context.Context, interaction Interaction) string {
	if interaction.Type == interactionTypeMessageComponent {
		return h.executeComponent(ctx, interaction)
	}
	return h.execute(ctx, interaction)
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

func (h *Handler) executeComponent(ctx context.Context, interaction Interaction) string {
	action, err := parseOpsButtonCustomID(interaction.Data.CustomID)
	if err != nil {
		return "지원하지 않는 버튼입니다. 메시지의 fallback 명령어를 사용하세요."
	}
	return h.executeButtonAction(ctx, interaction, action)
}

func (h *Handler) executeButtonAction(ctx context.Context, interaction Interaction, action opsButtonAction) string {
	switch action.Kind {
	case "trace":
		return h.opsLogsCommand(ctx, interaction, ApplicationCommandOpt{Options: []ApplicationCommandOpt{
			stringInteractionOption("mode", "trace"),
			stringInteractionOption("query", action.TraceID),
		}})
	case "logs":
		return h.opsLogsCommand(ctx, interaction, ApplicationCommandOpt{Options: []ApplicationCommandOpt{
			stringInteractionOption("service", action.Service),
			stringInteractionOption("mode", action.Mode),
			stringInteractionOption("since", action.Since),
			stringInteractionOption("limit", strconv.Itoa(action.Limit)),
		}})
	default:
		return "지원하지 않는 버튼입니다. 메시지의 fallback 명령어를 사용하세요."
	}
}

func (h *Handler) opsCommand(ctx context.Context, interaction Interaction) string {
	subcommand, ok := opsSubcommand(interaction)
	if !ok {
		return formatting.HelpText()
	}
	switch subcommand.Name {
	case "dashboard":
		return h.opsDashboardCommand(ctx, interaction, subcommand)
	case "logs":
		return h.opsLogsCommand(ctx, interaction, subcommand)
	case "alert":
		return h.opsAlertCommand(ctx, interaction, subcommand)
	case "assignment":
		return h.assignmentCommand(ctx, interactionForCommand("assignment", subcommand.Options))
	case "help":
		return formatting.HelpTextFor(optionStringFromOptions(subcommand.Options, "topic"), optionStringFromOptions(subcommand.Options, "command"), optionStringFromOptions(subcommand.Options, "query"))
	case "service":
		return "deprecated: /ops dashboard service:<service> 를 사용하세요."
	case "trace":
		return "deprecated: /ops logs mode:trace query:<traceId> 를 사용하세요."
	case "watch", "unwatch", "watches":
		return "deprecated: /ops dashboard action:<watch|unwatch|status> 를 사용하세요."
	case "logs-watch", "logs-unwatch", "logs-watches":
		return "deprecated: /ops logs action:<watch|unwatch|watches> 를 사용하세요."
	case "assignments", "assignments-all", "assignment-check", "assignment-events", "assignment-ack", "assignment-unack", "submissions":
		return "deprecated: /ops assignment action:<list|check|submissions|ack|unack> 또는 view:events 를 사용하세요."
	default:
		return "지원하지 않는 /ops subcommand입니다. /ops help 를 확인하세요."
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

func (h *Handler) assignmentEventsCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	_ = ctx
	courseSlug := strings.TrimSpace(optionStringFromOptions(subcommand.Options, "course"))
	assignmentID := strings.TrimSpace(optionStringFromOptions(subcommand.Options, "id"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	return h.ops.AssignmentIssueHistory(courseSlug, assignmentID)
}

func (h *Handler) assignmentAckCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	_ = ctx
	courseSlug := strings.TrimSpace(optionStringFromOptions(subcommand.Options, "course"))
	assignmentID := strings.TrimSpace(optionStringFromOptions(subcommand.Options, "id"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	result, err := h.ops.AcknowledgeAssignmentIssue(
		courseSlug,
		assignmentID,
		optionStringFromOptions(subcommand.Options, "event"),
		optionStringFromOptions(subcommand.Options, "until"),
		optionStringFromOptions(subcommand.Options, "reason"),
		"discord",
	)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) assignmentUnackCommand(ctx context.Context, subcommand ApplicationCommandOpt) string {
	_ = ctx
	courseSlug := strings.TrimSpace(optionStringFromOptions(subcommand.Options, "course"))
	assignmentID := strings.TrimSpace(optionStringFromOptions(subcommand.Options, "id"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	result, err := h.ops.UnacknowledgeAssignmentIssue(courseSlug, assignmentID, optionStringFromOptions(subcommand.Options, "event"))
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
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

func (h *Handler) interactionTimeout(interaction Interaction) time.Duration {
	if interaction.Type == interactionTypeMessageComponent {
		return h.commandTimeout("ops")
	}
	return h.commandTimeout(interaction.Data.Name)
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
