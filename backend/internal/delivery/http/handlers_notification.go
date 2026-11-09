package http

import (
	"net/http"

	"receh/internal/domain"
	"receh/internal/usecase"
)

func (s *Server) inbox(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	in, err := s.svc.Notifications.Inbox(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// unreadCount is polled by the bell icon, so it stays a single cheap count.
func (s *Server) unreadCount(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	n, err := s.svc.Notifications.UnreadCount(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unread": n})
}

// markRead is allowed for frozen accounts too: reading is not changing data.
func (s *Server) markRead(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.svc.Notifications.MarkRead(r.Context(), u.ID, in.IDs, in.All); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) notificationPrefs(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	p, err := s.svc.Notifications.Prefs(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) saveNotificationPrefs(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.PrefsView
	if !readJSON(w, r, &in) {
		return
	}
	p, err := s.svc.Notifications.SavePrefs(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
