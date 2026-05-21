package discord

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
)

type fakeLogsAPI struct {
	mu          sync.Mutex
	startInputs []cloudwatchlogs.StartQueryInput
	rows        [][]types.ResultField
}

func (f *fakeLogsAPI) StartQuery(_ context.Context, input *cloudwatchlogs.StartQueryInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.StartQueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startInputs = append(f.startInputs, *input)
	return &cloudwatchlogs.StartQueryOutput{QueryId: aws.String("query-1")}, nil
}

func (f *fakeLogsAPI) GetQueryResults(context.Context, *cloudwatchlogs.GetQueryResultsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetQueryResultsOutput, error) {
	return &cloudwatchlogs.GetQueryResultsOutput{Status: types.QueryStatusComplete, Results: f.rows}, nil
}

func (f *fakeLogsAPI) DescribeLogGroups(context.Context, *cloudwatchlogs.DescribeLogGroupsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return &cloudwatchlogs.DescribeLogGroupsOutput{}, nil
}

type followUpTransport struct {
	ch chan interactionCallback
}

func (t *followUpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var payload interactionCallback
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	t.ch <- payload
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
	}, nil
}

func TestParseOpsButtonCustomIDValidatesShape(t *testing.T) {
	trace, err := parseOpsButtonCustomID("ops:v1:trace:f07e3433-1e8b-4ee0-b40c-3221b57f28eb")
	if err != nil {
		t.Fatal(err)
	}
	if trace.Kind != "trace" || trace.TraceID != "f07e3433-1e8b-4ee0-b40c-3221b57f28eb" {
		t.Fatalf("trace action mismatch: %#v", trace)
	}

	logs, err := parseOpsButtonCustomID("ops:v1:logs:blog:errors:30m:10")
	if err != nil {
		t.Fatal(err)
	}
	if logs.Kind != "logs" || logs.Service != "post" || logs.Mode != "errors" || logs.Since != "30m" || logs.Limit != 10 {
		t.Fatalf("logs action mismatch: %#v", logs)
	}

	for _, customID := range []string{
		"other:v1:trace:f07e3433-1e8b-4ee0-b40c-3221b57f28eb",
		"ops:v1:trace:bad trace",
		"ops:v1:logs:redis:errors:30m:10",
		"ops:v1:logs:gateway:slow:30m:10",
		"ops:v1:logs:gateway:errors:2h:10",
		"ops:v1:logs:gateway:errors:30m:x",
	} {
		if _, err := parseOpsButtonCustomID(customID); err == nil {
			t.Fatalf("expected %q to be rejected", customID)
		}
	}
}

