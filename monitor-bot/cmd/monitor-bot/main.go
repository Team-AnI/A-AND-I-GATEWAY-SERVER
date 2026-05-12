package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscloudwatch "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	awslogs "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/discord"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/monitor"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck(os.Args[2:]))
	}

	cfg := config.Load()
	status := newRuntimeStatus(cfg)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	var interactionHandler safeHandler
	mux := http.NewServeMux()
	mux.HandleFunc("/interactions", interactionHandler.ServeHTTP)
	mux.HandleFunc("/healthz", health.Handler(status.Snapshot))

	listener, err := net.Listen("tcp", ":"+cfg.HTTPPort)
	if err != nil {
		log.Fatalf("http server listen failed: %v", err)
	}
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrors := make(chan error, 1)
	status.SetHTTPServer(true)
	go func() {
		log.Printf("monitor-bot listening on :%s", cfg.HTTPPort)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	awsCtx, cancelAWS := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAWS()

	awsCfg, err := awsconfig.LoadDefaultConfig(awsCtx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		status.SetAWSSDKConfigured(false)
		log.Printf("aws config init failed: %v", err)
		if cfg.StrictStartupChecks {
			log.Fatalf("STRICT_STARTUP_CHECKS enabled; aws config init failed: %v", err)
		}
	} else {
		status.SetAWSSDKConfigured(true)
	}

	registrationCtx, cancelRegistration := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelRegistration()
	if err := registerCommandsIfEnabled(registrationCtx, cfg, httpClient, status); err != nil {
		log.Fatalf("%v", err)
	}

	if err != nil {
		log.Printf("monitor-bot started without AWS clients; interactions remain unavailable")
		waitForServer(serverErrors)
		return
	}

	healthClient := health.NewClient(cfg.HealthURLs, cfg.HealthRequestTimeout)
	logsClient := cw.NewLogsClient(awslogs.NewFromConfig(awsCfg), cfg.CloudWatchQueryTimeout, cfg.CloudWatchQueryPollInterval, cfg.CloudWatchQueryLimit)
	alarmClient := cw.NewAlarmClient(awscloudwatch.NewFromConfig(awsCfg))
	stateStore := state.NewStore(cfg.StatePath)
	if err := stateStore.Load(); err != nil {
		log.Printf("state load failed: %v", err)
	}
	monitorService := monitor.NewService(cfg, healthClient, logsClient, alarmClient, stateStore, httpClient)
	readyInteractionHandler := discord.NewHandler(cfg, healthClient, logsClient, alarmClient)
	readyInteractionHandler.SetWatcher(monitorService)
	interactionHandler.Set(readyInteractionHandler)
	monitorService.Start(context.Background())

	waitForServer(serverErrors)
}

type runtimeStatus struct {
	mu     sync.RWMutex
	status health.ServerStatus
}

func newRuntimeStatus(cfg config.Config) *runtimeStatus {
	return &runtimeStatus{
		status: health.ServerStatus{
			OK:                       true,
			DiscordPublicKeyProvided: cfg.DiscordPublicKey != "",
			DashboardEnabled:         cfg.Dashboard.Enabled,
			AlertEnabled:             cfg.Alert.Enabled,
			Version:                  version,
		},
	}
}

func (s *runtimeStatus) Snapshot() health.ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *runtimeStatus) SetHTTPServer(value bool) {
	s.update(func(status *health.ServerStatus) {
		status.HTTPServer = value
	})
}

func (s *runtimeStatus) SetAWSSDKConfigured(value bool) {
	s.update(func(status *health.ServerStatus) {
		status.AWSSDKConfigured = value
	})
}

func (s *runtimeStatus) SetDiscordRegistration(registered bool, err error) {
	s.update(func(status *health.ServerStatus) {
		status.DiscordCommandsRegistered = registered
		if err == nil {
			status.DiscordCommandRegistrationError = ""
			return
		}
		status.DiscordCommandRegistrationError = security.SanitizeText(err.Error())
	})
}

func (s *runtimeStatus) update(updateFn func(*health.ServerStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	updateFn(&s.status)
}

type safeHandler struct {
	mu      sync.RWMutex
	handler http.Handler
}

func (h *safeHandler) Set(handler http.Handler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handler = handler
}

func (h *safeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	handler := h.handler
	h.mu.RUnlock()
	if handler == nil {
		http.Error(w, "monitor-bot is starting", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

func registerCommandsIfEnabled(ctx context.Context, cfg config.Config, client *http.Client, status *runtimeStatus) error {
	if !cfg.DiscordRegisterCommands {
		return nil
	}
	if cfg.DiscordCommandScope != "guild" {
		err := fmt.Errorf("DISCORD_COMMAND_SCOPE=%s is not supported; use guild scope for safe registration", cfg.DiscordCommandScope)
		status.SetDiscordRegistration(false, err)
		log.Printf("discord command registration skipped: %v", err)
		if cfg.StrictStartupChecks {
			return err
		}
		return nil
	}
	if err := discord.RegisterGuildCommands(ctx, client, cfg.DiscordBotToken, cfg.DiscordApplicationID, cfg.DiscordAllowedGuildID); err != nil {
		status.SetDiscordRegistration(false, err)
		log.Printf("discord command registration failed: %v", err)
		if cfg.StrictStartupChecks {
			return err
		}
		return nil
	}
	status.SetDiscordRegistration(true, nil)
	log.Printf("discord guild commands registered")
	return nil
}

func waitForServer(serverErrors <-chan error) {
	if err := <-serverErrors; err != nil {
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
