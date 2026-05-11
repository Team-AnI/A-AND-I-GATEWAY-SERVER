package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort                    string
	DiscordApplicationID        string
	DiscordPublicKey            string
	DiscordBotToken             string
	DiscordAllowedGuildID       string
	DiscordAllowedRoleIDs       []string
	DiscordRegisterCommands     bool
	DiscordCommandScope         string
	DiscordEphemeralResponses   bool
	AWSRegion                   string
	CloudWatchQueryTimeout      time.Duration
	CloudWatchQueryPollInterval time.Duration
	CloudWatchQueryLimit        int
	CloudWatchMaxLogGroups      int
	HealthRequestTimeout        time.Duration
	LogGroups                   map[string]string
	HealthURLs                  map[string]string
	ServiceRegistry             []ServiceDefinition
	StatePath                   string
	Dashboard                   DashboardConfig
	Alert                       AlertConfig
}

type ServiceDefinition struct {
	Name        string
	DisplayName string
	ServiceName string
	DomainCode  int
	LogGroup    string
	HealthURL   string
	Enabled     bool
}

type DashboardConfig struct {
	Enabled              bool
	ChannelID            string
	RefreshInterval      time.Duration
	Since                string
	MaxCloudWatchQueries int
}

type AlertConfig struct {
	Enabled                  bool
	ChannelID                string
	PollInterval             time.Duration
	Cooldown                 time.Duration
	FiveXXThreshold5m        int
	ErrorThreshold5m         int
	HealthDownConsecutive    int
	NoLogsMinutes            int
	CopyAPIFiveXXThreshold5m int
}

func Load() Config {
	logGroups := map[string]string{
		"gateway":      env("LOG_GROUP_GATEWAY", "/a-and-i/gateway"),
		"auth":         env("LOG_GROUP_AUTH", "/a-and-i/auth"),
		"report":       env("LOG_GROUP_REPORT", "/a-and-i/prod/report"),
		"online-judge": env("LOG_GROUP_ONLINE_JUDGE", "/a-and-i/online-judge"),
		"post":         env("LOG_GROUP_POST", "/a-and-i/prod/tech-blog"),
	}
	healthURLs := map[string]string{
		"gateway":      env("HEALTH_URL_GATEWAY", "http://gateway:9090/actuator/health"),
		"auth":         env("HEALTH_URL_AUTH", ""),
		"report":       env("HEALTH_URL_REPORT", ""),
		"online-judge": env("HEALTH_URL_ONLINE_JUDGE", ""),
		"post":         env("HEALTH_URL_POST", ""),
	}
	return Config{
		HTTPPort:                    env("BOT_HTTP_PORT", "8088"),
		DiscordApplicationID:        env("DISCORD_APPLICATION_ID", ""),
		DiscordPublicKey:            env("DISCORD_PUBLIC_KEY", ""),
		DiscordBotToken:             env("DISCORD_BOT_TOKEN", ""),
		DiscordAllowedGuildID:       env("DISCORD_ALLOWED_GUILD_ID", ""),
		DiscordAllowedRoleIDs:       splitCSV(env("DISCORD_ALLOWED_ROLE_IDS", "")),
		DiscordRegisterCommands:     envBool("DISCORD_REGISTER_COMMANDS", false),
		DiscordCommandScope:         env("DISCORD_COMMAND_SCOPE", "guild"),
		DiscordEphemeralResponses:   envBool("DISCORD_EPHEMERAL_RESPONSES", true),
		AWSRegion:                   env("AWS_REGION", "ap-northeast-2"),
		CloudWatchQueryTimeout:      time.Duration(envInt("CLOUDWATCH_QUERY_TIMEOUT_SECONDS", 8)) * time.Second,
		CloudWatchQueryPollInterval: time.Duration(envInt("CLOUDWATCH_QUERY_POLL_INTERVAL_MS", 500)) * time.Millisecond,
		CloudWatchQueryLimit:        envInt("CLOUDWATCH_QUERY_LIMIT", 20),
		CloudWatchMaxLogGroups:      envInt("CLOUDWATCH_MAX_LOG_GROUPS_PER_QUERY", 5),
		HealthRequestTimeout:        time.Duration(envInt("HEALTH_REQUEST_TIMEOUT_MS", 2000)) * time.Millisecond,
		LogGroups:                   logGroups,
		HealthURLs:                  healthURLs,
		ServiceRegistry:             BuildServiceRegistry(logGroups, healthURLs),
		StatePath:                   env("MONITOR_BOT_STATE_PATH", "/var/lib/monitor-bot/state.json"),
		Dashboard: DashboardConfig{
			Enabled:              envBool("DASHBOARD_ENABLED", false),
			ChannelID:            env("DISCORD_DASHBOARD_CHANNEL_ID", ""),
			RefreshInterval:      time.Duration(envInt("DASHBOARD_REFRESH_INTERVAL_SECONDS", 300)) * time.Second,
			Since:                env("DASHBOARD_SINCE", "30m"),
			MaxCloudWatchQueries: envInt("MAX_CLOUDWATCH_QUERIES_PER_TICK", 6),
		},
		Alert: AlertConfig{
			Enabled:                  envBool("ALERT_ENABLED", false),
			ChannelID:                env("DISCORD_ALERT_CHANNEL_ID", ""),
			PollInterval:             time.Duration(envInt("ALERT_POLL_INTERVAL_SECONDS", 180)) * time.Second,
			Cooldown:                 time.Duration(envInt("ALERT_COOLDOWN_SECONDS", 900)) * time.Second,
			FiveXXThreshold5m:        envInt("ALERT_5XX_THRESHOLD_5M", 3),
			ErrorThreshold5m:         envInt("ALERT_ERROR_THRESHOLD_5M", 5),
			HealthDownConsecutive:    envInt("ALERT_HEALTH_DOWN_CONSECUTIVE", 2),
			NoLogsMinutes:            envInt("ALERT_NO_LOGS_MINUTES", 30),
			CopyAPIFiveXXThreshold5m: envInt("ALERT_COPY_API_5XX_THRESHOLD_5M", 1),
		},
	}
}

func BuildServiceRegistry(logGroups, healthURLs map[string]string) []ServiceDefinition {
	order := []struct {
		name        string
		displayName string
		serviceName string
		domainCode  int
	}{
		{"gateway", "gateway", "gateway", 0},
		{"auth", "auth", "auth-service", 1},
		{"report", "report", "report-service", 4},
		{"online-judge", "online-judge", "online-judge-service", 3},
		{"post", "post", "post-service", 2},
	}
	registry := make([]ServiceDefinition, 0, len(order))
	for _, item := range order {
		logGroup := strings.TrimSpace(logGroups[item.name])
		healthURL := strings.TrimSpace(healthURLs[item.name])
		registry = append(registry, ServiceDefinition{
			Name:        item.name,
			DisplayName: item.displayName,
			ServiceName: item.serviceName,
			DomainCode:  item.domainCode,
			LogGroup:    logGroup,
			HealthURL:   healthURL,
			Enabled:     logGroup != "" || healthURL != "",
		})
	}
	return registry
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
