package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pr-reviewer/internal/api"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrTeamExists        = errors.New("team already exists")
	ErrPullRequestExists = errors.New("pull request already exists")
	ErrReviewerNotFound  = errors.New("reviewer not found for this PR")
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) CreateTeamWithMembers(ctx context.Context, team api.Team) (api.Team, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return api.Team{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)`,
		team.TeamName,
	).Scan(&exists); err != nil {
		return api.Team{}, fmt.Errorf("check team exists: %w", err)
	}
	if exists {
		return api.Team{}, ErrTeamExists
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO teams (team_name) VALUES ($1)`,
		team.TeamName,
	); err != nil {
		return api.Team{}, fmt.Errorf("insert team: %w", err)
	}

	for _, m := range team.Members {
		if _, err := tx.Exec(ctx,
			`INSERT INTO users (user_id, username, team_name, is_active)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (user_id) DO UPDATE
			   SET username = EXCLUDED.username,
			       team_name = EXCLUDED.team_name,
			       is_active = EXCLUDED.is_active`,
			m.UserId, m.Username, team.TeamName, m.IsActive,
		); err != nil {
			return api.Team{}, fmt.Errorf("upsert user %s: %w", m.UserId, err)
		}
	}

	loaded, err := loadTeamTx(ctx, tx, team.TeamName)
	if err != nil {
		return api.Team{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return api.Team{}, fmt.Errorf("commit tx: %w", err)
	}
	return loaded, nil
}

func loadTeamTx(ctx context.Context, tx pgx.Tx, teamName string) (api.Team, error) {
	rows, err := tx.Query(ctx,
		`SELECT user_id, username, is_active
		   FROM users
		  WHERE team_name = $1
		  ORDER BY user_id`,
		teamName,
	)
	if err != nil {
		return api.Team{}, fmt.Errorf("select members: %w", err)
	}
	defer rows.Close()

	var members []api.TeamMember
	for rows.Next() {
		var id, username string
		var active bool
		if err := rows.Scan(&id, &username, &active); err != nil {
			return api.Team{}, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, api.TeamMember{
			UserId:   id,
			Username: username,
			IsActive: active,
		})
	}
	if err := rows.Err(); err != nil {
		return api.Team{}, fmt.Errorf("rows err: %w", err)
	}

	return api.Team{
		TeamName: teamName,
		Members:  members,
	}, nil
}

func (r *Repo) GetTeam(ctx context.Context, teamName string) (api.Team, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)`,
		teamName,
	).Scan(&exists); err != nil {
		return api.Team{}, fmt.Errorf("check team: %w", err)
	}
	if !exists {
		return api.Team{}, ErrNotFound
	}

	rows, err := r.pool.Query(ctx,
		`SELECT user_id, username, is_active
		   FROM users
		  WHERE team_name = $1
		  ORDER BY user_id`,
		teamName,
	)
	if err != nil {
		return api.Team{}, fmt.Errorf("select members: %w", err)
	}
	defer rows.Close()

	var members []api.TeamMember
	for rows.Next() {
		var id, username string
		var active bool
		if err := rows.Scan(&id, &username, &active); err != nil {
			return api.Team{}, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, api.TeamMember{
			UserId:   id,
			Username: username,
			IsActive: active,
		})
	}
	if err := rows.Err(); err != nil {
		return api.Team{}, fmt.Errorf("rows err: %w", err)
	}

	return api.Team{
		TeamName: teamName,
		Members:  members,
	}, nil
}

func (r *Repo) SetUserActive(ctx context.Context, userID string, isActive bool) (api.User, error) {
	var id, username, teamName string
	var active bool

	err := r.pool.QueryRow(ctx,
		`UPDATE users
		    SET is_active = $2
		  WHERE user_id = $1
		  RETURNING user_id, username, team_name, is_active`,
		userID, isActive,
	).Scan(&id, &username, &teamName, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.User{}, ErrNotFound
	}
	if err != nil {
		return api.User{}, fmt.Errorf("update user: %w", err)
	}

	return api.User{
		UserId:   id,
		Username: username,
		TeamName: teamName,
		IsActive: active,
	}, nil
}

func (r *Repo) GetUser(ctx context.Context, userID string) (api.User, error) {
	var id, username, teamName string
	var active bool

	err := r.pool.QueryRow(ctx,
		`SELECT user_id, username, team_name, is_active
		   FROM users
		  WHERE user_id = $1`,
		userID,
	).Scan(&id, &username, &teamName, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.User{}, ErrNotFound
	}
	if err != nil {
		return api.User{}, fmt.Errorf("get user: %w", err)
	}

	return api.User{
		UserId:   id,
		Username: username,
		TeamName: teamName,
		IsActive: active,
	}, nil
}

func (r *Repo) ListActiveUsersInTeam(ctx context.Context, teamName string) ([]api.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, username, team_name, is_active
		   FROM users
		  WHERE team_name = $1
		    AND is_active = true`,
		teamName,
	)
	if err != nil {
		return nil, fmt.Errorf("select active users: %w", err)
	}
	defer rows.Close()

	var users []api.User
	for rows.Next() {
		var id, username, tname string
		var active bool
		if err := rows.Scan(&id, &username, &tname, &active); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, api.User{
			UserId:   id,
			Username: username,
			TeamName: tname,
			IsActive: active,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return users, nil
}

func (r *Repo) PullRequestExists(ctx context.Context, prID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pull_requests WHERE pull_request_id = $1)`,
		prID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check pr exists: %w", err)
	}
	return exists, nil
}

func (r *Repo) CreatePullRequest(ctx context.Context, prID, prName, authorID string, reviewerIDs []string) (api.PullRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return api.PullRequest{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status)
		 VALUES ($1, $2, $3, 'OPEN')`,
		prID, prName, authorID,
	)
	if err != nil {
		return api.PullRequest{}, fmt.Errorf("insert pr: %w", err)
	}

	for i, rid := range reviewerIDs {
		slot := int16(i + 1)
		if _, err := tx.Exec(ctx,
			`INSERT INTO pr_reviewers (pr_id, slot, reviewer_id)
			 VALUES ($1, $2, $3)`,
			prID, slot, rid,
		); err != nil {
			return api.PullRequest{}, fmt.Errorf("insert reviewer: %w", err)
		}
	}

	pr, err := loadPullRequestTx(ctx, tx, prID)
	if err != nil {
		return api.PullRequest{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return api.PullRequest{}, fmt.Errorf("commit tx: %w", err)
	}
	return pr, nil
}

