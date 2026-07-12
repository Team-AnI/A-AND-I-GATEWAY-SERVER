package discord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
)

func TestOpsAssignmentQueryViewsUseAdminAPIContracts(t *testing.T) {
	const assignmentID = "11111111-1111-1111-1111-111111111111"
	server, requestedPaths := assignmentQueryServer(t, assignmentID)
	defer server.Close()

	h := &Handler{reportAdmin: reportadmin.NewClientWithRefresh(server.URL, server.URL, "refresh-secret", time.Second)}
	cases := []struct {
		name        string
		interaction Interaction
		wantPath    string
		wantText    string
	}{
		{
			name: "list",
			interaction: assignmentInteraction("list",
				stringInteractionOption("course", "course-1"),
			),
			wantPath: "/v2/admin/courses/course-1/assignments",
			wantText: "published",
		},
		{
			name: "summary",
			interaction: assignmentInteraction("",
				stringInteractionOption("course", "course-1"),
				stringInteractionOption("id", assignmentID),
			),
			wantPath: "/v2/admin/courses/course-1/assignments/" + assignmentID,
			wantText: assignmentID,
		},
		{
			name: "check",
			interaction: assignmentInteraction("check",
				stringInteractionOption("course", "course-1"),
				stringInteractionOption("id", assignmentID),
			),
			wantPath: "/v2/admin/courses/course-1/assignments/" + assignmentID,
			wantText: "check",
		},
		{
			name: "submissions",
			interaction: assignmentInteraction("submissions",
				stringInteractionOption("course", "course-1"),
				stringInteractionOption("id", assignmentID),
			),
			wantPath: "/v2/admin/courses/course-1/assignments/" + assignmentID + "/submission-statuses",
			wantText: "submitted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := h.assignmentCommand(context.Background(), tc.interaction)
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.wantText)) {
				t.Fatalf("response does not contain %q: %s", tc.wantText, got)
			}
			if !requestedPaths.contains(tc.wantPath) {
				t.Fatalf("admin endpoint was not requested: %s; got %#v", tc.wantPath, requestedPaths.snapshot())
			}
		})
	}
}

type pathRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (r *pathRecorder) add(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

func (r *pathRecorder) contains(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.paths {
		if got == path {
			return true
		}
	}
	return false
}

func (r *pathRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func assignmentQueryServer(t *testing.T, assignmentID string) (*httptest.Server, *pathRecorder) {
	t.Helper()
	requested := &pathRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested.add(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/auth/refresh":
			_, _ = w.Write([]byte(`{"data":{"accessToken":"access-token"}}`))
		case "/v2/admin/courses":
			_, _ = w.Write([]byte(`{"data":[{"courseSlug":"course-1","status":"OPEN"}]}`))
		case "/v2/admin/courses/course-1/assignments":
			_, _ = w.Write([]byte(`{"data":{"assignments":[{"assignmentId":"` + assignmentID + `","title":"Contract","status":"published","problemId":"p1"}]}}`))
		case "/v2/admin/courses/course-1/assignments/" + assignmentID:
			_, _ = w.Write([]byte(`{"data":{"assignmentId":"` + assignmentID + `","title":"Contract","status":"published","problemId":"p1"}}`))
		case "/v2/admin/courses/course-1/assignments/" + assignmentID + "/submission-statuses":
			_, _ = w.Write([]byte(`{"data":[{"submitted":true,"status":"GRADED","score":90},{"submitted":false,"status":"PENDING"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	return server, requested
}
