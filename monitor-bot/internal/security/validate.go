package security

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	allowedServices = map[string]struct{}{
		"gateway":      {},
		"auth":         {},
		"report":       {},
		"online-judge": {},
		"post":         {},
	}
	allowedSince = map[string]time.Duration{
		"5m":  5 * time.Minute,
		"15m": 15 * time.Minute,
		"30m": 30 * time.Minute,
		"1h":  time.Hour,
		"3h":  3 * time.Hour,
	}
	allowedLevels = map[string]struct{}{
		"INFO":  {},
		"WARN":  {},
		"ERROR": {},
	}
	allowedCountTypes = map[string]struct{}{
		"all":   {},
		"api":   {},
		"error": {},
		"4xx":   {},
		"5xx":   {},
	}
	allowedTopBy = map[string]struct{}{
		"path":   {},
		"error":  {},
		"status": {},
	}
	traceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,128}$`)
)

func NormalizeService(service string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(service))
	_, ok := allowedServices[normalized]
	return normalized, ok
}

func ValidateService(service string) bool {
	_, ok := NormalizeService(service)
	return ok
}

func ParseSince(value string) (time.Duration, bool) {
	duration, ok := allowedSince[strings.TrimSpace(value)]
	return duration, ok
}

func NormalizeLevel(level string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(level))
	_, ok := allowedLevels[normalized]
	return normalized, ok
}

func NormalizeCountType(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	_, ok := allowedCountTypes[normalized]
	return normalized, ok
}

func NormalizeTopBy(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	_, ok := allowedTopBy[normalized]
	return normalized, ok
}

func ValidateTraceID(traceID string) bool {
	return traceIDPattern.MatchString(strings.TrimSpace(traceID))
}

func ClampLimit(limit, fallback, max int) int32 {
	if limit <= 0 {
		limit = fallback
	}
	if limit > max {
		limit = max
	}
	return int32(limit)
}

func ParsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
