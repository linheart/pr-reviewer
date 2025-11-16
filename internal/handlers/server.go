package handlers

import (
	"net/http"

	"pr-reviewer/internal/api"
	"pr-reviewer/internal/service"
)

type Server struct {
	svc *service.Service
}

func NewServer(svc *service.Service) *Server {
	return &Server{svc: svc}
}

func (s *Server) PostTeamAdd(w http.ResponseWriter, r *http.Request) {
	var body api.PostTeamAddJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		badRequest(w, err)
		return
	}

	team, err := s.svc.CreateTeam(r.Context(), api.Team(body))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"team": team,
	})
}

func (s *Server) GetTeamGet(w http.ResponseWriter, r *http.Request, params api.GetTeamGetParams) {
	team, err := s.svc.GetTeam(r.Context(), params.TeamName)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (s *Server) PostUsersSetIsActive(w http.ResponseWriter, r *http.Request) {
	var body api.PostUsersSetIsActiveJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		badRequest(w, err)
		return
	}

	user, err := s.svc.SetUserActive(r.Context(), body.UserId, body.IsActive)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) PostPullRequestCreate(w http.ResponseWriter, r *http.Request) {
	var body api.PostPullRequestCreateJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		badRequest(w, err)
		return
	}

	pr, err := s.svc.CreatePullRequest(r.Context(), body.PullRequestId, body.PullRequestName, body.AuthorId)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"pr": pr,
	})
}

func (s *Server) PostPullRequestMerge(w http.ResponseWriter, r *http.Request) {
	var body api.PostPullRequestMergeJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		badRequest(w, err)
		return
	}

	pr, err := s.svc.MergePullRequest(r.Context(), body.PullRequestId)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr": pr,
	})
}

func (s *Server) PostPullRequestReassign(w http.ResponseWriter, r *http.Request) {
	var body api.PostPullRequestReassignJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		badRequest(w, err)
		return
	}

	pr, replacedBy, err := s.svc.ReassignReviewer(r.Context(), body.PullRequestId, body.OldUserId)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr":          pr,
		"replaced_by": replacedBy,
	})
}

func (s *Server) GetUsersGetReview(w http.ResponseWriter, r *http.Request, params api.GetUsersGetReviewParams) {
	prs, err := s.svc.ListUserReviewPRs(r.Context(), params.UserId)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":       params.UserId,
		"pull_requests": prs,
	})
}
