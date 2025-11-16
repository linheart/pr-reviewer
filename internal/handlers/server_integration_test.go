package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pr-reviewer/internal/api"
	"pr-reviewer/internal/migrations"
	"pr-reviewer/internal/repo"
	"pr-reviewer/internal/service"
)

func TestIntegration_FullFlow(t *testing.T) {
	app := newIntegrationApp(t)
	defer app.Close()

	app.postJSON("/team/add", http.StatusCreated, map[string]any{
		"team_name": "backend",
		"members": []map[string]any{
			{"user_id": "u1", "username": "Alice", "is_active": true},
			{"user_id": "u2", "username": "Bob", "is_active": true},
			{"user_id": "u3", "username": "Charlie", "is_active": true},
			{"user_id": "u4", "username": "Dora", "is_active": true},
		},
	})

	var backend api.Team
	app.decodeResponse(app.getJSON("/team/get?team_name=backend", http.StatusOK), &backend)
	if len(backend.Members) != 4 {
		t.Fatalf("expected 4 members, got %d", len(backend.Members))
	}

	app.expectAPIError(http.StatusBadRequest, api.TEAMEXISTS, "/team/add", map[string]any{
		"team_name": "backend",
		"members": []map[string]any{
			{"user_id": "u1", "username": "Alice", "is_active": true},
		},
	})
	app.expectGETError("/team/get?team_name=unknown-team", http.StatusNotFound, api.NOTFOUND)

	app.expectAPIError(http.StatusNotFound, api.NOTFOUND, "/pullRequest/create", map[string]string{
		"pull_request_id":   "pr-missing-author",
		"pull_request_name": "No Author",
		"author_id":         "ghost",
	})

	var pr1 struct {
		PR api.PullRequest `json:"pr"`
	}
	app.decodeResponse(app.postJSON("/pullRequest/create", http.StatusCreated, map[string]any{
		"pull_request_id":   "pr-1",
		"pull_request_name": "Initial",
		"author_id":         "u1",
	}), &pr1)
	if len(pr1.PR.AssignedReviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d", len(pr1.PR.AssignedReviewers))
	}

	app.expectAPIError(http.StatusConflict, api.PREXISTS, "/pullRequest/create", map[string]any{
		"pull_request_id":   "pr-1",
		"pull_request_name": "Initial",
		"author_id":         "u1",
	})

	candidates := []string{"u2", "u3", "u4"}
	spare := spareReviewer(candidates, pr1.PR.AssignedReviewers)
	if spare == "" {
		t.Fatalf("expected spare candidate, assigned=%v", pr1.PR.AssignedReviewers)
	}
	oldReviewer := pr1.PR.AssignedReviewers[0]

	var reassignResp struct {
		PR         api.PullRequest `json:"pr"`
		ReplacedBy string          `json:"replaced_by"`
	}
	app.decodeResponse(app.postJSON("/pullRequest/reassign", http.StatusOK, map[string]string{
		"pull_request_id": "pr-1",
		"old_user_id":     oldReviewer,
	}), &reassignResp)
	if reassignResp.ReplacedBy != spare {
		t.Fatalf("expected replacement by %s, got %s", spare, reassignResp.ReplacedBy)
	}

	app.expectAPIError(http.StatusConflict, api.NOTASSIGNED, "/pullRequest/reassign", map[string]string{
		"pull_request_id": "pr-1",
		"old_user_id":     oldReviewer,
	})

	var merged struct {
		PR api.PullRequest `json:"pr"`
	}
	app.decodeResponse(app.postJSON("/pullRequest/merge", http.StatusOK, map[string]string{
		"pull_request_id": "pr-1",
	}), &merged)
	if merged.PR.Status != api.PullRequestStatusMERGED {
		t.Fatalf("expected MERGED status, got %s", merged.PR.Status)
	}
	if merged.PR.MergedAt == nil {
		t.Fatalf("expected mergedAt to be set")
	}

	var mergedAgain struct {
		PR api.PullRequest `json:"pr"`
	}
	app.decodeResponse(app.postJSON("/pullRequest/merge", http.StatusOK, map[string]string{
		"pull_request_id": "pr-1",
	}), &mergedAgain)
	if mergedAgain.PR.Status != api.PullRequestStatusMERGED {
		t.Fatalf("expected MERGED status on repeat merge")
	}

	app.expectAPIError(http.StatusConflict, api.PRMERGED, "/pullRequest/reassign", map[string]string{
		"pull_request_id": "pr-1",
		"old_user_id":     reassignResp.PR.AssignedReviewers[0],
	})

	var reviews struct {
		UserID       string                 `json:"user_id"`
		PullRequests []api.PullRequestShort `json:"pull_requests"`
	}
	app.decodeResponse(app.getJSON("/users/getReview?user_id="+spare, http.StatusOK), &reviews)
	if reviews.UserID != spare {
		t.Fatalf("expected user_id %s, got %s", spare, reviews.UserID)
	}
	if !containsPR(reviews.PullRequests, "pr-1", api.PullRequestShortStatusMERGED) {
		t.Fatalf("expected pull_requests to contain pr-1 with MERGED status: %+v", reviews.PullRequests)
	}

	var updated api.User
	app.decodeResponse(app.postJSON("/users/setIsActive", http.StatusOK, map[string]any{
		"user_id":   "u2",
		"is_active": false,
	}), &updated)
	if updated.UserId != "u2" || updated.IsActive {
		t.Fatalf("expected u2 to become inactive, got %+v", updated)
	}
	app.decodeResponse(app.postJSON("/users/setIsActive", http.StatusOK, map[string]any{
		"user_id":   "u2",
		"is_active": true,
	}), &updated)
	if !updated.IsActive {
		t.Fatalf("expected u2 to become active again")
	}
	app.expectAPIError(http.StatusNotFound, api.NOTFOUND, "/users/setIsActive", map[string]any{
		"user_id":   "ghost-user",
		"is_active": true,
	})

	app.postJSON("/team/add", http.StatusCreated, map[string]any{
		"team_name": "solo",
		"members": []map[string]any{
			{"user_id": "solo-1", "username": "Solo", "is_active": true},
		},
	})

	var prZero struct {
		PR api.PullRequest `json:"pr"`
	}
	app.decodeResponse(app.postJSON("/pullRequest/create", http.StatusCreated, map[string]string{
		"pull_request_id":   "pr-0",
		"pull_request_name": "Solo PR",
		"author_id":         "solo-1",
	}), &prZero)
	if len(prZero.PR.AssignedReviewers) != 0 {
		t.Fatalf("expected 0 reviewers, got %d", len(prZero.PR.AssignedReviewers))
	}

	app.postJSON("/team/add", http.StatusCreated, map[string]any{
		"team_name": "pair",
		"members": []map[string]any{
			{"user_id": "pair-1", "username": "PairAuthor", "is_active": true},
			{"user_id": "pair-2", "username": "PairReviewer", "is_active": true},
		},
	})

	var prOne struct {
		PR api.PullRequest `json:"pr"`
	}
	app.decodeResponse(app.postJSON("/pullRequest/create", http.StatusCreated, map[string]string{
		"pull_request_id":   "pr-1-one",
		"pull_request_name": "Pair Work",
		"author_id":         "pair-1",
	}), &prOne)
	if len(prOne.PR.AssignedReviewers) != 1 {
		t.Fatalf("expected 1 reviewer, got %d", len(prOne.PR.AssignedReviewers))
	}
	if prOne.PR.AssignedReviewers[0] != "pair-2" {
		t.Fatalf("expected reviewer pair-2, got %v", prOne.PR.AssignedReviewers)
	}

	app.expectAPIError(http.StatusConflict, api.NOCANDIDATE, "/pullRequest/reassign", map[string]string{
		"pull_request_id": "pr-1-one",
		"old_user_id":     "pair-2",
	})

	var pairReviews struct {
		UserID       string                 `json:"user_id"`
		PullRequests []api.PullRequestShort `json:"pull_requests"`
	}
	app.decodeResponse(app.getJSON("/users/getReview?user_id=pair-2", http.StatusOK), &pairReviews)
	if !containsPR(pairReviews.PullRequests, "pr-1-one", api.PullRequestShortStatusOPEN) {
		t.Fatalf("expected pull_requests to contain pr-1-one: %+v", pairReviews.PullRequests)
	}

	var emptyReviews struct {
		UserID       string                 `json:"user_id"`
		PullRequests []api.PullRequestShort `json:"pull_requests"`
	}
	app.decodeResponse(app.getJSON("/users/getReview?user_id=unknown-user", http.StatusOK), &emptyReviews)
	if len(emptyReviews.PullRequests) != 0 {
		t.Fatalf("expected no pull requests for unknown user: %+v", emptyReviews.PullRequests)
	}

	app.expectAPIError(http.StatusNotFound, api.NOTFOUND, "/pullRequest/merge", map[string]string{
		"pull_request_id": "pr-missing",
	})
	app.expectAPIError(http.StatusNotFound, api.NOTFOUND, "/pullRequest/reassign", map[string]string{
		"pull_request_id": "pr-missing",
		"old_user_id":     "u2",
	})
}

