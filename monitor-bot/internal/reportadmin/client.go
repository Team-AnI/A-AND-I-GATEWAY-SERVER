package reportadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const (
	StatusOK              = "OK"
	StatusWarn            = "WARN"
	StatusError           = "ERROR"
	StatusNoData          = "NO_DATA"
	StatusConfigError     = "CONFIG_ERROR"
	StatusAuthError       = "AUTH_ERROR"
	StatusForbidden       = "FORBIDDEN"
	StatusUpstreamError   = "UPSTREAM_ERROR"
	StatusTimeout         = "TIMEOUT"
	StatusInvalidResponse = "INVALID_RESPONSE"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type APIError struct {
	Status     string
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s: upstream status %d", e.Status, e.StatusCode)
	}
	return e.Status + ": " + security.SanitizeText(e.Message)
}

type Course struct {
	Slug  string
	Title string
}

type Assignment struct {
	ID          string
	Title       string
	Status      string
	PublishedAt string
	StartAt     string
	EndAt       string
	ProblemID   string
	UpdatedAt   string
	Raw         map[string]any
}

type SubmissionSummary struct {
	TotalStudents    int
	Submitted        int
	NotSubmitted     int
	Graded           int
	Pending          int
	Failed           int
	AverageScore     string
	HighestScore     string
	LowestScore      string
	RecentGradedAt   string
	UnsupportedShape bool
	RawCount         int
}

type AssignmentCheck struct {
	Status   string
	Findings []string
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func NewClientWithHTTP(baseURL, token string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), token: strings.TrimSpace(token), httpClient: client}
}

func (c *Client) ListCourses(ctx context.Context) ([]Course, error) {
	payload, err := c.get(ctx, "/v2/admin/courses")
	if err != nil {
		return nil, err
	}
	items := arrayFrom(unwrapData(payload), "courses", "courseList", "content", "items", "list")
	if len(items) == 0 {
		return nil, nil
	}
	courses := make([]Course, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		course := Course{
			Slug:  firstString(object, "courseSlug", "slug", "id", "code"),
			Title: firstString(object, "title", "name", "courseName"),
		}
		if course.Slug != "" {
			courses = append(courses, course)
		}
	}
	return courses, nil
}

func (c *Client) ListAssignments(ctx context.Context, courseSlug string) ([]Assignment, error) {
	payload, err := c.get(ctx, "/v2/admin/courses/"+url.PathEscape(strings.TrimSpace(courseSlug))+"/assignments")
	if err != nil {
		return nil, err
	}
	items := arrayFrom(unwrapData(payload), "assignments", "assignmentList", "content", "items", "list")
	if len(items) == 0 {
		return nil, nil
	}
	assignments := make([]Assignment, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		assignments = append(assignments, assignmentFrom(object))
	}
	return assignments, nil
}

func (c *Client) GetAssignment(ctx context.Context, courseSlug, assignmentID string) (Assignment, error) {
	payload, err := c.get(ctx, "/v2/admin/courses/"+url.PathEscape(strings.TrimSpace(courseSlug))+"/assignments/"+url.PathEscape(strings.TrimSpace(assignmentID)))
	if err != nil {
		return Assignment{}, err
	}
	object, ok := unwrapData(payload).(map[string]any)
	if !ok {
		return Assignment{}, apiError(StatusInvalidResponse, 0, "assignment response is not an object")
	}
	return assignmentFrom(object), nil
}

func (c *Client) SubmissionStatuses(ctx context.Context, courseSlug, assignmentID string) (SubmissionSummary, error) {
	payload, err := c.get(ctx, "/v2/admin/courses/"+url.PathEscape(strings.TrimSpace(courseSlug))+"/assignments/"+url.PathEscape(strings.TrimSpace(assignmentID))+"/submission-statuses")
	if err != nil {
		return SubmissionSummary{}, err
	}
	return summarizeSubmissions(unwrapData(payload)), nil
}

func CheckAssignment(assignment Assignment) AssignmentCheck {
	findings := make([]string, 0, 5)
	status := StatusOK
	if strings.TrimSpace(assignment.ID) == "" {
		status = StatusError
		findings = append(findings, "assignmentId가 비어 있습니다.")
	}
	if strings.TrimSpace(assignment.Status) == "" || strings.EqualFold(assignment.Status, "unknown") {
		if status != StatusError {
			status = StatusWarn
		}
		findings = append(findings, "상태 필드가 없거나 unknown입니다.")
	}
	if strings.TrimSpace(assignment.ProblemID) == "" {
		if status != StatusError {
			status = StatusWarn
		}
		findings = append(findings, "problemId가 비어 있습니다.")
	}
	if strings.TrimSpace(assignment.StartAt) == "" || strings.TrimSpace(assignment.EndAt) == "" {
		if status != StatusError {
			status = StatusWarn
		}
		findings = append(findings, "startAt 또는 endAt이 비어 있습니다.")
	} else if start, startOK := parseFlexibleTime(assignment.StartAt); startOK {
		if end, endOK := parseFlexibleTime(assignment.EndAt); endOK && end.Before(start) {
			status = StatusError
			findings = append(findings, "endAt이 startAt보다 빠릅니다.")
		}
	}
	if len(findings) == 0 {
		findings = append(findings, "과제 기본 필드가 정상 범위입니다.")
	}
	return AssignmentCheck{Status: status, Findings: findings}
}