func loadPullRequestTx(ctx context.Context, tx pgx.Tx, prID string) (api.PullRequest, error) {
	var id, name, authorID, statusStr string
	var createdAt time.Time
	var mergedAt *time.Time

	err := tx.QueryRow(ctx,
		`SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		   FROM pull_requests
		  WHERE pull_request_id = $1`,
		prID,
	).Scan(&id, &name, &authorID, &statusStr, &createdAt, &mergedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.PullRequest{}, ErrNotFound
	}
	if err != nil {
		return api.PullRequest{}, fmt.Errorf("get pr: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT reviewer_id
		   FROM pr_reviewers
		  WHERE pr_id = $1
		  ORDER BY slot`,
		prID,
	)
	if err != nil {
		return api.PullRequest{}, fmt.Errorf("select reviewers: %w", err)
	}
	defer rows.Close()

	var reviewers []string
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			return api.PullRequest{}, fmt.Errorf("scan reviewer: %w", err)
		}
		reviewers = append(reviewers, rid)
	}
	if err := rows.Err(); err != nil {
		return api.PullRequest{}, fmt.Errorf("rows err: %w", err)
	}

	status := api.PullRequestStatus(statusStr)

	pr := api.PullRequest{
		PullRequestId:     id,
		PullRequestName:   name,
		AuthorId:          authorID,
		Status:            status,
		AssignedReviewers: reviewers,
	}
	pr.CreatedAt = &createdAt
	pr.MergedAt = mergedAt

	return pr, nil
}

func (r *Repo) GetPullRequest(ctx context.Context, prID string) (api.PullRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return api.PullRequest{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	pr, err := loadPullRequestTx(ctx, tx, prID)
	if err != nil {
		return api.PullRequest{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return api.PullRequest{}, fmt.Errorf("commit: %w", err)
	}
	return pr, nil
}

func (r *Repo) MarkPullRequestMerged(ctx context.Context, prID string) (api.PullRequest, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return api.PullRequest{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx,
		`UPDATE pull_requests
		    SET status = 'MERGED',
		        merged_at = COALESCE(merged_at, now())
		  WHERE pull_request_id = $1`,
		prID,
	)
	if err != nil {
		return api.PullRequest{}, fmt.Errorf("update pr: %w", err)
	}

	pr, err := loadPullRequestTx(ctx, tx, prID)
	if err != nil {
		return api.PullRequest{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return api.PullRequest{}, fmt.Errorf("commit: %w", err)
	}
	return pr, nil
}

func (r *Repo) ReplaceReviewer(ctx context.Context, prID, oldUserID, newUserID string) error {
	cmd, err := r.pool.Exec(ctx,
		`UPDATE pr_reviewers
		    SET reviewer_id = $3
		  WHERE pr_id = $1
		    AND reviewer_id = $2`,
		prID, oldUserID, newUserID,
	)
	if err != nil {
		return fmt.Errorf("update reviewer: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrReviewerNotFound
	}
	return nil
}

func (r *Repo) ListUserReviewPRs(ctx context.Context, userID string) ([]api.PullRequestShort, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT pr.pull_request_id,
		        pr.pull_request_name,
		        pr.author_id,
		        pr.status
		   FROM pull_requests pr
		   JOIN pr_reviewers r ON pr.pull_request_id = r.pr_id
		  WHERE r.reviewer_id = $1
		  ORDER BY pr.created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("select prs: %w", err)
	}
	defer rows.Close()

	var res []api.PullRequestShort
	for rows.Next() {
		var id, name, authorID, statusStr string
		if err := rows.Scan(&id, &name, &authorID, &statusStr); err != nil {
			return nil, fmt.Errorf("scan pr: %w", err)
		}
		res = append(res, api.PullRequestShort{
			PullRequestId:   id,
			PullRequestName: name,
			AuthorId:        authorID,
			Status:          api.PullRequestShortStatus(statusStr),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return res, nil
}
