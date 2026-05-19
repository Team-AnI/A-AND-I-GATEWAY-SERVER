package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func (s *Service) WatchLogFeed(ctx context.Context, channelID, service, mode, since string, interval time.Duration, limit int) (string, error) {
	normalized, ok := security.NormalizeService(service)
	if !ok {
		return "", fmt.Errorf("지원하지 않는 service입니다")
	}
	if !isServiceOpsNameConnected(normalized) {
		return fmt.Sprintf("⚠️ 아직 연동되지 않은 서비스입니다\n\nservice: %s\n상태: NO_V2_LOG\nlogs-watch는 현재 gateway/auth/report V2 로그만 지원합니다.", normalized), nil
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "errors", "slow", "recent", "security":
	default:
		return "", fmt.Errorf("지원하지 않는 logs-watch mode입니다")
	}
	if _, ok := security.ParseSince(since); !ok {
		return "", fmt.Errorf("지원하지 않는 since 값입니다")
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if limit <= 0 {
		limit = 10
	}
	if strings.TrimSpace(channelID) == "" {
		return "", fmt.Errorf("channel id is required")
	}
	rows, err := s.queryLogFeedRows(ctx, normalized, mode, since, limit)
	if err != nil {
		return "", err
	}
	fingerprints := make(map[string]time.Time, len(rows))
	now := time.Now()
	for _, row := range rows {
		fingerprints[logFeedFingerprint(normalized, mode, row)] = now
	}
	feed := state.LogFeed{
		Service:       normalized,
		Mode:          mode,
		ChannelID:     strings.TrimSpace(channelID),
		IntervalSec:   int(interval.Seconds()),
		Since:         since,
		Limit:         limit,
		LastCheckedAt: now,
		Fingerprints:  fingerprints,
		Status:        "ACTIVE",
	}
	if err := s.store.Update(func(data *state.Data) {
		data.LogFeeds[state.LogFeedKey(normalized, mode)] = feed
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ 로그 피드 등록 완료\n\nservice: %s\nmode: %s\ninterval: %s\nsince: %s\nlimit: %d개\n\n기존 로그는 baseline으로 저장했고, 이후 새 항목부터 이 채널에 전송합니다.", normalized, mode, formatKoreanDuration(interval), since, limit), nil
}

func (s *Service) UnwatchLogFeed(ctx context.Context, service, mode string) (string, error) {
	normalized, ok := security.NormalizeService(service)
	if !ok {
		return "", fmt.Errorf("지원하지 않는 service입니다")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	key := state.LogFeedKey(normalized, mode)
	existed := false
	if err := s.store.Update(func(data *state.Data) {
		_, existed = data.LogFeeds[key]
		delete(data.LogFeeds, key)
	}); err != nil {
		return "", err
	}
	if !existed {
		return "NO_DATA: 해당 로그 피드가 이미 비활성 상태입니다.", nil
	}
	return "✅ 자동 로그 피드를 중지했습니다. 기존 Discord 메시지는 삭제하지 않습니다.", nil
}

func (s *Service) ListLogFeeds(ctx context.Context) string {
	snapshot := s.store.Snapshot()
	if len(snapshot.LogFeeds) == 0 {
		return "등록된 로그 피드가 없습니다.\n\nNext:\n- `/ops logs-watch service:report mode:errors interval:5m since:30m limit:10`"
	}
	keys := make([]string, 0, len(snapshot.LogFeeds))
	for key := range snapshot.LogFeeds {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("🧾 Service Log Feeds\n\n")
	for _, key := range keys {
		feed := snapshot.LogFeeds[key]
		status := firstNonEmpty(feed.Status, "ACTIVE")
		if feed.Disabled {
			status = "DISABLED"
		}
		fmt.Fprintf(&b, "- %s channel=<#%s> interval=%s since=%s limit=%d fingerprints=%d status=%s\n",
			key,
			feed.ChannelID,
			formatKoreanDuration(time.Duration(feed.IntervalSec)*time.Second),
			feed.Since,
			feed.Limit,
			len(feed.Fingerprints),
			status,
		)
	}
	return formatting.TruncateDiscordMessage(b.String())
}

func (s *Service) logFeedLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		s.refreshDueLogFeeds(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) refreshDueLogFeeds(ctx context.Context) {
	snapshot := s.store.Snapshot()
	now := time.Now()
	queries := newQueryBudget(s.cfg.Dashboard.MaxCloudWatchQueries)
	for key, feed := range snapshot.LogFeeds {
		if feed.Disabled || !isServiceOpsNameConnected(feed.Service) {
			continue
		}
		interval := time.Duration(feed.IntervalSec) * time.Second
		if interval <= 0 {
			interval = 5 * time.Minute
		}
		if !feed.LastCheckedAt.IsZero() && now.Sub(feed.LastCheckedAt) < interval {
			continue
		}
		if !queries.Allow() {
			continue
		}
		if err := s.refreshLogFeed(ctx, key, feed); err != nil {
			log.Printf("log feed refresh failed for %s: %v", key, err)
		}
	}
}

func (s *Service) refreshLogFeed(ctx context.Context, key string, feed state.LogFeed) error {
	rows, err := s.queryLogFeedRows(ctx, feed.Service, feed.Mode, feed.Since, feed.Limit)
	if err != nil {
		_ = s.store.Update(func(data *state.Data) {
			current := data.LogFeeds[key]
			current.Status = logStatusFromQueryError(err)
			current.LastCheckedAt = time.Now()
			data.LogFeeds[key] = current
		})
		return err
	}
	now := time.Now()
	var fresh []map[string]string
	known := feed.Fingerprints
	if known == nil {
		known = make(map[string]time.Time)
	}
	for _, row := range rows {
		fp := logFeedFingerprint(feed.Service, feed.Mode, row)
		if _, ok := known[fp]; ok {
			continue
		}
		known[fp] = now
		fresh = append(fresh, row)
		if len(fresh) >= feed.Limit {
			break
		}
	}
	if len(fresh) > 0 {
		content := formatLogFeedMessage(feed.Service, feed.Mode, fresh, feed.Limit)
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, feed.ChannelID, content); err != nil {
			return err
		}
	}
	return s.store.Update(func(data *state.Data) {
		current := data.LogFeeds[key]
		current.Fingerprints = known
		current.LastCheckedAt = now
		current.Status = "ACTIVE"
		data.LogFeeds[key] = current
	})
}

func (s *Service) queryLogFeedRows(ctx context.Context, service, mode, sinceLabel string, limit int) ([]map[string]string, error) {
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		return nil, fmt.Errorf("unsupported since: %s", sinceLabel)
	}
	groups, err := cw.LogGroupsForService(s.cfg.LogGroups, service)
	if err != nil {
		return nil, err
	}
	var query string
	switch mode {
	case "errors":
		query, err = cw.BuildAlertQuery(service)
	case "slow":
		query, err = cw.BuildSlowQuery(service, 0, limit)
	case "recent":
		query, err = cw.BuildRecentLogsQuery(service, "ERROR", limit)
	case "security":
		query, err = cw.BuildSecurityQuery(service, limit)
	default:
		err = fmt.Errorf("unsupported log feed mode: %s", mode)
	}
	if err != nil {
		return nil, err
	}
	return s.logs.Query(ctx, groups, query, since, int32(limit))
}

func logFeedFingerprint(service, mode string, row map[string]string) string {
	traceID := strings.TrimSpace(row["trace.traceId"])
	if traceID != "" {
		return service + ":" + mode + ":trace:" + traceID
	}
	parts := []string{
		service,
		mode,
		row["@timestamp"],
		row["http.method"],
		firstNonEmpty(row["http.route"], row["http.path"]),
		row["http.statusCode"],
		row["response.error.code"],
		row["response.error.value"],
		security.SanitizeText(row["message"]),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return service + ":" + mode + ":hash:" + hex.EncodeToString(sum[:8])
}

func formatLogFeedMessage(service, mode string, rows []map[string]string, limit int) string {
	title := "🚨 " + service + " ERROR 로그 감지"
	if mode == "slow" {
		title = "🐢 " + service + " 느린 API 감지"
	}
	if mode == "recent" {
		title = "🧾 " + service + " 최근 로그 감지"
	}
	if mode == "security" {
		title = "🛡️ " + service + " 보안 로그 감지"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", title)
	for i, row := range rows {
		if i >= limit || i >= 10 {
			break
		}
		fmt.Fprintf(&b, "%d. 시각: %s\n", i+1, shortLogValue(row["@timestamp"]))
		fmt.Fprintf(&b, "   path: %s\n", security.SanitizeText(firstNonEmpty(row["http.route"], row["http.path"])))
		if row["http.latencyMs"] != "" {
			fmt.Fprintf(&b, "   duration: %sms\n", security.SanitizeText(row["http.latencyMs"]))
		}
		if row["http.statusCode"] != "" {
			fmt.Fprintf(&b, "   status: %s\n", security.SanitizeText(row["http.statusCode"]))
		}
		if row["response.error.message"] != "" || row["message"] != "" {
			fmt.Fprintf(&b, "   message: %s\n", shortLogValue(firstNonEmpty(row["response.error.message"], row["message"])))
		}
		if row["trace.traceId"] != "" {
			fmt.Fprintf(&b, "   traceId: %s\n", security.SanitizeText(row["trace.traceId"]))
		}
	}
	b.WriteString("\n상세 확인:\n")
	if len(rows) > 0 && strings.TrimSpace(rows[0]["trace.traceId"]) != "" {
		fmt.Fprintf(&b, "/ops trace trace_id:%s\n", security.SanitizeText(rows[0]["trace.traceId"]))
	}
	fmt.Fprintf(&b, "/ops logs service:%s mode:%s since:30m limit:10", service, mode)
	return formatting.TruncateDiscordMessage(b.String())
}

func shortLogValue(value string) string {
	value = security.SanitizeText(strings.TrimSpace(value))
	if len([]rune(value)) > 120 {
		return string([]rune(value)[:120]) + "..."
	}
	if value == "" {
		return "-"
	}
	return value
}
