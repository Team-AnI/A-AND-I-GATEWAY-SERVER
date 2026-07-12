package discord

import (
	"context"
	"errors"
	"testing"
)

type alertOpsRecorder struct {
	OpsController
	call   *alertConfigureCall
	result string
	err    error
}

type alertConfigureCall struct {
	channelID string
	action    string
	roleID    string
	target    string
}

func (r *alertOpsRecorder) ConfigureAlert(
	_ context.Context,
	channelID, action, roleID, target string,
) (string, error) {
	r.call = &alertConfigureCall{
		channelID: channelID,
		action:    action,
		roleID:    roleID,
		target:    target,
	}
	return r.result, r.err
}

func TestOpsAlertUsesInteractionChannelAndForwardsOptions(t *testing.T) {
	ops := &alertOpsRecorder{result: "configured"}
	h := &Handler{ops: ops}

	got := h.opsAlertCommand(
		context.Background(),
		Interaction{ChannelID: "interaction-channel"},
		alertSubcommand(
			stringInteractionOption("action", "role"),
			stringInteractionOption("target", "critical"),
			stringInteractionOption("role", "operator-role"),
		),
	)

	if got != "configured" {
		t.Fatalf("unexpected result: %s", got)
	}
	want := alertConfigureCall{
		channelID: "interaction-channel",
		action:    "role",
		roleID:    "operator-role",
		target:    "critical",
	}
	if ops.call == nil || *ops.call != want {
		t.Fatalf("unexpected ConfigureAlert call: %#v", ops.call)
	}
}

func TestOpsAlertPrefersExplicitChannel(t *testing.T) {
	ops := &alertOpsRecorder{result: "configured"}
	h := &Handler{ops: ops}

	got := h.opsAlertCommand(
		context.Background(),
		Interaction{ChannelID: "interaction-channel"},
		alertSubcommand(
			stringInteractionOption("action", "channel"),
			stringInteractionOption("channel", "configured-channel"),
		),
	)

	if got != "configured" {
		t.Fatalf("unexpected result: %s", got)
	}
	want := alertConfigureCall{channelID: "configured-channel", action: "channel"}
	if ops.call == nil || *ops.call != want {
		t.Fatalf("unexpected ConfigureAlert call: %#v", ops.call)
	}
}

func TestOpsCommandDispatchesAlert(t *testing.T) {
	ops := &alertOpsRecorder{result: "alert status"}
	h := &Handler{ops: ops}
	interaction := Interaction{
		ChannelID: "interaction-channel",
		Data: ApplicationCommandData{
			Name: "ops",
			Options: []ApplicationCommandOpt{alertSubcommand(
				stringInteractionOption("action", "status"),
				stringInteractionOption("target", "all"),
			)},
		},
	}

	got := h.opsCommand(context.Background(), interaction)

	if got != "alert status" {
		t.Fatalf("unexpected dispatch result: %s", got)
	}
	want := alertConfigureCall{channelID: "interaction-channel", action: "status", target: "all"}
	if ops.call == nil || *ops.call != want {
		t.Fatalf("unexpected ConfigureAlert call: %#v", ops.call)
	}
}

func TestOpsAlertRequiresControllerAndSanitizesErrors(t *testing.T) {
	t.Run("missing controller", func(t *testing.T) {
		h := &Handler{}

		got := h.opsAlertCommand(context.Background(), Interaction{}, alertSubcommand())

		if got != "Service Ops controller is not ready." {
			t.Fatalf("unexpected missing controller result: %s", got)
		}
	})

	t.Run("controller error", func(t *testing.T) {
		ops := &alertOpsRecorder{err: errors.New("failed token=super-secret")}
		h := &Handler{ops: ops}

		got := h.opsAlertCommand(
			context.Background(),
			Interaction{},
			alertSubcommand(stringInteractionOption("action", "status")),
		)

		if got != "failed token=[REDACTED]" {
			t.Fatalf("unexpected sanitized error: %s", got)
		}
	})
}

func alertSubcommand(options ...ApplicationCommandOpt) ApplicationCommandOpt {
	return ApplicationCommandOpt{Name: "alert", Options: options}
}
