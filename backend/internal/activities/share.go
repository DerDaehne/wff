package activities

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/DerDaehne/wff/internal/auth"
	"github.com/jackc/pgx/v5"
)

// shareStatus is what the owner's UI needs to render the "shared" badge and
// the copy/revoke controls (#641). The full share URL is built client-side
// from window.location.origin — no new server config for a public base URL.
type shareStatus struct {
	Active    bool       `json:"active"`
	Token     *string    `json:"token,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// publicRideSummary is everything a share link reveals to someone without a
// login — stats only, deliberately. No lat/lon, no samples, no calorie text
// (that references the owner's weight/age): the only place in the app where
// data reaches a person who never authenticated, so the shape is a short,
// explicit allow-list rather than a trimmed-down copy of the owner's view.
type publicRideSummary struct {
	StartedAt           time.Time `json:"started_at"`
	Sport               string    `json:"sport"`
	MovingSeconds       int       `json:"moving_seconds"`
	DistanceMeters      *float64  `json:"distance_meters"`
	ElevationGainMeters *float64  `json:"elevation_gain_meters"`
	TrainingStressScore *float64  `json:"training_stress_score"`
}

func (h *Handlers) ownsActivity(ctx context.Context, userID, activityID int64) bool {
	var owner int64
	err := h.pool.QueryRow(ctx, `SELECT user_id FROM activities WHERE id = $1`, activityID).Scan(&owner)
	return err == nil && owner == userID
}

// shareStatusFor is the activity's current, non-revoked share, or an
// inactive status if it has never been shared (or was revoked).
func (h *Handlers) shareStatusFor(ctx context.Context, activityID int64) (shareStatus, error) {
	var token string
	var createdAt time.Time
	err := h.pool.QueryRow(ctx, `
		SELECT token, created_at FROM ride_shares
		WHERE activity_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC LIMIT 1`,
		activityID,
	).Scan(&token, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return shareStatus{Active: false}, nil
		}
		return shareStatus{}, err
	}
	return shareStatus{Active: true, Token: &token, CreatedAt: &createdAt}, nil
}

func (h *Handlers) getShare(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	activityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !h.ownsActivity(r.Context(), userID, activityID) {
		http.Error(w, "activity not found", http.StatusNotFound)
		return
	}

	status, err := h.shareStatusFor(r.Context(), activityID)
	if err != nil {
		http.Error(w, "could not load share status", http.StatusInternalServerError)
		return
	}
	writeActivitiesJSON(w, status)
}

// createShare is idempotent — a double click (or an already-shared ride)
// returns the existing active token instead of creating a second one, so a
// ride never quietly accumulates multiple valid links.
func (h *Handlers) createShare(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	activityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !h.ownsActivity(r.Context(), userID, activityID) {
		http.Error(w, "activity not found", http.StatusNotFound)
		return
	}

	existing, err := h.shareStatusFor(r.Context(), activityID)
	if err != nil {
		http.Error(w, "could not create share", http.StatusInternalServerError)
		return
	}
	if existing.Active {
		writeActivitiesJSON(w, existing)
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		http.Error(w, "could not create share", http.StatusInternalServerError)
		return
	}
	// Stored as-is, not hashed like a session token: this token only grants
	// read access to one ride's stats, not the account, so the risk it
	// guards against doesn't justify a hash-and-compare lookup.
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO ride_shares (activity_id, token) VALUES ($1, $2)`, activityID, token,
	); err != nil {
		http.Error(w, "could not create share", http.StatusInternalServerError)
		return
	}

	status, err := h.shareStatusFor(r.Context(), activityID)
	if err != nil {
		http.Error(w, "could not create share", http.StatusInternalServerError)
		return
	}
	writeActivitiesJSON(w, status)
}

func (h *Handlers) revokeShare(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	activityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !h.ownsActivity(r.Context(), userID, activityID) {
		http.Error(w, "activity not found", http.StatusNotFound)
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE ride_shares SET revoked_at = now() WHERE activity_id = $1 AND revoked_at IS NULL`,
		activityID,
	); err != nil {
		http.Error(w, "could not revoke share", http.StatusInternalServerError)
		return
	}
	writeActivitiesJSON(w, shareStatus{Active: false})
}

// publicShare is the one endpoint in the whole app that answers without
// RequireAuth — a revoked or never-created token gets the same 404 as a
// nonexistent one, so probing tokens can't distinguish "wrong" from
// "revoked".
func (h *Handlers) publicShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	var summary publicRideSummary
	var revokedAt *time.Time
	err := h.pool.QueryRow(r.Context(), `
		SELECT a.started_at, a.sport, a.moving_seconds, a.distance_meters,
		       a.elevation_gain_meters, a.training_stress_score, s.revoked_at
		FROM ride_shares s JOIN activities a ON a.id = s.activity_id
		WHERE s.token = $1`,
		token,
	).Scan(&summary.StartedAt, &summary.Sport, &summary.MovingSeconds, &summary.DistanceMeters,
		&summary.ElevationGainMeters, &summary.TrainingStressScore, &revokedAt)
	if err != nil || revokedAt != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeActivitiesJSON(w, summary)
}
