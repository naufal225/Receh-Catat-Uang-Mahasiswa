package http

import (
	"net/http"

	"receh/internal/domain"
)

// Authed is a handler that already knows who is calling.
type Authed = func(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session)

// Guards wrap a module's handlers with the core's authentication rules:
// Read needs a user account, Write also refuses frozen accounts, Admin needs
// an admin, Anyone needs any session.
type Guards struct {
	Anyone, Read, Write, Admin func(Authed) http.HandlerFunc
}

// Mounter registers a module's routes. Paths must start with /api/.
type Mounter func(mux *http.ServeMux, g Guards)

// Mount adds a module's routes; call it before Handler.
func (s *Server) Mount(m Mounter) { s.mounts = append(s.mounts, m) }

func (s *Server) guards() Guards {
	return Guards{Anyone: s.anyone, Read: s.userRead, Write: s.userWrite, Admin: s.admin}
}

// Exported response helpers so modules answer exactly like the core does.

func WriteJSON(w http.ResponseWriter, status int, v any) { writeJSON(w, status, v) }
func WriteError(w http.ResponseWriter, err error)        { writeError(w, err) }
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return readJSON(w, r, dst)
}
func ReadJSONLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	return readJSONLimit(w, r, dst, limit)
}
func PathID(w http.ResponseWriter, r *http.Request) (int64, bool) { return pathID(w, r) }
func QueryInt(r *http.Request, key string) int                    { return queryInt(r, key) }
func QueryInt64(r *http.Request, key string) int64                { return queryInt64(r, key) }
