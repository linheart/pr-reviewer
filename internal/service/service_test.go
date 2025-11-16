package service

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"pr-reviewer/internal/api"
	"pr-reviewer/internal/repo"
)

type mockRepo struct {
	createTeamWithMembers func(context.Context, api.Team) (api.Team, error)
	getTeam               func(context.Context, string) (api.Team, error)
	setUserActive         func(context.Context, string, bool) (api.User, error)
	getUser               func(context.Context, string) (api.User, error)
	listActiveUsersInTeam func(context.Context, string) ([]api.User, error)
	pullRequestExists     func(context.Context, string) (bool, error)
	createPullRequest     func(context.Context, string, string, string, []string) (api.PullRequest, error)
	getPullRequest        func(context.Context, string) (api.PullRequest, error)
	markPullRequestMerged func(context.Context, string) (api.PullRequest, error)
	replaceReviewer       func(context.Context, string, string, string) error
	listUserReviewPRs     func(context.Context, string) ([]api.PullRequestShort, error)
}

func (m *mockRepo) CreateTeamWithMembers(ctx context.Context, team api.Team) (api.Team, error) {
	return m.createTeamWithMembers(ctx, team)
}

func (m *mockRepo) GetTeam(ctx context.Context, name string) (api.Team, error) {
	return m.getTeam(ctx, name)
}

func (m *mockRepo) SetUserActive(ctx context.Context, id string, active bool) (api.User, error) {
	return m.setUserActive(ctx, id, active)
}

func (m *mockRepo) GetUser(ctx context.Context, id string) (api.User, error) {
	return m.getUser(ctx, id)
}

func (m *mockRepo) ListActiveUsersInTeam(ctx context.Context, team string) ([]api.User, error) {
	return m.listActiveUsersInTeam(ctx, team)
}

func (m *mockRepo) PullRequestExists(ctx context.Context, id string) (bool, error) {
	return m.pullRequestExists(ctx, id)
}

func (m *mockRepo) CreatePullRequest(ctx context.Context, id, name, authorID string, reviewers []string) (api.PullRequest, error) {
	return m.createPullRequest(ctx, id, name, authorID, reviewers)
}

func (m *mockRepo) GetPullRequest(ctx context.Context, id string) (api.PullRequest, error) {
	return m.getPullRequest(ctx, id)
}

func (m *mockRepo) MarkPullRequestMerged(ctx context.Context, id string) (api.PullRequest, error) {
	return m.markPullRequestMerged(ctx, id)
}

func (m *mockRepo) ReplaceReviewer(ctx context.Context, prID, oldUserID, newUserID string) error {
	return m.replaceReviewer(ctx, prID, oldUserID, newUserID)
}

func (m *mockRepo) ListUserReviewPRs(ctx context.Context, userID string) ([]api.PullRequestShort, error) {
	return m.listUserReviewPRs(ctx, userID)
}

func TestService_CreatePullRequest_PRExists(t *testing.T) {
	svc := newTestService(&mockRepo{
		pullRequestExists: func(context.Context, string) (bool, error) {
			return true, nil
		},
	})

	_, err := svc.CreatePullRequest(context.Background(), "pr-1", "Add feature", "author-1")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	sErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected service.Error, got %T", err)
	}
	if sErr.Code != api.PREXISTS {
		t.Fatalf("expected code %s, got %s", api.PREXISTS, sErr.Code)
	}
}

func TestService_CreatePullRequest_AuthorNotFound(t *testing.T) {
	svc := newTestService(&mockRepo{
		pullRequestExists: func(context.Context, string) (bool, error) {
			return false, nil
		},
		getUser: func(context.Context, string) (api.User, error) {
			return api.User{}, repo.ErrNotFound
		},
	})

	_, err := svc.CreatePullRequest(context.Background(), "pr-2", "Add", "missing-author")
	assertServiceErrorCode(t, err, api.NOTFOUND)
}

func TestService_ReassignReviewer_PRMerged(t *testing.T) {
	svc := newTestService(&mockRepo{
		getPullRequest: func(context.Context, string) (api.PullRequest, error) {
			return api.PullRequest{
				PullRequestId:     "pr-1",
				AuthorId:          "author",
				Status:            api.PullRequestStatusMERGED,
				AssignedReviewers: []string{"u1", "u2"},
			}, nil
		},
	})

	_, _, err := svc.ReassignReviewer(context.Background(), "pr-1", "u1")
	assertServiceErrorCode(t, err, api.PRMERGED)
}

func TestService_ReassignReviewer_NotAssigned(t *testing.T) {
	svc := newTestService(&mockRepo{
		getPullRequest: func(context.Context, string) (api.PullRequest, error) {
			return api.PullRequest{
				PullRequestId:     "pr-1",
				AuthorId:          "author",
				Status:            api.PullRequestStatusOPEN,
				AssignedReviewers: []string{"u2", "u3"},
			}, nil
		},
	})

	_, _, err := svc.ReassignReviewer(context.Background(), "pr-1", "u1")
	assertServiceErrorCode(t, err, api.NOTASSIGNED)
}

func TestService_ReassignReviewer_NoCandidate(t *testing.T) {
	svc := newTestService(&mockRepo{
		getPullRequest: func(context.Context, string) (api.PullRequest, error) {
			return api.PullRequest{
				PullRequestId:     "pr-1",
				AuthorId:          "author",
				Status:            api.PullRequestStatusOPEN,
				AssignedReviewers: []string{"u1"},
			}, nil
		},
		getUser: func(context.Context, string) (api.User, error) {
			return api.User{UserId: "u1", TeamName: "team"}, nil
		},
		listActiveUsersInTeam: func(context.Context, string) ([]api.User, error) {
			return []api.User{
				{UserId: "u1", TeamName: "team", IsActive: true},
			}, nil
		},
	})

	_, _, err := svc.ReassignReviewer(context.Background(), "pr-1", "u1")
	assertServiceErrorCode(t, err, api.NOCANDIDATE)
}

func newTestService(r Repository) *Service {
	svc := NewService(r)
	svc.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	return svc
}

func assertServiceErrorCode(t *testing.T, err error, code api.ErrorResponseErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	sErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected service.Error, got %T", err)
	}
	if sErr.Code != code {
		t.Fatalf("expected code %s, got %s", code, sErr.Code)
	}
}
