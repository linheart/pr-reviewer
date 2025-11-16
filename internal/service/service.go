package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"pr-reviewer/internal/api"
	"pr-reviewer/internal/repo"
)

type Repository interface {
	CreateTeamWithMembers(ctx context.Context, team api.Team) (api.Team, error)
	GetTeam(ctx context.Context, teamName string) (api.Team, error)

	SetUserActive(ctx context.Context, userID string, isActive bool) (api.User, error)
	GetUser(ctx context.Context, userID string) (api.User, error)
	ListActiveUsersInTeam(ctx context.Context, teamName string) ([]api.User, error)

	PullRequestExists(ctx context.Context, prID string) (bool, error)
	CreatePullRequest(ctx context.Context, prID, prName, authorID string, reviewerIDs []string) (api.PullRequest, error)
	GetPullRequest(ctx context.Context, prID string) (api.PullRequest, error)
	MarkPullRequestMerged(ctx context.Context, prID string) (api.PullRequest, error)
	ReplaceReviewer(ctx context.Context, prID, oldUserID, newUserID string) error
	ListUserReviewPRs(ctx context.Context, userID string) ([]api.PullRequestShort, error)
}

var _ Repository = (*repo.Repo)(nil)

type Service struct {
	repo Repository
	rng  *rand.Rand
}

func NewService(r Repository) *Service {
	return &Service{
		repo: r,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type Error struct {
	Code api.ErrorResponseErrorCode
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func NewError(code api.ErrorResponseErrorCode, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

func (s *Service) CreateTeam(ctx context.Context, team api.Team) (api.Team, error) {
	created, err := s.repo.CreateTeamWithMembers(ctx, team)
	if err != nil {
		if err == repo.ErrTeamExists {
			return api.Team{}, NewError(api.TEAMEXISTS, "team already exists")
		}
		return api.Team{}, err
	}
	return created, nil
}

func (s *Service) GetTeam(ctx context.Context, teamName string) (api.Team, error) {
	team, err := s.repo.GetTeam(ctx, teamName)
	if err != nil {
		if err == repo.ErrNotFound {
			return api.Team{}, NewError(api.NOTFOUND, "team not found")
		}
		return api.Team{}, err
	}
	return team, nil
}

func (s *Service) SetUserActive(ctx context.Context, userID string, isActive bool) (api.User, error) {
	user, err := s.repo.SetUserActive(ctx, userID, isActive)
	if err != nil {
		if err == repo.ErrNotFound {
			return api.User{}, NewError(api.NOTFOUND, "user not found")
		}
		return api.User{}, err
	}
	return user, nil
}

func (s *Service) CreatePullRequest(ctx context.Context, id, name, authorID string) (api.PullRequest, error) {
	exists, err := s.repo.PullRequestExists(ctx, id)
	if err != nil {
		return api.PullRequest{}, err
	}
	if exists {
		return api.PullRequest{}, NewError(api.PREXISTS, "pull request already exists")
	}

	author, err := s.repo.GetUser(ctx, authorID)
	if err != nil {
		if err == repo.ErrNotFound {
			return api.PullRequest{}, NewError(api.NOTFOUND, "author not found")
		}
		return api.PullRequest{}, err
	}

	users, err := s.repo.ListActiveUsersInTeam(ctx, author.TeamName)
	if err != nil {
		return api.PullRequest{}, err
	}

	var candidates []string
	for _, u := range users {
		if u.UserId == author.UserId {
			continue
		}
		candidates = append(candidates, u.UserId)
	}

	reviewers := pickRandom(s.rng, candidates, 2)

	pr, err := s.repo.CreatePullRequest(ctx, id, name, authorID, reviewers)
	if err != nil {
		return api.PullRequest{}, err
	}
	return pr, nil
}

func (s *Service) MergePullRequest(ctx context.Context, prID string) (api.PullRequest, error) {
	pr, err := s.repo.GetPullRequest(ctx, prID)
	if err != nil {
		if err == repo.ErrNotFound {
			return api.PullRequest{}, NewError(api.NOTFOUND, "pull request not found")
		}
		return api.PullRequest{}, err
	}

	if pr.Status == api.PullRequestStatusMERGED {
		return pr, nil
	}

	return s.repo.MarkPullRequestMerged(ctx, prID)
}

func (s *Service) ReassignReviewer(ctx context.Context, prID, oldUserID string) (api.PullRequest, string, error) {
	pr, err := s.repo.GetPullRequest(ctx, prID)
	if err != nil {
		if err == repo.ErrNotFound {
			return api.PullRequest{}, "", NewError(api.NOTFOUND, "pull request not found")
		}
		return api.PullRequest{}, "", err
	}

	if pr.Status == api.PullRequestStatusMERGED {
		return api.PullRequest{}, "", NewError(api.PRMERGED, "cannot reassign on merged PR")
	}

	assigned := false
	for _, rid := range pr.AssignedReviewers {
		if rid == oldUserID {
			assigned = true
			break
		}
	}
	if !assigned {
		return api.PullRequest{}, "", NewError(api.NOTASSIGNED, "reviewer is not assigned to this PR")
	}

	oldUser, err := s.repo.GetUser(ctx, oldUserID)
	if err != nil {
		if err == repo.ErrNotFound {
			return api.PullRequest{}, "", NewError(api.NOTFOUND, "reviewer not found")
		}
		return api.PullRequest{}, "", err
	}

	users, err := s.repo.ListActiveUsersInTeam(ctx, oldUser.TeamName)
	if err != nil {
		return api.PullRequest{}, "", err
	}

	exclude := map[string]struct{}{
		pr.AuthorId: {},
		oldUserID:   {},
	}
	for _, rid := range pr.AssignedReviewers {
		exclude[rid] = struct{}{}
	}

	var candidates []string
	for _, u := range users {
		if _, skip := exclude[u.UserId]; skip {
			continue
		}
		candidates = append(candidates, u.UserId)
	}

	if len(candidates) == 0 {
		return api.PullRequest{}, "", NewError(api.NOCANDIDATE, "no active replacement candidate in team")
	}

	newID := pickRandom(s.rng, candidates, 1)[0]

	if err := s.repo.ReplaceReviewer(ctx, prID, oldUserID, newID); err != nil {
		if err == repo.ErrReviewerNotFound {
			return api.PullRequest{}, "", NewError(api.NOTASSIGNED, "reviewer is not assigned to this PR")
		}
		return api.PullRequest{}, "", err
	}

	updated, err := s.repo.GetPullRequest(ctx, prID)
	if err != nil {
		return api.PullRequest{}, "", err
	}

	return updated, newID, nil
}

func (s *Service) ListUserReviewPRs(ctx context.Context, userID string) ([]api.PullRequestShort, error) {
	return s.repo.ListUserReviewPRs(ctx, userID)
}

func pickRandom(rng *rand.Rand, items []string, max int) []string {
	if len(items) <= max {
		out := make([]string, len(items))
		copy(out, items)
		return out
	}
	out := make([]string, max)
	perm := rng.Perm(len(items))
	for i := 0; i < max; i++ {
		out[i] = items[perm[i]]
	}
	return out
}
