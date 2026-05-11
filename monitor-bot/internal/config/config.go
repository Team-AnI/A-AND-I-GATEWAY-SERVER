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
}

func Load() Config {
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
		LogGroups: map[string]string{
			"gateway":      env("LOG_GROUP_GATEWAY", "/a-and-i/gateway"),
			"auth":         env("LOG_GROUP_AUTH", "/a-and-i/auth"),
			"report":       env("LOG_GROUP_REPORT", "/a-and-i/prod/report"),
			"online-judge": env("LOG_GROUP_ONLINE_JUDGE", "/a-and-i/online-judge"),
			"post":         env("LOG_GROUP_POST", "/a-and-i/prod/tech-blog"),
		},
		HealthURLs: map[string]string{
			"gateway":      env("HEALTH_URL_GATEWAY", "http://gateway:9090/actuator/health"),
			"auth":         env("HEALTH_URL_AUTH", ""),
			"report":       env("HEALTH_URL_REPORT", ""),
			"online-judge": env("HEALTH_URL_ONLINE_JUDGE", ""),
			"post":         env("HEALTH_URL_POST", ""),
		},
	}
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
