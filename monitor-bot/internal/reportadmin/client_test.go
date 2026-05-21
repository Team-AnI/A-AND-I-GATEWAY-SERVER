package reportadmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMissingTokenReturnsConfigError(t *testing.T) {
	client := NewClient("http://report", "", time.Second)
	_, err := client.ListAssignments(context.Background(), "kotlin-basic")
	if StatusOf(err) != StatusConfigError {
		t.Fatalf("missing token status = %s, err=%v", StatusOf(err), err)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestAdminAPISuccessParsingUsesGETOnly(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/auth/refresh" {
			if r.Method != http.MethodPost {
				t.Fatalf("refresh must use POST, got %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"data":{"accessToken":"secret-token"}}`))
			return
		}
		methods = append(methods, r.Method)
		if r.Method != http.MethodGet {
			t.Fatalf("admin client must use GET only, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.Header.Get("Authenticate"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authenticate header: %q", got)
		}
		if got := r.Header.Get("deviceOS"); got != "monitor-bot" {
			t.Fatalf("unexpected deviceOS header: %q", got)
		}
		if got := r.Header.Get("timestamp"); got == "" {
			t.Fatal("timestamp header was not set")
		} else if _, err := time.Parse(time.RFC3339, got); err != nil {
			t.Fatalf("timestamp header is not RFC3339: %q", got)
		}
		if got := r.Header.Get("salt"); got != "" {
			t.Fatalf("salt header should not be set, got %q", got)
		}
		switch r.URL.Path {
		case "/v2/admin/courses":
			_, _ = w.Write([]byte(`{"data":[{"courseSlug":"kotlin-basic","title":"Kotlin"}]}`))
		case "/v2/admin/courses/kotlin-basic/assignments":
			_, _ = w.Write([]byte(`{"data":{"assignments":[{"assignmentId":"a1","status":"published","startAt":"2026-05-13T09:00:00+09:00","endAt":"2026-05-20T09:00:00+09:00","problemId":"p1","updatedAt":"2026-05-13T10:00:00+09:00"}]}}`))
		case "/v2/admin/courses/kotlin-basic/assignments/a1":
			_, _ = w.Write([]byte(`{"data":{"assignmentId":"a1","status":"published","problemId":"p1","startAt":"2026-05-13T09:00:00+09:00","endAt":"2026-05-20T09:00:00+09:00"}}`))
		case "/v2/admin/courses/kotlin-basic/assignments/a1/submission-statuses":
			_, _ = w.Write([]byte(`{"data":[{"submitted":true,"status":"GRADED","score":80,"gradedAt":"2026-05-13T11:00:00+09:00"},{"submitted":false,"status":"PENDING"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithRefresh(server.URL, server.URL, "refresh-secret", time.Second)
	courses, err := client.ListCourses(context.Background())
	if err != nil || len(courses) != 1 || courses[0].Slug != "kotlin-basic" {
		t.Fatalf("courses parse failed: courses=%#v err=%v", courses, err)
	}
	assignments, err := client.ListAssignments(context.Background(), "kotlin-basic")
	if err != nil || len(assignments) != 1 || assignments[0].ID != "a1" || assignments[0].ProblemID != "p1" {
		t.Fatalf("assignments parse failed: assignments=%#v err=%v", assignments, err)
	}
	assignment, err := client.GetAssignment(context.Background(), "kotlin-basic", "a1")
	if err != nil || assignment.ID != "a1" {
		t.Fatalf("assignment parse failed: assignment=%#v err=%v", assignment, err)
	}
	summary, err := client.SubmissionStatuses(context.Background(), "kotlin-basic", "a1")
	if err != nil || summary.TotalStudents != 2 || summary.Submitted != 1 || summary.Graded != 1 {
		t.Fatalf("submission summary failed: summary=%#v err=%v", summary, err)
	}
	for _, method := range methods {
		if method != http.MethodGet {
			t.Fatalf("non-GET method used: %s", method)
		}
	}
}

func TestAdminAPIRefreshesTokenWithoutPostingAdminEndpoints(t *testing.T) {
	var adminMethods []string
	var refreshCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/auth/refresh":
			refreshCalls++
			if r.Method != http.MethodPost {
				t.Fatalf("refresh must use POST, got %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("unexpected refresh content-type: %q", got)
			}
			if got := r.Header.Get("salt"); got != "" {
				t.Fatalf("refresh salt header should not be set, got %q", got)
			}
			_, _ = w.Write([]byte(`{"data":{"accessToken":"fresh-access","refreshToken":"fresh-refresh"}}`))
		case "/v2/admin/courses":
			adminMethods = append(adminMethods, r.Method)
			if r.Method != http.MethodGet {
				t.Fatalf("admin API must use GET only, got %s", r.Method)
			}
			if got := r.Header.Get("Authenticate"); got != "Bearer fresh-access" {
				t.Fatalf("admin API did not use refreshed token: %q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"courseSlug":"kotlin-basic","title":"Kotlin"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithRefresh(server.URL, server.URL, "refresh-secret", time.Second)
	courses, err := client.ListCourses(context.Background())
	if err != nil || len(courses) != 1 {
		t.Fatalf("refresh-backed list courses failed: courses=%#v err=%v", courses, err)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}
	for _, method := range adminMethods {
		if method != http.MethodGet {
			t.Fatalf("admin endpoint used non-GET method: %s", method)
		}
	}
}

func TestAdminAPIRefreshesAndRetriesOnUnauthorized(t *testing.T) {
	var adminCalls int
	var refreshCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/auth/refresh":
			refreshCalls++
			if refreshCalls == 1 {
				_, _ = w.Write([]byte(`{"data":{"accessToken":"old-access"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"accessToken":"fresh-access"}}`))
		case "/v2/admin/courses":
			adminCalls++
			if adminCalls == 1 {
				http.Error(w, "expired old-access refresh-secret", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("Authenticate"); got != "Bearer fresh-access" {
				t.Fatalf("retry did not use refreshed token: %q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"courseSlug":"kotlin-basic"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithRefresh(server.URL, server.URL, "refresh-secret", time.Second)
	courses, err := client.ListCourses(context.Background())
	if err != nil || len(courses) != 1 {
		t.Fatalf("retry after refresh failed: courses=%#v err=%v", courses, err)
	}
	if adminCalls != 2 {
		t.Fatalf("admin calls = %d, want 2", adminCalls)
	}
	if refreshCalls != 2 {
		t.Fatalf("refresh calls = %d, want 2", refreshCalls)
	}
}

func TestTokenRefreshErrorDoesNotLeakRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "refresh-secret", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClientWithRefresh(server.URL, server.URL, "refresh-secret", time.Second)
	_, err := client.ListCourses(context.Background())
	if StatusOf(err) != StatusAuthError {
		t.Fatalf("refresh auth error status = %s, err=%v", StatusOf(err), err)
	}
	if strings.Contains(err.Error(), "refresh-secret") {
		t.Fatalf("refresh token leaked in error: %v", err)
	}
}

func TestAdminAPIEmptyAndErrorStatuses(t *testing.T) {
	cases := map[string]string{
		"/unauthorized": StatusAuthError,
		"/forbidden":    StatusForbidden,
		"/missing":      StatusNoData,
		"/upstream":     StatusUpstreamError,
		"/invalid":      StatusInvalidResponse,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/auth/refresh":
			_, _ = w.Write([]byte(`{"data":{"accessToken":"secret-token"}}`))
		case "/v2/admin/courses/empty/assignments":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/v2/admin/courses/unauthorized/assignments":
			http.Error(w, "nope secret-token", http.StatusUnauthorized)
		case "/v2/admin/courses/forbidden/assignments":
			http.Error(w, "forbidden secret-token", http.StatusForbidden)
		case "/v2/admin/courses/missing/assignments":
			http.NotFound(w, r)
		case "/v2/admin/courses/upstream/assignments":
			http.Error(w, "bad", http.StatusInternalServerError)
		case "/v2/admin/courses/invalid/assignments":
			_, _ = w.Write([]byte(`not-json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithRefresh(server.URL, server.URL, "refresh-secret", time.Second)
	assignments, err := client.ListAssignments(context.Background(), "empty")
	if err != nil || len(assignments) != 0 {
		t.Fatalf("empty list should be safe, assignments=%#v err=%v", assignments, err)
	}
	for course, want := range cases {
		_, err := client.ListAssignments(context.Background(), strings.TrimPrefix(course, "/"))
		if StatusOf(err) != want {
			t.Fatalf("%s status = %s, want %s, err=%v", course, StatusOf(err), want, err)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Fatalf("token leaked in error: %v", err)
		}
	}
}

func TestAdminAPITimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/auth/refresh" {
			_, _ = w.Write([]byte(`{"data":{"accessToken":"secret-token"}}`))
			return
		}
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client := NewClientWithRefresh(server.URL, server.URL, "refresh-secret", time.Millisecond)
	_, err := client.ListAssignments(context.Background(), "kotlin-basic")
	if StatusOf(err) != StatusTimeout {
		t.Fatalf("timeout status = %s, err=%v", StatusOf(err), err)
	}
}

func TestAssignmentCheckValidator(t *testing.T) {
	ok := CheckAssignment(Assignment{ID: "a1", Status: "published", ProblemID: "p1", StartAt: "2026-05-13T09:00:00+09:00", EndAt: "2026-05-14T09:00:00+09:00"})
	if ok.Status != StatusOK {
		t.Fatalf("valid assignment status = %s findings=%v", ok.Status, ok.Findings)
	}
	bad := CheckAssignment(Assignment{ID: "a1", Status: "published", ProblemID: "p1", StartAt: "2026-05-14T09:00:00+09:00", EndAt: "2026-05-13T09:00:00+09:00"})
	if bad.Status != StatusError {
		t.Fatalf("reversed time should be ERROR, got %s findings=%v", bad.Status, bad.Findings)
	}
	warn := CheckAssignment(Assignment{ID: "a1"})
	if warn.Status != StatusWarn {
		t.Fatalf("missing optional fields should be WARN, got %s findings=%v", warn.Status, warn.Findings)
	}
}