type integrationApp struct {
	client  *http.Client
	server  *httptest.Server
	cleanup func()
	baseURL string
	t       *testing.T
}

func newIntegrationApp(t *testing.T) *integrationApp {
	t.Helper()

	dsn, stopContainer := startPostgresContainer(t)

	ctx := context.Background()
	pool, err := waitForPool(ctx, dsn)
	if err != nil {
		stopContainer()
		t.Fatalf("connect to postgres: %v", err)
	}
	if err := migrations.Run(ctx, pool); err != nil {
		pool.Close()
		stopContainer()
		t.Fatalf("apply migrations: %v", err)
	}

	repository := repo.NewRepo(pool)
	svc := service.NewService(repository)
	apiServer := NewServer(svc)

	httpSrv := httptest.NewServer(api.Handler(apiServer))

	cleanup := func() {
		httpSrv.Close()
		pool.Close()
		stopContainer()
	}

	return &integrationApp{
		client:  httpSrv.Client(),
		server:  httpSrv,
		cleanup: cleanup,
		baseURL: httpSrv.URL,
		t:       t,
	}
}

func (a *integrationApp) Close() {
	a.cleanup()
}

func (a *integrationApp) postJSON(path string, wantStatus int, payload any) []byte {
	a.t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		a.t.Fatalf("marshal payload: %v", err)
	}

	resp, err := a.client.Post(a.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		a.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode != wantStatus {
		a.t.Fatalf("POST %s: expected %d, got %d: %s", path, wantStatus, resp.StatusCode, body)
	}

	return body
}

