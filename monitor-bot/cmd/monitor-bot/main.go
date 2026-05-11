package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscloudwatch "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	awslogs "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/discord"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/monitor"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck(os.Args[2:]))
	}

	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		log.Fatalf("aws config init failed: %v", err)
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	if cfg.DiscordRegisterCommands {
		if cfg.DiscordCommandScope != "guild" {
			log.Fatalf("DISCORD_COMMAND_SCOPE=%s is not supported; use guild scope for safe registration", cfg.DiscordCommandScope)
		}
		if err := discord.RegisterGuildCommands(ctx, httpClient, cfg.DiscordBotToken, cfg.DiscordApplicationID, cfg.DiscordAllowedGuildID); err != nil {
			log.Fatalf("discord command registration failed: %v", err)
		}
		log.Printf("discord guild commands registered")
	}

	healthClient := health.NewClient(cfg.HealthURLs, cfg.HealthRequestTimeout)
	logsClient := cw.NewLogsClient(awslogs.NewFromConfig(awsCfg), cfg.CloudWatchQueryTimeout, cfg.CloudWatchQueryPollInterval, cfg.CloudWatchQueryLimit)
	alarmClient := cw.NewAlarmClient(awscloudwatch.NewFromConfig(awsCfg))
	stateStore := state.NewStore(cfg.StatePath)
	if err := stateStore.Load(); err != nil {
		log.Printf("state load failed: %v", err)
	}
	monitorService := monitor.NewService(cfg, healthClient, logsClient, alarmClient, stateStore, httpClient)
	interactionHandler := discord.NewHandler(cfg, healthClient, logsClient, alarmClient)
	interactionHandler.SetWatcher(monitorService)
	monitorService.Start(context.Background())

	mux := http.NewServeMux()
	mux.Handle("/interactions", interactionHandler)
	mux.HandleFunc("/healthz", health.Handler(func() health.ServerStatus {
		return health.ServerStatus{
			OK:                       true,
			AWSSDKConfigured:         true,
			DiscordPublicKeyProvided: cfg.DiscordPublicKey != "",
			Version:                  version,
		}
	}))

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("monitor-bot listening on :%s", cfg.HTTPPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server failed: %v", err)
	}
}

func runHealthcheck(args []string) int {
	url := "http://127.0.0.1:8088/healthz"
	for i := 0; i < len(args); i++ {
		if args[i] == "--url" && i+1 < len(args) {
			url = args[i+1]
			i++
		}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		fmt.Fprintf(os.Stderr, "healthcheck failed: HTTP %d %s\n", resp.StatusCode, string(body))
		return 1
	}
	return 0
}
