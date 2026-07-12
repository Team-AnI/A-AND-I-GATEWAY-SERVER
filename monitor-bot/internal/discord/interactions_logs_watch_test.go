package discord

import (
	"context"
	"errors"
	"testing"
	"time"
)

type logsWatchOpsRecorder struct {
	OpsController
	watchCall     *logsWatchCall
	watchResult   string
	watchErr      error
	unwatchCall   *logsUnwatchCall
	unwatchResult string
	unwatchErr    error
	listCalls     int
	listResult    string
}

type logsWatchCall struct {
	channelID string
	service   string
	mode      string
	since     string
	interval  time.Duration
	limit     int
}

type logsUnwatchCall struct {
	service string
	mode    string
}

func (r *logsWatchOpsRecorder) WatchLogFeed(
	_ context.Context,
	channelID, service, mode, since string,
	interval time.Duration,
	limit int,
) (string, error) {
	r.watchCall = &logsWatchCall{channelID, service, mode, since, interval, limit}
	return r.watchResult, r.watchErr
}

func (r *logsWatchOpsRecorder) UnwatchLogFeed(
	_ context.Context,
	service, mode string,
) (string, error) {
	r.unwatchCall = &logsUnwatchCall{service, mode}
	return r.unwatchResult, r.unwatchErr
}

func (r *logsWatchOpsRecorder) ListLogFeeds(context.Context) string {
	r.listCalls++
	return r.listResult
}

func TestOpsLogsWatchUsesInteractionChannelAndDefaults(t *testing.T) {
	ops := &logsWatchOpsRecorder{watchResult: "watching"}
	h := &Handler{ops: ops}

	got := h.opsLogsCommand(
		context.Background(),
		Interaction{ChannelID: "interaction-channel"},
		logsSubcommand("watch"),
	)

	if got != "watching" {
		t.Fatalf("unexpected result: %s", got)
	}
	want := logsWatchCall{
		channelID: "interaction-channel",
		service:   "report",
		mode:      "errors",
		since:     "30m",
		interval:  5 * time.Minute,
		limit:     10,
	}
	if ops.watchCall == nil || *ops.watchCall != want {
		t.Fatalf("unexpected watch call: %#v", ops.watchCall)
	}
}

func TestOpsLogsWatchForwardsExplicitOptionsAndCapsLimit(t *testing.T) {
	ops := &logsWatchOpsRecorder{watchResult: "watching"}
	h := &Handler{ops: ops}

	got := h.opsLogsCommand(
		context.Background(),
		Interaction{ChannelID: "interaction-channel"},
		logsSubcommand(
			"watch",
			stringInteractionOption("channel", "configured-channel"),
			stringInteractionOption("service", "auth"),
			stringInteractionOption("mode", "recent"),
			stringInteractionOption("since", "1h"),
			stringInteractionOption("interval", "10m"),
			stringInteractionOption("limit", "25"),
		),
	)

	if got != "watching" {
		t.Fatalf("unexpected result: %s", got)
	}
	want := logsWatchCall{"configured-channel", "auth", "recent", "1h", 10 * time.Minute, 20}
	if ops.watchCall == nil || *ops.watchCall != want {
		t.Fatalf("unexpected watch call: %#v", ops.watchCall)
	}
}

func TestOpsLogsUnwatchAndWatchesDelegate(t *testing.T) {
	ops := &logsWatchOpsRecorder{unwatchResult: "unwatched", listResult: "watch list"}
	h := &Handler{ops: ops}

	unwatch := h.opsLogsCommand(
		context.Background(),
		Interaction{},
		logsSubcommand("unwatch", stringInteractionOption("service", "post"), stringInteractionOption("mode", "security")),
	)
	if unwatch != "unwatched" || ops.unwatchCall == nil || *ops.unwatchCall != (logsUnwatchCall{"post", "security"}) {
		t.Fatalf("unexpected unwatch result=%q call=%#v", unwatch, ops.unwatchCall)
	}

	watches := h.opsLogsCommand(context.Background(), Interaction{}, logsSubcommand("watches"))
	if watches != "watch list" || ops.listCalls != 1 {
		t.Fatalf("unexpected watches result=%q calls=%d", watches, ops.listCalls)
	}
}

func TestOpsCommandDispatchesLogsWatch(t *testing.T) {
	ops := &logsWatchOpsRecorder{watchResult: "watching"}
	h := &Handler{ops: ops}
	interaction := Interaction{
		ChannelID: "interaction-channel",
		Data: ApplicationCommandData{
			Name:    "ops",
			Options: []ApplicationCommandOpt{logsSubcommand("watch")},
		},
	}

	got := h.opsCommand(context.Background(), interaction)

	if got != "watching" || ops.watchCall == nil {
		t.Fatalf("unexpected dispatch result=%q call=%#v", got, ops.watchCall)
	}
}

func TestOpsLogsWatchRejectsInvalidIntervalBeforeControllerCall(t *testing.T) {
	ops := &logsWatchOpsRecorder{}
	h := &Handler{ops: ops}

	got := h.opsLogsCommand(
		context.Background(),
		Interaction{},
		logsSubcommand("watch", stringInteractionOption("interval", "2m")),
	)

	if got != "지원하지 않는 interval입니다. 3m, 5m, 10m, 15m 중 하나를 사용하세요." {
		t.Fatalf("unexpected invalid interval result: %s", got)
	}
	if ops.watchCall != nil {
		t.Fatalf("controller must not be called for invalid input: %#v", ops.watchCall)
	}
}

func TestOpsLogsWatchActionsRequireControllerAndSanitizeErrors(t *testing.T) {
	for _, action := range []string{"watch", "unwatch", "watches"} {
		t.Run("missing "+action, func(t *testing.T) {
			h := &Handler{}
			got := h.opsLogsCommand(context.Background(), Interaction{}, logsSubcommand(action))
			if got != "Service Ops controller is not ready." {
				t.Fatalf("unexpected missing controller result: %s", got)
			}
		})
	}

	t.Run("watch error", func(t *testing.T) {
		ops := &logsWatchOpsRecorder{watchErr: errors.New("failed token=super-secret")}
		h := &Handler{ops: ops}
		got := h.opsLogsCommand(context.Background(), Interaction{}, logsSubcommand("watch"))
		if got != "failed token=[REDACTED]" {
			t.Fatalf("unexpected sanitized error: %s", got)
		}
	})
}

func logsSubcommand(action string, options ...ApplicationCommandOpt) ApplicationCommandOpt {
	return ApplicationCommandOpt{
		Name: "logs",
		Options: append(
			[]ApplicationCommandOpt{stringInteractionOption("action", action)},
			options...,
		),
	}
}
