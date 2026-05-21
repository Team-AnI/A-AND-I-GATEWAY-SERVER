package discord

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const (
	interactionTypeMessageComponent = 3

	componentTypeActionRow = 1
	componentTypeButton    = 2

	buttonStylePrimary   = 1
	buttonStyleSecondary = 2
	buttonStyleSuccess   = 3
	buttonStyleDanger    = 4

	maxButtonCustomIDLength = 100
	opsButtonPrefix         = "ops:v1"
)

type MessageComponent struct {
	Type       int                `json:"type"`
	Components []MessageComponent `json:"components,omitempty"`
	Style      int                `json:"style,omitempty"`
	Label      string             `json:"label,omitempty"`
	CustomID   string             `json:"custom_id,omitempty"`
	Disabled   bool               `json:"disabled,omitempty"`
}

type messageComponent = MessageComponent

type opsButtonAction struct {
	Kind    string
	TraceID string
	Service string
	Mode    string
	Since   string
	Limit   int
}

func ActionRow(components ...MessageComponent) MessageComponent {
	return MessageComponent{Type: componentTypeActionRow, Components: components}
}

func PrimaryButton(label, customID string) MessageComponent {
	return button(buttonStylePrimary, label, customID)
}

func SecondaryButton(label, customID string) MessageComponent {
	return button(buttonStyleSecondary, label, customID)
}

func button(style int, label, customID string) MessageComponent {
	return MessageComponent{
		Type:     componentTypeButton,
		Style:    style,
		Label:    strings.TrimSpace(label),
		CustomID: strings.TrimSpace(customID),
	}
}

func OpsTraceButtonCustomID(traceID string) (string, bool) {
	traceID = strings.TrimSpace(traceID)
	if !security.ValidateTraceID(traceID) {
		return "", false
	}
	customID := opsButtonPrefix + ":trace:" + traceID
	if len(customID) > maxButtonCustomIDLength {
		return "", false
	}
	return customID, true
}

func OpsServiceErrorsButtonCustomID(service string) (string, bool) {
	normalized, ok := security.NormalizeService(service)
	if !ok || !isOpsV2Service(normalized) {
		return "", false
	}
	customID := fmt.Sprintf("%s:logs:%s:errors:30m:10", opsButtonPrefix, normalized)
	if len(customID) > maxButtonCustomIDLength {
		return "", false
	}
	return customID, true
}

func parseOpsButtonCustomID(customID string) (opsButtonAction, error) {
	customID = strings.TrimSpace(customID)
	if customID == "" || len(customID) > maxButtonCustomIDLength {
		return opsButtonAction{}, fmt.Errorf("invalid button custom_id")
	}
	parts := strings.Split(customID, ":")
	if len(parts) < 3 || strings.Join(parts[:2], ":") != opsButtonPrefix {
		return opsButtonAction{}, fmt.Errorf("unsupported button custom_id")
	}
	switch parts[2] {
	case "trace":
		if len(parts) != 4 || !security.ValidateTraceID(parts[3]) {
			return opsButtonAction{}, fmt.Errorf("invalid trace button custom_id")
		}
		return opsButtonAction{Kind: "trace", TraceID: strings.TrimSpace(parts[3])}, nil
	case "logs":
		if len(parts) != 7 {
			return opsButtonAction{}, fmt.Errorf("invalid logs button custom_id")
		}
		service, ok := security.NormalizeService(parts[3])
		if !ok || !isOpsV2Service(service) {
			return opsButtonAction{}, fmt.Errorf("invalid logs service")
		}
		if parts[4] != "errors" {
			return opsButtonAction{}, fmt.Errorf("invalid logs mode")
		}
		since := strings.TrimSpace(parts[5])
		if _, ok := security.ParseSince(since); !ok {
			return opsButtonAction{}, fmt.Errorf("invalid logs since")
		}
		rawLimit, err := strconv.Atoi(strings.TrimSpace(parts[6]))
		if err != nil || rawLimit <= 0 {
			return opsButtonAction{}, fmt.Errorf("invalid logs limit")
		}
		return opsButtonAction{
			Kind:    "logs",
			Service: service,
			Mode:    "errors",
			Since:   since,
			Limit:   parseOpsLimit(strconv.Itoa(rawLimit), 10),
		}, nil
	default:
		return opsButtonAction{}, fmt.Errorf("unsupported button action")
	}
}