func TestExecuteComponentMatchesOpsLogsTracePath(t *testing.T) {
	h := newComponentTestHandler(resultRows([]map[string]string{{
		"@timestamp":     "2026-05-21T10:00:00+09:00",
		"logType":        "API_ERROR",
		"service.name":   "gateway",
		"service.domain": "gateway",
		"trace.traceId":  "trace-123",
	}}))
	interaction := Interaction{Type: interactionTypeMessageComponent, Data: ApplicationCommandData{CustomID: "ops:v1:trace:trace-123"}}

	got := h.executeComponent(context.Background(), interaction)
	want := h.opsLogsCommand(context.Background(), Interaction{}, ApplicationCommandOpt{Options: []ApplicationCommandOpt{
		stringInteractionOption("mode", "trace"),
		stringInteractionOption("query", "trace-123"),
	}})
	if got != want {
		t.Fatalf("component trace output mismatch:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestExecuteComponentMatchesOpsLogsServiceErrorsPath(t *testing.T) {
	h := newComponentTestHandler(resultRows([]map[string]string{{
		"count":                  "2",
		"service.domain":         "gateway",
		"service.name":           "gateway",
		"http.route":             "/v2/test",
		"http.statusCode":        "500",
		"response.error.code":    "18801",
		"response.error.message": "internal",
		"trace.traceId":          "trace-123",
		"logType":                "API_ERROR",
	}}))
	interaction := Interaction{Type: interactionTypeMessageComponent, Data: ApplicationCommandData{CustomID: "ops:v1:logs:gateway:errors:30m:10"}}

	got := h.executeComponent(context.Background(), interaction)
	want := h.opsLogsCommand(context.Background(), Interaction{}, ApplicationCommandOpt{Options: []ApplicationCommandOpt{
		stringInteractionOption("service", "gateway"),
		stringInteractionOption("mode", "errors"),
		stringInteractionOption("since", "30m"),
		stringInteractionOption("limit", "10"),
	}})
	if got != want {
		t.Fatalf("component logs output mismatch:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestExecuteComponentRejectsInvalidCustomIDSafely(t *testing.T) {
	h := newComponentTestHandler(nil)
	got := h.executeComponent(context.Background(), Interaction{Type: interactionTypeMessageComponent, Data: ApplicationCommandData{CustomID: "ops:v1:trace:bad trace"}})
	if !strings.Contains(got, "지원하지 않는 버튼") {
		t.Fatalf("unexpected invalid button response: %s", got)
	}
}

func TestMessageComponentReturnsDeferredEphemeralFollowUp(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	followUps := make(chan interactionCallback, 1)
	h := newComponentTestHandler(resultRows([]map[string]string{{
		"@timestamp":     "2026-05-21T10:00:00+09:00",
		"logType":        "API_ERROR",
		"service.name":   "gateway",
		"service.domain": "gateway",
		"trace.traceId":  "trace-123",
	}}))
	h.cfg.DiscordPublicKey = hex.EncodeToString(publicKey)
	h.cfg.DiscordApplicationID = "app-id"
	h.cfg.DiscordAllowedGuildID = "guild-1"
	h.cfg.DiscordAllowedRoleIDs = []string{"role-1"}
	h.httpClient = &http.Client{Transport: &followUpTransport{ch: followUps}}

	body := []byte(`{"id":"interaction-1","application_id":"app-id","channel_id":"channel-1","type":3,"token":"token-1","guild_id":"guild-1","member":{"roles":["role-1"]},"data":{"custom_id":"ops:v1:trace:trace-123","component_type":2}}`)
	req := signedInteractionRequest(t, privateKey, body)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d %s", rr.Code, rr.Body.String())
	}
	var response interactionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Type != InteractionResponseDeferredChannelMessage || response.Data == nil || response.Data.Flags != MessageFlagEphemeral {
		t.Fatalf("expected ephemeral deferred response: %#v", response)
	}
	select {
	case payload := <-followUps:
		if payload.Flags != MessageFlagEphemeral {
			t.Fatalf("follow-up must be ephemeral: %#v", payload)
		}
		if !strings.Contains(payload.Content, "Trace 조회 결과") {
			t.Fatalf("follow-up should contain trace result: %s", payload.Content)
		}
		if payload.AllowedMentions == nil || len(payload.AllowedMentions.Parse) != 0 {
			t.Fatalf("follow-up must suppress mentions: %#v", payload.AllowedMentions)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for follow-up")
	}
}

func TestMessageComponentUsesSameAuthorizationAsSlashCommands(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newComponentTestHandler(nil)
	h.cfg.DiscordPublicKey = hex.EncodeToString(publicKey)
	h.cfg.DiscordAllowedGuildID = "guild-1"
	h.cfg.DiscordAllowedRoleIDs = []string{"role-1"}

	body := []byte(`{"id":"interaction-1","application_id":"app-id","channel_id":"channel-1","type":3,"token":"token-1","guild_id":"guild-1","member":{"roles":["other-role"]},"data":{"custom_id":"ops:v1:trace:trace-123","component_type":2}}`)
	req := signedInteractionRequest(t, privateKey, body)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var response interactionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Type != InteractionResponseChannelMessage || response.Data == nil || response.Data.Flags != MessageFlagEphemeral {
		t.Fatalf("expected ephemeral auth rejection: %#v", response)
	}
	if !strings.Contains(response.Data.Content, "권한") {
		t.Fatalf("unexpected auth response: %s", response.Data.Content)
	}
}

func TestPingAndUnsupportedInteractionBehavior(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := newComponentTestHandler(nil)
	h.cfg.DiscordPublicKey = hex.EncodeToString(publicKey)

	ping := httptest.NewRecorder()
	h.ServeHTTP(ping, signedInteractionRequest(t, privateKey, []byte(`{"type":1}`)))
	var pingResponse interactionResponse
	if err := json.Unmarshal(ping.Body.Bytes(), &pingResponse); err != nil {
		t.Fatal(err)
	}
	if pingResponse.Type != InteractionResponsePong {
		t.Fatalf("unexpected ping response: %#v", pingResponse)
	}

	unsupported := httptest.NewRecorder()
	h.ServeHTTP(unsupported, signedInteractionRequest(t, privateKey, []byte(`{"type":9}`)))
	var unsupportedResponse interactionResponse
	if err := json.Unmarshal(unsupported.Body.Bytes(), &unsupportedResponse); err != nil {
		t.Fatal(err)
	}
	if unsupportedResponse.Type != InteractionResponseChannelMessage || unsupportedResponse.Data == nil || !strings.Contains(unsupportedResponse.Data.Content, "지원하지 않는 interaction type") {
		t.Fatalf("unexpected unsupported response: %#v", unsupportedResponse)
	}
}

func newComponentTestHandler(rows [][]types.ResultField) *Handler {
	cfg := config.Config{
		DiscordApplicationID:        "app-id",
		DiscordEphemeralResponses:   true,
		CloudWatchQueryTimeout:      time.Second,
		CloudWatchQueryPollInterval: time.Nanosecond,
		CloudWatchQueryLimit:        20,
		CloudWatchMaxLogGroups:      5,
		LogGroups: map[string]string{
			"gateway": "/gateway",
			"auth":    "/auth",
			"report":  "/report",
			"post":    "/post",
		},
	}
	return &Handler{
		cfg:          cfg,
		logs:         cw.NewLogsClient(&fakeLogsAPI{rows: rows}, cfg.CloudWatchQueryTimeout, cfg.CloudWatchQueryPollInterval, cfg.CloudWatchQueryLimit),
		httpClient:   http.DefaultClient,
		replayWindow: time.Hour,
	}
}

func resultRows(rows []map[string]string) [][]types.ResultField {
	results := make([][]types.ResultField, 0, len(rows))
	for _, row := range rows {
		fields := make([]types.ResultField, 0, len(row))
		for key, value := range row {
			fields = append(fields, types.ResultField{Field: aws.String(key), Value: aws.String(value)})
		}
		results = append(results, fields)
	}
	return results
}

func signedInteractionRequest(t *testing.T, privateKey ed25519.PrivateKey, body []byte) *http.Request {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := ed25519.Sign(privateKey, append([]byte(timestamp), body...))
	req := httptest.NewRequest(http.MethodPost, "/interactions", strings.NewReader(string(body)))
	req.Header.Set("X-Signature-Timestamp", timestamp)
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(signature))
	return req
}
