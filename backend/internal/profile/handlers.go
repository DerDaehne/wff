// Package profile exposes per-user training settings (FTP, LTHR) that the
// analysis pipeline (internal/analyze) reads to compute IF/TSS. No FTP
// history/versioning: the current value applies to future calculations
// only, past activities keep whatever they were computed with (see
// arch-wff-analyze).
package profile

import (
	"encoding/json"
	"net/http"

	"github.com/DerDaehne/wff/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool}
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/me/settings", auth.RequireAuth(h.pool)(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/me/settings", auth.RequireAuth(h.pool)(http.HandlerFunc(h.update)))
}

type settings struct {
	FTPWatts *int `json:"ftp_watts"`
	LTHRBpm  *int `json:"lthr_bpm"`
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	var s settings
	if err := h.pool.QueryRow(r.Context(),
		`SELECT ftp_watts, lthr_bpm FROM users WHERE id = $1`, userID,
	).Scan(&s.FTPWatts, &s.LTHRBpm); err != nil {
		http.Error(w, "could not load settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, s)
}

// update is a partial PATCH: omitted fields keep their current value. There
// is currently no way to clear a field back to NULL once set — acceptable
// for this MVP (FTP/LTHR practically only ever get corrected, not removed).
func (h *Handlers) update(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	var body settings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.FTPWatts != nil && *body.FTPWatts <= 0 {
		http.Error(w, "ftp_watts must be positive", http.StatusBadRequest)
		return
	}
	if body.LTHRBpm != nil && *body.LTHRBpm <= 0 {
		http.Error(w, "lthr_bpm must be positive", http.StatusBadRequest)
		return
	}

	if _, err := h.pool.Exec(r.Context(), `
		UPDATE users SET
			ftp_watts = COALESCE($2, ftp_watts),
			lthr_bpm = COALESCE($3, lthr_bpm)
		WHERE id = $1`,
		userID, body.FTPWatts, body.LTHRBpm,
	); err != nil {
		http.Error(w, "could not update settings", http.StatusInternalServerError)
		return
	}
	h.get(w, r)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
