package discord

import (
	"context"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

func (h *Handler) opsDashboardCommand(ctx context.Context, interaction Interaction, subcommand ApplicationCommandOpt) string {
	action := optionStringFromOptions(subcommand.Options, "action")
	if action == "" {
		action = "view"
	}
	service := optionStringFromOptions(subcommand.Options, "service")
	since := optionStringFromOptions(subcommand.Options, "since")
	if since == "" {
		since = "30m"
	}
	switch action {
	case "view":
		if strings.TrimSpace(service) != "" {
			return h.opsServiceCommand(ctx, ApplicationCommandOpt{Options: withDefaultOptions(subcommand.Options, map[string]string{"since": since})})
		}
		return h.dashboardCommand(ctx, interactionForCommand("dashboard", withDefaultOptions(subcommand.Options, map[string]string{"since": since})))
	case "watch":
		scope := "all"
		if strings.TrimSpace(service) != "" {
			scope = "service"
		}
		options := withDefaultOptions(subcommand.Options, map[string]string{"scope": scope, "service": service})
		return h.opsWatchCommand(ctx, interaction, ApplicationCommandOpt{Options: options})
	case "unwatch":
		scope := "all"
		if strings.TrimSpace(service) != "" {
			scope = "service"
		}
		options := withDefaultOptions(subcommand.Options, map[string]string{"scope": scope, "service": service})
		return h.opsUnwatchCommand(ctx, ApplicationCommandOpt{Options: options})
	case "status":
		return h.opsWatchesCommand(ctx)
	default:
		return "지원하지 않는 dashboard action입니다. view, watch, unwatch, status 중 하나를 사용하세요."
	}
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
		return "지원하지 않는 dashboard service view입니다. 상태는 `/ops dashboard service:<service>`, 로그 분석은 `/ops logs`를 사용하세요."
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
