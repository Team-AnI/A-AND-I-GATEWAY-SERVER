package discord

import (
	"context"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

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
