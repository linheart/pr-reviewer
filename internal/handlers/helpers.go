package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"pr-reviewer/internal/api"
	"pr-reviewer/internal/service"
)

const (
	errorCodeBadRequest api.ErrorResponseErrorCode = "BAD_REQUEST"
	errorCodeInternal   api.ErrorResponseErrorCode = "INTERNAL_ERROR"
)

type errorBody struct {
	Error struct {
		Code    api.ErrorResponseErrorCode `json:"code"`
		Message string                     `json:"message"`
	} `json:"error"`
}

func decodeJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain only one JSON object")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if payload == nil {
		return
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, code api.ErrorResponseErrorCode, msg string) {
	resp := errorBody{}
	resp.Error.Code = code
	resp.Error.Message = msg
	writeJSON(w, status, resp)
}

func writeServiceError(w http.ResponseWriter, err error) {
	var svcErr *service.Error
	if errors.As(err, &svcErr) {
		writeAPIError(w, statusFromCode(svcErr.Code), svcErr.Code, svcErr.Msg)
		return
	}
	writeAPIError(w, http.StatusInternalServerError, errorCodeInternal, "internal error")
}

func statusFromCode(code api.ErrorResponseErrorCode) int {
	switch code {
	case api.TEAMEXISTS:
		return http.StatusBadRequest
	case api.NOTFOUND:
		return http.StatusNotFound
	case api.PREXISTS,
		api.PRMERGED,
		api.NOTASSIGNED,
		api.NOCANDIDATE:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func badRequest(w http.ResponseWriter, err error) {
	msg := "invalid request"
	if err != nil {
		msg = err.Error()
	}
	writeAPIError(w, http.StatusBadRequest, errorCodeBadRequest, msg)
}