func (a *integrationApp) getJSON(path string, wantStatus int) []byte {
	a.t.Helper()

	resp, err := a.client.Get(a.baseURL + path)
	if err != nil {
		a.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode != wantStatus {
		a.t.Fatalf("GET %s: expected %d, got %d: %s", path, wantStatus, resp.StatusCode, body)
	}

	return body
}

func (a *integrationApp) expectAPIError(wantStatus int, wantCode api.ErrorResponseErrorCode, path string, payload any) {
	a.t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		a.t.Fatalf("marshal payload: %v", err)
	}

	resp, err := a.client.Post(a.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		a.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode != wantStatus {
		a.t.Fatalf("POST %s: expected status %d, got %d: %s", path, wantStatus, resp.StatusCode, body)
	}

	var apiErr struct {
		Error struct {
			Code api.ErrorResponseErrorCode `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &apiErr); err != nil {
		a.t.Fatalf("unmarshal error response: %v (%s)", err, body)
	}
	if apiErr.Error.Code != wantCode {
		a.t.Fatalf("expected error code %s, got %s", wantCode, apiErr.Error.Code)
	}
}

func (a *integrationApp) expectGETError(path string, wantStatus int, wantCode api.ErrorResponseErrorCode) {
	a.t.Helper()

	resp, err := a.client.Get(a.baseURL + path)
	if err != nil {
		a.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode != wantStatus {
		a.t.Fatalf("GET %s: expected status %d, got %d: %s", path, wantStatus, resp.StatusCode, body)
	}

	var apiErr struct {
		Error struct {
			Code api.ErrorResponseErrorCode `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &apiErr); err != nil {
		a.t.Fatalf("unmarshal error response: %v (%s)", err, body)
	}
	if apiErr.Error.Code != wantCode {
		a.t.Fatalf("expected error code %s, got %s", wantCode, apiErr.Error.Code)
	}
}

func (a *integrationApp) decodeResponse(data []byte, dst any) {
	a.t.Helper()
	if err := json.Unmarshal(data, dst); err != nil {
		a.t.Fatalf("decode response: %v (%s)", err, data)
	}
}

func spareReviewer(candidates, assigned []string) string {
	set := make(map[string]struct{}, len(assigned))
	for _, id := range assigned {
		set[id] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, ok := set[candidate]; !ok {
			return candidate
		}
	}
	return ""
}

func containsPR(list []api.PullRequestShort, prID string, status api.PullRequestShortStatus) bool {
	for _, pr := range list {
		if pr.PullRequestId == prID && pr.Status == status {
			return true
		}
	}
	return false
}

func startPostgresContainer(t *testing.T) (string, func()) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI is required for integration tests")
	}

	run := exec.Command(
		"docker", "run", "-d", "--rm",
		"-e", "POSTGRES_PASSWORD=postgres",
		"-e", "POSTGRES_USER=postgres",
		"-e", "POSTGRES_DB=app",
		"-P",
		"postgres:18",
	)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("docker run postgres: %v (%s)", err, out)
	}
	containerID := strings.TrimSpace(string(out))

	stop := func() {
		_ = exec.Command("docker", "stop", containerID).Run()
	}

	port := fetchMappedPort(t, containerID)
	waitForPostgres(t, containerID)

	dsn := fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:%s/app?sslmode=disable", port)
	return dsn, stop
}

func fetchMappedPort(t *testing.T, containerID string) string {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command("docker", "port", containerID, "5432/tcp")
		out, err := cmd.CombinedOutput()
		if err == nil && len(out) > 0 {
			line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
			if idx := strings.LastIndex(line, ":"); idx != -1 && idx+1 < len(line) {
				return line[idx+1:]
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("failed to determine mapped port for container %s", containerID)
	return ""
}

func waitForPostgres(t *testing.T, containerID string) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command(
			"docker", "exec",
			"-e", "PGPASSWORD=postgres",
			containerID,
			"pg_isready", "-U", "postgres", "-d", "app",
		)
		if err := cmd.Run(); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("postgres container %s did not become ready in time", containerID)
}

func waitForPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	const maxAttempts = 10
	for attempt := 0; attempt < maxAttempts; attempt++ {
		pool, err := repo.NewPool(ctx, dsn)
		if err == nil {
			return pool, nil
		}
		if attempt == maxAttempts-1 {
			return nil, fmt.Errorf("unable to connect to postgres: %w", err)
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("unable to connect to postgres")
}
