package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
)

type Client struct {
	urls       map[string]string
	httpClient *http.Client
}

func NewClient(urls map[string]string, timeout time.Duration) *Client {
	return &Client{
		urls: urls,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Check(ctx context.Context, service string) formatting.ServiceStatus {
	url := strings.TrimSpace(c.urls[service])
	if url == "" {
		return formatting.ServiceStatus{Service: service, State: "UNKNOWN", Detail: "health URL 미설정"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return formatting.ServiceStatus{Service: service, State: "DOWN", Detail: "잘못된 health URL"}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return formatting.ServiceStatus{Service: service, State: "DOWN", Detail: err.Error()}
	}
	defer resp.Body.Close()

	state := "DOWN"
	detail := fmt.Sprintf("HTTP %d", resp.StatusCode)
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && payload.Status != "" {
		detail = payload.Status
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.EqualFold(payload.Status, "UP") {
		state = "UP"
	} else if resp.StatusCode >= 200 && resp.StatusCode < 300 && payload.Status == "" {
		state = "UP"
	}
	return formatting.ServiceStatus{Service: service, State: state, Detail: detail}
}

func (c *Client) CheckAll(ctx context.Context, services []string) []formatting.ServiceStatus {
	statuses := make([]formatting.ServiceStatus, 0, len(services))
	for _, service := range services {
		statuses = append(statuses, c.Check(ctx, service))
	}
	return statuses
}
