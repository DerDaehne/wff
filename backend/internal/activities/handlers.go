// Package activities implements the .fit upload HTTP endpoint, wiring
// fitparse (decode) and ingest (persist) together behind the auth middleware.
package activities

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/DerDaehne/wff/internal/analyze"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/enrich"
	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/DerDaehne/wff/internal/ingest"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxUploadBytes is generous for .fit: even multi-hour rides at 1 Hz are
// typically low single-digit MB.
const maxUploadBytes = 50 << 20

type Handlers struct {
	pool      *pgxpool.Pool
	uploadDir string
	weather   *openmeteo.Client
}

func NewHandlers(pool *pgxpool.Pool, uploadDir string, weather *openmeteo.Client) *Handlers {
	return &Handlers{pool: pool, uploadDir: uploadDir, weather: weather}
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/activities", auth.RequireAuth(h.pool)(http.HandlerFunc(h.upload)))
	mux.Handle("GET /api/activities", auth.RequireAuth(h.pool)(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/activities/{id}/samples", auth.RequireAuth(h.pool)(http.HandlerFunc(h.samples)))
	mux.Handle("GET /api/activities/{id}/weather", auth.RequireAuth(h.pool)(http.HandlerFunc(h.weatherSummary)))
}

type activitySummary struct {
	ID                  int64     `json:"id"`
	StartedAt           time.Time `json:"started_at"`
	Sport               string    `json:"sport"`
	ElapsedSeconds      int       `json:"elapsed_seconds"`
	MovingSeconds       int       `json:"moving_seconds"`
	DistanceMeters      *float64  `json:"distance_meters"`
	TrainingStressScore *float64  `json:"training_stress_score"`
}

// list returns the requesting person's activities, most recent first — the
// Ride-Liste view's data source (#572).
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, started_at, sport, elapsed_seconds, moving_seconds, distance_meters, training_stress_score
		 FROM activities WHERE user_id = $1 ORDER BY started_at DESC`,
		userID,
	)
	if err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	activities := []activitySummary{} // never null in the JSON response
	for rows.Next() {
		var a activitySummary
		if err := rows.Scan(&a.ID, &a.StartedAt, &a.Sport, &a.ElapsedSeconds, &a.MovingSeconds, &a.DistanceMeters, &a.TrainingStressScore); err != nil {
			http.Error(w, "could not load activities", http.StatusInternalServerError)
			return
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}

	writeActivitiesJSON(w, activities)
}

type sampleDTO struct {
	Time           time.Time `json:"time"`
	Lat            *float64  `json:"lat"`
	Lon            *float64  `json:"lon"`
	AltitudeMeters *float64  `json:"altitude_meters"`
	PowerWatts     *int      `json:"power_watts"`
	HeartRate      *int      `json:"heart_rate"`
}

// samples returns an activity's raw time series (GPS track, elevation,
// power, heart rate) for the Ride-Detail view (#572): map, elevation
// profile, power/HR curve. Ownership is checked explicitly — RequireAuth
// only proves who's asking, not that the activity is theirs.
func (h *Handlers) samples(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	activityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid activity id", http.StatusBadRequest)
		return
	}

	var ownerID int64
	err = h.pool.QueryRow(r.Context(), `SELECT user_id FROM activities WHERE id = $1`, activityID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		// Same response whether the activity doesn't exist or belongs to
		// someone else — don't leak which via the status code.
		http.Error(w, "activity not found", http.StatusNotFound)
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT time, lat, lon, altitude_meters, power_watts, heart_rate
		 FROM samples WHERE activity_id = $1 ORDER BY time`,
		activityID,
	)
	if err != nil {
		http.Error(w, "could not load samples", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	samples := []sampleDTO{} // never null in the JSON response
	for rows.Next() {
		var s sampleDTO
		if err := rows.Scan(&s.Time, &s.Lat, &s.Lon, &s.AltitudeMeters, &s.PowerWatts, &s.HeartRate); err != nil {
			http.Error(w, "could not load samples", http.StatusInternalServerError)
			return
		}
		samples = append(samples, s)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load samples", http.StatusInternalServerError)
		return
	}

	writeActivitiesJSON(w, samples)
}

type weatherSummaryDTO struct {
	AvgWindSpeedMps       *float64 `json:"avg_wind_speed_mps"`
	AvgHeadwindMps        *float64 `json:"avg_headwind_mps"`
	AvgTemperatureCelsius *float64 `json:"avg_temperature_celsius"`
	BucketsEnriched       int      `json:"buckets_enriched"`
}

// weatherSummary returns the activity's average wind/temperature context
// (see internal/enrich) for the Ride-Detail view. BucketsEnriched == 0 means
// not yet enriched (or no GPS data) — not an error, just nothing to show.
func (h *Handlers) weatherSummary(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	activityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid activity id", http.StatusBadRequest)
		return
	}

	var ownerID int64
	err = h.pool.QueryRow(r.Context(), `SELECT user_id FROM activities WHERE id = $1`, activityID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		http.Error(w, "activity not found", http.StatusNotFound)
		return
	}

	var summary weatherSummaryDTO
	if err := h.pool.QueryRow(r.Context(), `
		SELECT avg(wind_speed_mps), avg(headwind_mps), avg(temperature_celsius), count(*)
		FROM enrichment WHERE activity_id = $1`,
		activityID,
	).Scan(&summary.AvgWindSpeedMps, &summary.AvgHeadwindMps, &summary.AvgTemperatureCelsius, &summary.BucketsEnriched); err != nil {
		http.Error(w, "could not load weather summary", http.StatusInternalServerError)
		return
	}

	writeActivitiesJSON(w, summary)
}

func writeActivitiesJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (h *Handlers) upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "request too large or malformed", http.StatusRequestEntityTooLarge)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing \"file\" field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read upload", http.StatusBadRequest)
		return
	}

	act, err := fitparse.Parse(raw)
	if err != nil {
		http.Error(w, "invalid .fit file: "+err.Error(), http.StatusBadRequest)
		return
	}

	externalUID := ingest.ExternalUID(act.FileID, raw)

	rawPath, err := h.saveRawFile(userID, externalUID, raw)
	if err != nil {
		http.Error(w, "could not store upload", http.StatusInternalServerError)
		return
	}

	activityID, err := ingest.Store(r.Context(), h.pool, userID, act, externalUID)
	if err != nil {
		if errors.Is(err, ingest.ErrDuplicateActivity) {
			// rawPath is deterministic from externalUID, so on a duplicate it's
			// either the pre-existing file untouched or overwritten with
			// identical bytes — either way it still correctly belongs to the
			// original activity row. Do not remove it.
			http.Error(w, "activity already exists", http.StatusConflict)
			return
		}
		os.Remove(rawPath) // genuine failure: no activity row references this file
		http.Error(w, "could not store activity", http.StatusInternalServerError)
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE activities SET raw_file_path = $1 WHERE id = $2`, rawPath, activityID,
	); err != nil {
		http.Error(w, "could not store activity", http.StatusInternalServerError)
		return
	}

	// NP/IF/TSS: pure CPU work over samples already on disk, no external
	// API call like enrichment — safe to compute synchronously. A failure
	// here just leaves those columns NULL; it doesn't invalidate the upload
	// itself (the activity + samples are already correctly stored).
	if err := analyze.Activity(r.Context(), h.pool, activityID); err != nil {
		log.Printf("analyze: activity %d: %v", activityID, err)
	}

	// Best-effort immediate attempt: usually a no-op wait (ERA5 has ~5 day
	// lag) or a no-op for GPS-less rides, but occasionally data is already
	// there. Uses its own background context — the HTTP response below has
	// already gone out by the time this runs. The retry poller (started in
	// main) is the durable path; this is purely a latency optimization.
	go func() {
		if _, err := enrich.Activity(context.Background(), h.pool, h.weather, activityID); err != nil {
			log.Printf("enrich: immediate attempt for activity %d: %v", activityID, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"activity_id":` + strconv.FormatInt(activityID, 10) + `}`))
}

func (h *Handlers) saveRawFile(userID int64, externalUID string, raw []byte) (string, error) {
	dir := filepath.Join(h.uploadDir, strconv.FormatInt(userID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, externalUID+".fit")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
