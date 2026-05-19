package cloudwatch

import "strings"

func RetentionTargetLogGroups(logGroups map[string]string) []string {
	targets := []string{
		valueOrDefault(logGroups, "gateway", "/a-and-i/gateway"),
		"/a-and-i/prod/monitor-bot",
		valueOrDefault(logGroups, "report", "/a-and-i/prod/report"),
		"/a-and-i/prod/report-mongodb",
		valueOrDefault(logGroups, "auth", "/a-and-i/auth"),
		valueOrDefault(logGroups, "online-judge", "/a-and-i/online-judge"),
		valueOrDefault(logGroups, "post", "/a-and-i/prod/tech-blog"),
	}
	result := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		result = append(result, target)
	}
	return result
}

func valueOrDefault(values map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	return fallback
}
