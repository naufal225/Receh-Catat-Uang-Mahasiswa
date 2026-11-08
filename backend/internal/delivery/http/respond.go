package http

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"receh/internal/domain"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("write json: %v", err)
		}
	}
}

// writeError maps domain errors to statuses. Anything unexpected is logged and
// answered generically, so internals never reach the client.
func writeError(w http.ResponseWriter, err error) {
	var app *domain.AppError
	if !errors.As(err, &app) {
		log.Printf("internal error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Code: "INTERNAL", Message: "Terjadi kesalahan di server. Coba lagi sebentar."})
		return
	}
	status := http.StatusBadRequest
	switch app.Kind {
	case domain.KindUnauthorized:
		status = http.StatusUnauthorized
	case domain.KindForbidden:
		status = http.StatusForbidden
	case domain.KindNotFound:
		status = http.StatusNotFound
	case domain.KindConflict:
		status = http.StatusConflict
	case domain.KindTooMany:
		status = http.StatusTooManyRequests
	}
	writeJSON(w, status, errorBody{Code: app.Code, Message: app.Message, Field: app.Field})
}

// readJSON decodes a small JSON body into dst and answers the error itself.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		msg := "Data yang dikirim tidak bisa dibaca."
		if errors.Is(err, io.EOF) {
			msg = "Data kosong."
		}
		writeError(w, domain.Invalid("", msg))
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, domain.NotFound("Data tidak ditemukan."))
		return 0, false
	}
	return id, true
}

func queryInt(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	return v
}

func queryInt64(r *http.Request, key string) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return v
}
