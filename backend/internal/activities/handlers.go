// Package activities implements the .fit upload HTTP endpoint, wiring
// fitparse (decode) and ingest (persist) together behind the auth middleware.
package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// The one endpoint that also accepts a device token, so an iOS Shortcut can
	// post a ride without a browser session (#617).
	mux.Handle("POST /api/activities", auth.RequireUploadAuth(h.pool)(http.HandlerFunc(h.upload)))
	mux.Handle("GET /api/activities", auth.RequireAuth(h.pool)(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/activities/{id}/samples", auth.RequireAuth(h.pool)(http.HandlerFunc(h.samples)))
	mux.Handle("GET /api/activities/{id}/weather", auth.RequireAuth(h.pool)(http.HandlerFunc(h.weatherSummary)))
	mux.Handle("GET /api/activities/{id}/story", auth.RequireAuth(h.pool)(http.HandlerFunc(h.story)))
	mux.Handle("GET /api/activities/{id}/export", auth.RequireAuth(h.pool)(http.HandlerFunc(h.export)))
	mux.Handle("GET /api/me/export", auth.RequireAuth(h.pool)(http.HandlerFunc(h.meExport)))
	// Android's share sheet posts here (#617). Deliberately not under /api:
	// this is a navigation the browser performs, and it answers with a
	// redirect into the app rather than with JSON.
	mux.Handle("POST /share-target", auth.RequireAuth(h.pool)(http.HandlerFunc(h.shareTarget)))
}

// story returns the ride explained in plain language (#601): what kind of
// session it was, what it did to the rider, and why it felt that way. The
// wording and the thresholds behind it live in analyze.RideStory — this
// handler only gathers the facts.
func (h *Handlers) story(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	activityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid activity id", http.StatusBadRequest)
		return
	}

	var (
		facts                     analyze.RideFacts
		normalizedPower, avgPower *float64
		lthrBpm                   *int
		ownerID                   int64
	)
	err = h.pool.QueryRow(r.Context(), `
		SELECT a.user_id, a.started_at, a.elapsed_seconds, a.moving_seconds, a.distance_meters,
		       a.elevation_gain_meters, a.intensity_factor, a.training_stress_score, a.normalized_power_watts,
		       a.avg_power_watts, u.weight_kg, coalesce(u.primary_metric, ''), u.lthr_bpm,
		       a.avg_heart_rate, u.birth_year, u.sex
		FROM activities a JOIN users u ON u.id = a.user_id
		WHERE a.id = $1`,
		activityID,
	).Scan(&ownerID, &facts.StartedAt, &facts.ElapsedSeconds, &facts.MovingSeconds, &facts.DistanceMeters,
		&facts.ElevationGainMeters, &facts.IntensityFactor, &facts.TSS, &normalizedPower,
		&avgPower, &facts.WeightKg, &facts.PrimaryMetric, &lthrBpm,
		&facts.AvgHeartRate, &facts.BirthYear, &facts.Sex)
	if err != nil || ownerID != userID {
		// Same 404 for "not yours" and "doesn't exist" — see samples().
		http.Error(w, "activity not found", http.StatusNotFound)
		return
	}
	facts.FromPower = normalizedPower != nil

	// Weather is best-effort context: a ride that was never enriched (or has
	// no GPS) simply gets no wind/temperature statement.
	_ = h.pool.QueryRow(r.Context(), `
		SELECT avg(headwind_mps), avg(temperature_celsius)
		FROM enrichment WHERE activity_id = $1`,
		activityID,
	).Scan(&facts.HeadwindMps, &facts.TemperatureCelsius)

	// Earlier rides only: including this ride (or later ones) in its own
	// baseline would flatten exactly the difference the comparison shows.
	// Average speed comes along as the baseline for riders with neither power
	// nor heart rate, where TSS is always NULL (#606).
	// ponytail: last 30 rides is plenty for a median; widen if the baseline
	// ever needs to be season-aware.
	rows, err := h.pool.Query(r.Context(), `
		SELECT training_stress_score,
		       CASE WHEN moving_seconds > 0 AND distance_meters IS NOT NULL
		            THEN distance_meters / moving_seconds * 3.6 END
		FROM activities
		WHERE user_id = $1 AND started_at < $2
		ORDER BY started_at DESC LIMIT 30`,
		userID, facts.StartedAt,
	)
	if err != nil {
		http.Error(w, "could not load ride history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var tss, speedKmh *float64
		if err := rows.Scan(&tss, &speedKmh); err != nil {
			http.Error(w, "could not load ride history", http.StatusInternalServerError)
			return
		}
		if tss != nil {
			facts.PriorTSS = append(facts.PriorTSS, *tss)
		}
		if speedKmh != nil {
			facts.PriorSpeedsKmh = append(facts.PriorSpeedsKmh, *speedKmh)
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load ride history", http.StatusInternalServerError)
		return
	}

	course, err := h.courseStats(r.Context(), activityID)
	if err != nil {
		http.Error(w, "could not load ride course", http.StatusInternalServerError)
		return
	}
	facts.Course = course

	// Personal records, own history only (#636).
	facts.Milestones, err = analyze.PriorBests(r.Context(), h.pool, userID, facts.StartedAt)
	if err != nil {
		http.Error(w, "could not load ride history", http.StatusInternalServerError)
		return
	}

	// Endurance quality needs heart rate plus either power or speed, and only
	// says something on a steady aerobic ride — analyze decides that, this
	// just supplies the samples and the two steadiness inputs.
	endurance, err := h.enduranceOf(r.Context(), activityID, facts.IntensityFactor, avgPower, normalizedPower)
	if err != nil {
		http.Error(w, "could not evaluate endurance", http.StatusInternalServerError)
		return
	}
	facts.Endurance = endurance

	// Where the pulse actually sat during the ride (#621) — the average alone
	// hides whether this was steady or four hard efforts with rests. Without
	// a real threshold, the rider's own observed maximum stands in when it
	// looks like a real effort (#624).
	observedMax, err := analyze.ObservedMax(r.Context(), h.pool, ownerID)
	if err != nil {
		http.Error(w, "could not load observed maximum heart rate", http.StatusInternalServerError)
		return
	}
	effectiveLthr, assumedLthr := analyze.EffectiveLTHR(lthrBpm, observedMax, facts.BirthYear)
	zones, err := analyze.ActivityZones(r.Context(), h.pool, activityID, effectiveLthr, assumedLthr)
	if err != nil {
		http.Error(w, "could not compute heart-rate zones", http.StatusInternalServerError)
		return
	}
	facts.Zones = zones

	writeActivitiesJSON(w, analyze.RideStory(facts))
}

// courseStats loads the raw track plus the stored hourly wind vectors and
// derives the route statistics that need neither power nor heart rate. Returns
// nil when the ride has no usable GPS — then there simply is no course to talk
// about, which is not an error.
func (h *Handlers) courseStats(ctx context.Context, activityID int64) (*analyze.CourseStats, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT time, lat, lon, altitude_meters, speed_mps
		FROM samples WHERE activity_id = $1 ORDER BY time`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []analyze.CourseSample
	for rows.Next() {
		var s analyze.CourseSample
		if err := rows.Scan(&s.Time, &s.Lat, &s.Lon, &s.AltitudeMeters, &s.SpeedMps); err != nil {
			return nil, err
		}
		samples = append(samples, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(samples) < 2 {
		return nil, nil
	}

	windRows, err := h.pool.Query(ctx, `
		SELECT hour_bucket, wind_speed_mps, wind_direction_deg
		FROM enrichment WHERE activity_id = $1`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer windRows.Close()

	var winds []analyze.WindBucket
	for windRows.Next() {
		var b analyze.WindBucket
		if err := windRows.Scan(&b.Hour, &b.SpeedMps, &b.DirectionDeg); err != nil {
			return nil, err
		}
		winds = append(winds, b)
	}
	if err := windRows.Err(); err != nil {
		return nil, err
	}

	stats := analyze.Course(samples, winds)
	if stats.DistanceMeters <= 0 {
		return nil, nil
	}
	return &stats, nil
}

type activitySummary struct {
	ID                  int64     `json:"id"`
	StartedAt           time.Time `json:"started_at"`
	Sport               string    `json:"sport"`
	ElapsedSeconds      int       `json:"elapsed_seconds"`
	MovingSeconds       int       `json:"moving_seconds"`
	DistanceMeters      *float64  `json:"distance_meters"`
	TrainingStressScore *float64  `json:"training_stress_score"`
	// Zones is the ride's character at a glance in the list (#633) — nil
	// without enough recorded pulse, same rule as the ride-detail zones.
	Zones *analyze.ZoneShares `json:"zones,omitempty"`
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
	var ids []int64
	for rows.Next() {
		var a activitySummary
		if err := rows.Scan(&a.ID, &a.StartedAt, &a.Sport, &a.ElapsedSeconds, &a.MovingSeconds, &a.DistanceMeters, &a.TrainingStressScore); err != nil {
			http.Error(w, "could not load activities", http.StatusInternalServerError)
			return
		}
		activities = append(activities, a)
		ids = append(ids, a.ID)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}

	// Fahrt-Charakter auf einen Blick (#633): derselbe Schwellenpuls-Pfad wie
	// Ride-Detail und Dashboard (#624), sonst widersprechen sich die Ansichten.
	var lthrBpm, birthYear *int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT lthr_bpm, birth_year FROM users WHERE id = $1`, userID,
	).Scan(&lthrBpm, &birthYear); err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}
	observedMax, err := analyze.ObservedMax(r.Context(), h.pool, userID)
	if err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}
	effectiveLthr, assumedLthr := analyze.EffectiveLTHR(lthrBpm, observedMax, birthYear)
	zonesByActivity, err := analyze.ActivityZoneShares(r.Context(), h.pool, ids, effectiveLthr, assumedLthr)
	if err != nil {
		http.Error(w, "could not load activities", http.StatusInternalServerError)
		return
	}
	for i := range activities {
		if z, ok := zonesByActivity[activities[i].ID]; ok {
			activities[i].Zones = &z
		}
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

	activityID, err := h.ingestRide(r.Context(), userID, raw)
	switch {
	case errors.Is(err, ingest.ErrDuplicateActivity):
		http.Error(w, "activity already exists", http.StatusConflict)
		return
	case errors.Is(err, errInvalidFit):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		log.Printf("upload: user %d: %v", userID, err)
		http.Error(w, "could not store activity", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"activity_id":` + strconv.FormatInt(activityID, 10) + `}`))
}

// errInvalidFit separates "this file isn't a ride" from "we failed to store
// it". The two need different HTTP answers, and the share target needs to tell
// them apart to word what the rider ends up seeing.
var errInvalidFit = errors.New("invalid .fit file")

// ingestRide is the upload pipeline without the HTTP shell: parse, keep the
// raw file, insert, analyse, kick off enrichment. Shared by the JSON upload
// endpoint and the Android share target (#617), which differ only in how they
// answer.
func (h *Handlers) ingestRide(ctx context.Context, userID int64, raw []byte) (int64, error) {
	act, err := fitparse.Parse(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", errInvalidFit, err)
	}

	externalUID := ingest.ExternalUID(act.FileID, raw)

	rawPath, err := h.saveRawFile(userID, externalUID, raw)
	if err != nil {
		return 0, err
	}

	activityID, err := ingest.Store(ctx, h.pool, userID, act, externalUID)
	if err != nil {
		if errors.Is(err, ingest.ErrDuplicateActivity) {
			// rawPath is deterministic from externalUID, so on a duplicate it's
			// either the pre-existing file untouched or overwritten with
			// identical bytes — either way it still correctly belongs to the
			// original activity row. Do not remove it.
			return 0, err
		}
		os.Remove(rawPath) // genuine failure: no activity row references this file
		return 0, err
	}

	if _, err := h.pool.Exec(ctx,
		`UPDATE activities SET raw_file_path = $1 WHERE id = $2`, rawPath, activityID,
	); err != nil {
		return 0, err
	}

	// NP/IF/TSS: pure CPU work over samples already on disk, no external
	// API call like enrichment — safe to compute synchronously. A failure
	// here just leaves those columns NULL; it doesn't invalidate the upload
	// itself (the activity + samples are already correctly stored).
	if err := analyze.Activity(ctx, h.pool, activityID); err != nil {
		log.Printf("analyze: activity %d: %v", activityID, err)
	}

	// Best-effort immediate attempt: usually a no-op wait (ERA5 has ~5 day
	// lag) or a no-op for GPS-less rides, but occasionally data is already
	// there. Uses its own background context — the HTTP response has
	// already gone out by the time this runs. The retry poller (started in
	// main) is the durable path; this is purely a latency optimization.
	go func() {
		if _, err := enrich.Activity(context.Background(), h.pool, h.weather, activityID); err != nil {
			log.Printf("enrich: immediate attempt for activity %d: %v", activityID, err)
		}
	}()

	return activityID, nil
}

// shareTarget receives a .fit shared from another app on Android and answers
// with a redirect into the ride it just created.
//
// It redirects rather than returning JSON because the browser is performing a
// navigation: whatever comes back is what the person ends up looking at. A
// failure therefore has to land somewhere sensible too, which is why the error
// cases redirect to the upload page with a reason rather than rendering a bare
// status code at someone who just shared a file.
func (h *Handlers) shareTarget(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		redirectAfterShare(w, r, "zu-gross")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		redirectAfterShare(w, r, "keine-datei")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		redirectAfterShare(w, r, "nicht-lesbar")
		return
	}

	activityID, err := h.ingestRide(r.Context(), userID, raw)
	switch {
	case errors.Is(err, ingest.ErrDuplicateActivity):
		// Sharing the same ride twice is a normal slip, not a failure worth an
		// error page.
		redirectAfterShare(w, r, "schon-vorhanden")
		return
	case errors.Is(err, errInvalidFit):
		redirectAfterShare(w, r, "keine-fit-datei")
		return
	case err != nil:
		log.Printf("share-target: user %d: %v", userID, err)
		redirectAfterShare(w, r, "fehlgeschlagen")
		return
	}

	// 303 so that reloading the destination doesn't repost the file.
	http.Redirect(w, r, "/rides/"+strconv.FormatInt(activityID, 10), http.StatusSeeOther)
}

func redirectAfterShare(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/upload?geteilt="+reason, http.StatusSeeOther)
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

// enduranceOf loads what the efficiency calculation needs and hands the
// steadiness judgement to analyze. Variability (NP/average power) is the
// standard way to tell a steady ride from an interval session; without power
// there is no such measure, so a neutral 1.0 is passed and the intensity
// check plus the duration floor carry the decision.
func (h *Handlers) enduranceOf(ctx context.Context, activityID int64, intensityFactor, avgPower, normalizedPower *float64) (*analyze.Efficiency, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT time, power_watts, speed_mps, heart_rate
		FROM samples WHERE activity_id = $1 ORDER BY time`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		samples []analyze.EffortSample
		prev    time.Time
	)
	for rows.Next() {
		var (
			t         time.Time
			power, hr *int
			speed     *float64
		)
		if err := rows.Scan(&t, &power, &speed, &hr); err != nil {
			return nil, err
		}
		seconds := 0.0
		if !prev.IsZero() {
			// A stop must not weigh into either half of the ride.
			if gap := t.Sub(prev).Seconds(); gap > 0 && gap <= analyze.MaxSampleGapSeconds {
				seconds = gap
			}
		}
		prev = t
		if seconds == 0 {
			continue
		}
		samples = append(samples, analyze.EffortSample{
			Seconds:      seconds,
			PowerWatts:   intPtrToFloatPtr(power),
			SpeedMps:     speed,
			HeartRateBpm: intPtrToFloatPtr(hr),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	variability := 1.0
	if avgPower != nil && normalizedPower != nil && *avgPower > 0 {
		variability = *normalizedPower / *avgPower
	}

	efficiency, ok := analyze.EfficiencyOf(samples, intensityFactor, variability)
	if !ok {
		return nil, nil
	}
	return &efficiency, nil
}

func intPtrToFloatPtr(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}
