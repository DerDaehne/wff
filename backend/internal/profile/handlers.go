// Package profile exposes per-user training settings (FTP, LTHR) that the
// analysis pipeline (internal/analyze) reads to compute IF/TSS. No FTP
// history/versioning: the current value applies to future calculations
// only, past activities keep whatever they were computed with (see
// arch-wff-analyze).
package profile

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/DerDaehne/wff/internal/analyze"
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
	// WeightKg turns climbing speed into a power estimate (#610); optional,
	// because everything else works without it.
	WeightKg *float64 `json:"weight_kg"`
}

// settingsResponse carries the stored values plus what the rider's own rides
// suggest they should be (#609). Suggestions are never applied automatically —
// a value the rider entered is theirs, and an estimate silently overwriting it
// would be worse than no estimate at all.
type settingsResponse struct {
	settings
	Estimates analyze.Estimates `json:"estimates"`
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	var s settings
	if err := h.pool.QueryRow(r.Context(),
		`SELECT ftp_watts, lthr_bpm, weight_kg FROM users WHERE id = $1`, userID,
	).Scan(&s.FTPWatts, &s.LTHRBpm, &s.WeightKg); err != nil {
		http.Error(w, "could not load settings", http.StatusInternalServerError)
		return
	}

	// Best effort: a rider with no rides yet still gets their settings page.
	estimates, err := analyze.EstimateThresholds(r.Context(), h.pool, userID)
	if err != nil {
		log.Printf("profile: could not estimate thresholds for user %d: %v", userID, err)
	}

	writeJSON(w, settingsResponse{settings: s, Estimates: estimates})
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
	// A plausibility range, not a validation of the person: it exists to catch
	// grams entered as kilograms, which would make every W/kg figure absurd.
	if body.WeightKg != nil && (*body.WeightKg < 20 || *body.WeightKg > 300) {
		http.Error(w, "weight_kg must be between 20 and 300", http.StatusBadRequest)
		return
	}

	if _, err := h.pool.Exec(r.Context(), `
		UPDATE users SET
			ftp_watts = COALESCE($2, ftp_watts),
			lthr_bpm = COALESCE($3, lthr_bpm),
			weight_kg = COALESCE($4, weight_kg)
		WHERE id = $1`,
		userID, body.FTPWatts, body.LTHRBpm, body.WeightKg,
	); err != nil {
		http.Error(w, "could not update settings", http.StatusInternalServerError)
		return
	}

	// Setting FTP/LTHR for the first time shouldn't strand every activity
	// uploaded before that moment without a TSS forever — recompute the
	// ones that don't have one yet. Best-effort: a failure here doesn't fail
	// the settings update itself, just logs (the next poller/upload-triggered
	// analyze call would eventually catch new activities anyway; this is
	// specifically for the backlog of already-uploaded ones).
	if body.FTPWatts != nil || body.LTHRBpm != nil {
		h.recomputeMissingTSS(r.Context(), userID)
	}

	h.get(w, r)
}

func (h *Handlers) recomputeMissingTSS(ctx context.Context, userID int64) {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM activities WHERE user_id = $1 AND training_stress_score IS NULL`, userID)
	if err != nil {
		log.Printf("profile: recomputeMissingTSS: user_id=%d query failed: %v", userID, err)
		return
	}
	var activityIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.Printf("profile: recomputeMissingTSS: user_id=%d scan failed: %v", userID, err)
			return
		}
		activityIDs = append(activityIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Printf("profile: recomputeMissingTSS: user_id=%d rows error: %v", userID, err)
		return
	}

	for _, id := range activityIDs {
		if err := analyze.Activity(ctx, h.pool, id); err != nil {
			log.Printf("profile: recomputeMissingTSS: user_id=%d activity_id=%d failed: %v", userID, id, err)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