func FilterAssignments(assignments []Assignment, status string) []Assignment {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "" || normalized == "all" {
		return assignments
	}
	filtered := make([]Assignment, 0, len(assignments))
	for _, assignment := range assignments {
		if strings.EqualFold(assignment.Status, normalized) {
			filtered = append(filtered, assignment)
		}
	}
	return filtered
}

func (c *Client) get(ctx context.Context, endpoint string) (any, error) {
	if c.token == "" {
		return nil, apiError(StatusConfigError, 0, "REPORT_ADMIN_BEARER_TOKEN is required")
	}
	if c.baseURL == "" {
		return nil, apiError(StatusConfigError, 0, "REPORT_SERVICE_URI is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint, nil)
	if err != nil {
		return nil, apiError(StatusConfigError, 0, err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isTimeout(err) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, apiError(StatusTimeout, 0, "request timeout")
		}
		return nil, apiError(StatusUpstreamError, 0, err.Error())
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if readErr != nil {
		return nil, apiError(StatusInvalidResponse, resp.StatusCode, readErr.Error())
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, apiError(StatusAuthError, resp.StatusCode, "admin API unauthorized")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, apiError(StatusForbidden, resp.StatusCode, "admin API forbidden")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, apiError(StatusNoData, resp.StatusCode, "not found")
	}
	if resp.StatusCode >= 500 {
		return nil, apiError(StatusUpstreamError, resp.StatusCode, "admin API returned 5xx")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiError(StatusUpstreamError, resp.StatusCode, "admin API returned non-success")
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, apiError(StatusInvalidResponse, resp.StatusCode, "invalid JSON response")
	}
	return decoded, nil
}

func apiError(status string, statusCode int, message string) *APIError {
	return &APIError{Status: status, StatusCode: statusCode, Message: security.SanitizeText(message)}
}

func StatusOf(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return StatusError
}

func assignmentFrom(object map[string]any) Assignment {
	assignment := Assignment{
		ID:          firstString(object, "assignmentId", "id", "assignmentID", "uuid"),
		Title:       firstString(object, "title", "name", "assignmentTitle"),
		Status:      firstString(object, "status", "publishStatus", "publicationStatus", "state"),
		PublishedAt: firstString(object, "publishedAt", "publishAt", "openedAt", "openAt"),
		StartAt:     firstString(object, "startAt", "startedAt", "targetStartAt"),
		EndAt:       firstString(object, "endAt", "endedAt", "targetEndAt"),
		ProblemID:   firstString(object, "problemId", "problemID", "problem.id"),
		UpdatedAt:   firstString(object, "updatedAt", "modifiedAt"),
		Raw:         object,
	}
	if assignment.ProblemID == "" {
		if nested, ok := object["problem"].(map[string]any); ok {
			assignment.ProblemID = firstString(nested, "id", "problemId", "problemID")
		}
	}
	if assignment.Status == "" {
		assignment.Status = inferAssignmentStatus(object, assignment)
	}
	if assignment.Status == "" {
		assignment.Status = "unknown"
	}
	return assignment
}

func inferAssignmentStatus(object map[string]any, assignment Assignment) string {
	for _, key := range []string{"published", "isPublished", "visible", "isVisible"} {
		if value, ok := boolValue(object[key]); ok {
			if value {
				return "published"
			}
			return "draft"
		}
	}
	for _, key := range []string{"draft", "isDraft"} {
		if value, ok := boolValue(object[key]); ok && value {
			return "draft"
		}
	}
	if publishedAt := strings.TrimSpace(assignment.PublishedAt); publishedAt != "" {
		if parsed, ok := parseFlexibleTime(publishedAt); ok && parsed.After(time.Now()) {
			return "scheduled"
		}
		return "published"
	}
	return ""
}

func summarizeSubmissions(payload any) SubmissionSummary {
	items := arrayFrom(payload, "submissions", "submissionStatuses", "statuses", "content", "items", "list")
	if len(items) == 0 {
		if object, ok := payload.(map[string]any); ok {
			summary := SubmissionSummary{
				TotalStudents:  intValueAny(object["totalStudents"], object["total"], object["studentCount"]),
				Submitted:      intValueAny(object["submitted"], object["submittedCount"]),
				NotSubmitted:   intValueAny(object["notSubmitted"], object["notSubmittedCount"]),
				Graded:         intValueAny(object["graded"], object["gradedCount"], object["completedCount"]),
				Pending:        intValueAny(object["pending"], object["pendingCount"], object["judgingCount"]),
				Failed:         intValueAny(object["failed"], object["failedCount"], object["errorCount"]),
				AverageScore:   firstString(object, "averageScore", "avgScore"),
				HighestScore:   firstString(object, "highestScore", "maxScore"),
				LowestScore:    firstString(object, "lowestScore", "minScore"),
				RecentGradedAt: firstString(object, "recentGradedAt", "lastGradedAt", "updatedAt"),
			}
			if summary.TotalStudents+summary.Submitted+summary.NotSubmitted+summary.Graded+summary.Pending+summary.Failed > 0 {
				return summary
			}
		}
		return SubmissionSummary{UnsupportedShape: true}
	}
	summary := SubmissionSummary{TotalStudents: len(items), RawCount: len(items)}
	scores := make([]float64, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if submitted(object) {
			summary.Submitted++
		}
		status := strings.ToLower(firstString(object, "status", "submissionStatus", "gradingStatus", "judgeStatus"))
		switch {
		case strings.Contains(status, "fail") || strings.Contains(status, "error"):
			summary.Failed++
		case strings.Contains(status, "pending") || strings.Contains(status, "judg") || strings.Contains(status, "wait"):
			summary.Pending++
		case strings.Contains(status, "complete") || strings.Contains(status, "graded") || strings.Contains(status, "success"):
			summary.Graded++
		}
		if score, ok := floatValueAny(object["score"], object["totalScore"], object["finalScore"]); ok {
			scores = append(scores, score)
		}
		if candidate := firstString(object, "gradedAt", "judgedAt", "updatedAt"); newerTimeString(candidate, summary.RecentGradedAt) {
			summary.RecentGradedAt = candidate
		}
	}
	summary.NotSubmitted = summary.TotalStudents - summary.Submitted
	if len(scores) > 0 {
		var sum float64
		minScore, maxScore := scores[0], scores[0]
		for _, score := range scores {
			sum += score
			minScore = math.Min(minScore, score)
			maxScore = math.Max(maxScore, score)
		}
		summary.AverageScore = formatScore(sum / float64(len(scores)))
		summary.HighestScore = formatScore(maxScore)
		summary.LowestScore = formatScore(minScore)
	}
	return summary
}

func submitted(object map[string]any) bool {
	for _, key := range []string{"submitted", "isSubmitted"} {
		if value, ok := boolValue(object[key]); ok {
			return value
		}
	}
	for _, key := range []string{"submittedAt", "submissionId", "submittedTime"} {
		if strings.TrimSpace(firstString(object, key)) != "" {
			return true
		}
	}
	status := strings.ToLower(firstString(object, "status", "submissionStatus"))
	return strings.Contains(status, "submit") || strings.Contains(status, "complete") || strings.Contains(status, "graded")
}

func unwrapData(value any) any {
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	for _, key := range []string{"data", "result", "body"} {
		if nested, ok := object[key]; ok {
			return unwrapData(nested)
		}
	}
	return value
}

func arrayFrom(value any, keys ...string) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case map[string]any:
		for _, key := range keys {
			if nested, ok := typed[key]; ok {
				if items := arrayFrom(nested, keys...); len(items) > 0 {
					return items
				}
			}
		}
	}
	return nil
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := valueByPath(object, key)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return security.SanitizeText(strings.TrimSpace(typed))
			}
		case json.Number:
			return typed.String()
		case float64:
			if typed == math.Trunc(typed) {
				return strconv.FormatInt(int64(typed), 10)
			}
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(typed)
		}
	}
	return ""
}

func valueByPath(object map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = object
	for _, part := range parts {
		currentObject, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = currentObject[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func intValueAny(values ...any) int {
	for _, value := range values {
		switch typed := value.(type) {
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		case float64:
			return int(typed)
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func floatValueAny(values ...any) (float64, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case json.Number:
			if parsed, err := strconv.ParseFloat(typed.String(), 64); err == nil {
				return parsed, true
			}
		case float64:
			return typed, true
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func boolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}

func parseFlexibleTime(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05-07:00", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func newerTimeString(candidate, current string) bool {
	if strings.TrimSpace(candidate) == "" {
		return false
	}
	candidateTime, candidateOK := parseFlexibleTime(candidate)
	currentTime, currentOK := parseFlexibleTime(current)
	if candidateOK && currentOK {
		return candidateTime.After(currentTime)
	}
	return current == "" && candidate != ""
}

func formatScore(value float64) string {
	if value == math.Trunc(value) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
