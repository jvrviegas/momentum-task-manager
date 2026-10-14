package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// maxBody bounds a JSON request body.
const maxBody = 1 << 20

func (s *server) startDeviceSignIn(w http.ResponseWriter, r *http.Request) {
	start, err := s.auth.Start(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, start)
}

func (s *server) pollDeviceSignIn(w http.ResponseWriter, r *http.Request) {
	var request api.PollDeviceSignInRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&request); err != nil {
		writeError(w, &api.Error{Code: api.CodeInvalid, Message: "the request is not valid JSON"})
		return
	}
	if request.DeviceCode == "" {
		writeError(w, &api.Error{Code: api.CodeInvalid, Message: "deviceCode is required", Field: "deviceCode"})
		return
	}
	token, err := s.auth.Poll(r.Context(), request.DeviceCode)
	var apiErr *api.Error
	switch {
	case errors.As(err, &apiErr):
		writeError(w, apiErr)
	case err != nil:
		s.internalError(w, r, err)
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, token)
	}
}

// authenticated runs next for a request with a valid bearer token, acting
// for its user; others get CodeUnauthorized.
func (s *server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		if !strings.EqualFold(scheme, "Bearer") {
			token = ""
		}
		session, err := s.auth.Authenticate(r.Context(), strings.TrimSpace(token))
		switch {
		case errors.Is(err, auth.ErrUnauthorized):
			w.Header().Set("WWW-Authenticate", `Bearer realm="momentum"`)
			writeError(w, &api.Error{Code: api.CodeUnauthorized, Message: "sign in again"})
		case err != nil:
			s.internalError(w, r, err)
		default:
			ctx := auth.NewContext(viewer.NewContext(r.Context(), session.UserID), session)
			next(w, r.WithContext(ctx))
		}
	}
}

func (s *server) session(w http.ResponseWriter, r *http.Request) {
	session, _ := auth.FromContext(r.Context())
	writeJSON(w, http.StatusOK, api.Session{Email: session.Email, ExpiresAt: session.ExpiresAt})
}

func (s *server) signOut(w http.ResponseWriter, r *http.Request) {
	session, _ := auth.FromContext(r.Context())
	if err := s.auth.Revoke(r.Context(), session.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
