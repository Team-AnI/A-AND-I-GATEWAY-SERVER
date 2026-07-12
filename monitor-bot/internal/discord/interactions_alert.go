package discord

import (
	"context"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

func (h *Handler) opsAlertCommand(ctx context.Context, interaction Interaction, subcommand ApplicationCommandOpt) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	action := optionStringFromOptions(subcommand.Options, "action")
	target := optionStringFromOptions(subcommand.Options, "target")
	roleID := optionStringFromOptions(subcommand.Options, "role")
	channelID := firstNonEmpty(optionStringFromOptions(subcommand.Options, "channel"), interaction.ChannelID)
	result, err := h.ops.ConfigureAlert(ctx, channelID, action, roleID, target)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}
