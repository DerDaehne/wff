package activities

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/DerDaehne/wff/internal/auth"
)

// export serves a rider's own uploaded file back to them — the same bytes
// that were parsed, not a regenerated GPX/TCX (#639). Higher fidelity than
// re-deriving one (every field the device recorded, not just what WFF's own
// model kept) and no new file-format code to write and maintain.
func (h *Handlers) export(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	activityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid activity id", http.StatusBadRequest)
		return
	}

	var ownerID int64
	var startedAt time.Time
	var rawPath *string
	err = h.pool.QueryRow(r.Context(),
		`SELECT user_id, started_at, raw_file_path FROM activities WHERE id = $1`, activityID,
	).Scan(&ownerID, &startedAt, &rawPath)
	if err != nil || ownerID != userID {
		// Same 404 for "not yours" and "doesn't exist" as the other
		// per-activity endpoints — see samples() in handlers.go.
		http.Error(w, "activity not found", http.StatusNotFound)
		return
	}
	if rawPath == nil {
		http.Error(w, "no raw file stored for this activity", http.StatusNotFound)
		return
	}

	filename := fmt.Sprintf("fahrt-%s.fit", startedAt.Format("2006-01-02"))
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	http.ServeFile(w, r, *rawPath)
}

type exportSettings struct {
	FTPWatts      *int     `json:"ftp_watts"`
	LTHRBpm       *int     `json:"lthr_bpm"`
	WeightKg      *float64 `json:"weight_kg"`
	PrimaryMetric *string  `json:"primary_metric"`
	BirthYear     *int     `json:"birth_year"`
	Sex           *string  `json:"sex"`
}

// exportActivity is every figure WFF stores for a ride, minus internal-only
// columns (user_id, raw_file_path) — the raw file travels alongside instead.
type exportActivity struct {
	ID                   int64     `json:"id"`
	ExternalUID          string    `json:"external_uid"`
	Sport                string    `json:"sport"`
	StartedAt            time.Time `json:"started_at"`
	ElapsedSeconds       int       `json:"elapsed_seconds"`
	MovingSeconds        int       `json:"moving_seconds"`
	DistanceMeters       *float64  `json:"distance_meters"`
	ElevationGainMeters  *float64  `json:"elevation_gain_meters"`
	AvgPowerWatts        *float64  `json:"avg_power_watts"`
	MaxPowerWatts        *float64  `json:"max_power_watts"`
	NormalizedPowerWatts *float64  `json:"normalized_power_watts"`
	AvgHeartRate         *float64  `json:"avg_heart_rate"`
	MaxHeartRate         *float64  `json:"max_heart_rate"`
	AvgCadence           *float64  `json:"avg_cadence"`
	MaxCadence           *float64  `json:"max_cadence"`
	IntensityFactor      *float64  `json:"intensity_factor"`
	TrainingStressScore  *float64  `json:"training_stress_score"`
	rawFilePath          *string
}

// meExport bundles a rider's own data into one ZIP: profile settings, every
// activity's stored figures, and each activity's original uploaded file
// (#639) — a DSGVO-style full export, not just a single ride. Samples
// (GPS/power/heart rate) are already inside those raw files at full
// fidelity, so they are deliberately not also dumped as a second, redundant
// JSON representation.
func (h *Handlers) meExport(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	var username string
	var settings exportSettings
	if err := h.pool.QueryRow(r.Context(), `
		SELECT username, ftp_watts, lthr_bpm, weight_kg, primary_metric, birth_year, sex
		FROM users WHERE id = $1`, userID,
	).Scan(&username, &settings.FTPWatts, &settings.LTHRBpm, &settings.WeightKg,
		&settings.PrimaryMetric, &settings.BirthYear, &settings.Sex,
	); err != nil {
		http.Error(w, "could not load profile", http.StatusInternalServerError)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, external_uid, sport, started_at, elapsed_seconds, moving_seconds,
		       distance_meters, elevation_gain_meters, avg_power_watts, max_power_watts,
		       normalized_power_watts, avg_heart_rate, max_heart_rate, avg_cadence, max_cadence,
		       intensity_factor, training_stress_score, raw_file_path
		FROM activities WHERE user_id = $1 ORDER BY started_at`,
		userID,
	)
	if err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var activities []exportActivity
	for rows.Next() {
		var a exportActivity
		if err := rows.Scan(&a.ID, &a.ExternalUID, &a.Sport, &a.StartedAt, &a.ElapsedSeconds, &a.MovingSeconds,
			&a.DistanceMeters, &a.ElevationGainMeters, &a.AvgPowerWatts, &a.MaxPowerWatts,
			&a.NormalizedPowerWatts, &a.AvgHeartRate, &a.MaxHeartRate, &a.AvgCadence, &a.MaxCadence,
			&a.IntensityFactor, &a.TrainingStressScore, &a.rawFilePath,
		); err != nil {
			http.Error(w, "could not load activities", http.StatusInternalServerError)
			return
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="wff-export-%s.zip"`, username))

	zw := zip.NewWriter(w)
	defer zw.Close()

	if f, err := zw.Create("profil.json"); err == nil {
		json.NewEncoder(f).Encode(settings)
	}
	if f, err := zw.Create("fahrten.json"); err == nil {
		json.NewEncoder(f).Encode(activities)
	}
	for _, a := range activities {
		if a.rawFilePath == nil {
			continue
		}
		raw, err := os.ReadFile(*a.rawFilePath)
		if err != nil {
			continue // best-effort: a file missing on disk must not sink the whole export
		}
		name := fmt.Sprintf("fahrten/%s-%d.fit", a.StartedAt.Format("2006-01-02"), a.ID)
		if f, err := zw.Create(name); err == nil {
			f.Write(raw)
		}
	}
}
