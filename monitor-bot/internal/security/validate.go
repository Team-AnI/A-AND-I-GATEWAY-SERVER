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
		"24h": 24 * time.Hour,
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
	traceIDPattern            = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,128}$`)
	courseSlugPattern         = regexp.MustCompile(`^[a-zA-Z0-9._:-]{1,128}$`)
	logSearchQueryPattern     = regexp.MustCompile(`^[a-zA-Z0-9._:/-]{1,160}$`)
	allowedAssignmentStatuses = map[string]struct{}{
		"all":       {},
		"published": {},
		"draft":     {},
		"scheduled": {},
	}
	allowedAssignmentWindows = map[string]struct{}{
		"today":     {},
		"this-week": {},
	}
)

func NormalizeService(service string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(service))
	switch normalized {
	case "judge":
		normalized = "online-judge"
	case "blog":
		normalized = "post"
	}
	_, ok := allowedServices[normalized]
	return normalized, ok
}

func NormalizeServiceOrAll(service string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(service))
	if normalized == "all" {
		return normalized, true
	}
	return NormalizeService(service)
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

func ValidateAssignmentID(assignmentID string) bool {
	return traceIDPattern.MatchString(strings.TrimSpace(assignmentID))
}

func ValidateLogSearchQuery(query string) bool {
	return logSearchQueryPattern.MatchString(strings.TrimSpace(query))
}

func ValidateCourseSlug(courseSlug string) bool {
	return courseSlugPattern.MatchString(strings.TrimSpace(courseSlug))
}

func NormalizeAssignmentStatus(status string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "" {
		normalized = "all"
	}
	_, ok := allowedAssignmentStatuses[normalized]
	return normalized, ok
}

func NormalizeAssignmentWindow(window string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(window))
	if normalized == "" {
		normalized = "today"
	}
	_, ok := allowedAssignmentWindows[normalized]
	return normalized, ok
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
