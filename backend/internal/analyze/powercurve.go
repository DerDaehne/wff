package analyze

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// powerCurveDurations are the standard windows a power curve is read at —
// 5 s (sprint), 30 s, 1/5/10/20 min (the window FTP estimates come from,
// #594), 60 min. Fixed and few on purpose: a curve at every second would be
// noise nobody reads, and these are the durations riders and coaches
// actually compare.
var powerCurveDurations = []int{5, 30, 60, 300, 600, 1200, 3600}

// ComputePowerCurve stores this ride's best average power for each standard
// duration (#592) — independent of whether FTP is configured, unlike NP/IF/
// TSS: a power curve is raw sustained output, and #594 means to derive an
// FTP estimate FROM it, so it cannot itself require one already being set.
//
// A ride shorter than a given window simply has no point for it — not an
// error, just one fewer row. A ride with no power data at all writes nothing.
func ComputePowerCurve(ctx context.Context, pool *pgxpool.Pool, activityID int64) error {
	powers, _, times, err := loadEffortSamples(ctx, pool, activityID)
	if err != nil {
		return err
	}

	hasPower := false
	for _, p := range powers {
		if p != nil {
			hasPower = true
			break
		}
	}
	if !hasPower {
		return nil
	}

	for _, duration := range powerCurveDurations {
		best, ok := bestWindowAverage(powers, times, time.Duration(duration)*time.Second)
		if !ok {
			continue
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO power_curve_points (activity_id, duration_seconds, watts)
			VALUES ($1, $2, $3)
			ON CONFLICT (activity_id, duration_seconds) DO UPDATE SET watts = EXCLUDED.watts`,
			activityID, duration, int(best+0.5),
		); err != nil {
			return err
		}
	}
	return nil
}

// DefaultPowerCurveDuration is the lens the trend view opens on — 20
// minutes, the same window the FTP estimate (#609) and #594 read.
const DefaultPowerCurveDuration = 1200

// ValidPowerCurveDuration reports whether seconds is one of the fixed
// windows ComputePowerCurve actually wrote — anything else can never have a
// row, so the handler falls back rather than running a query that always
// comes back empty.
func ValidPowerCurveDuration(seconds int) bool {
	for _, d := range powerCurveDurations {
		if d == seconds {
			return true
		}
	}
	return false
}

// PowerCurveHistory is one ride's power-curve value at a chosen duration —
// the trend view (#593) reads the same figure ComputePowerCurve stored,
// across rides instead of within one.
type PowerCurveHistory struct {
	ActivityID int64     `json:"activity_id"`
	StartedAt  time.Time `json:"started_at"`
	Watts      int       `json:"watts"`
}

// PowerCurveOverTime is a rider's history at one fixed duration, oldest
// first — "is my 20-minute power going up" answered by the ride-by-ride
// figures themselves (natural day-to-day variance included), not a
// monotonic best-ever ratchet that would hide a real regression behind an
// old high.
func PowerCurveOverTime(ctx context.Context, pool *pgxpool.Pool, userID int64, durationSeconds int) ([]PowerCurveHistory, error) {
	rows, err := pool.Query(ctx, `
		SELECT a.id, a.started_at, p.watts
		FROM power_curve_points p
		JOIN activities a ON a.id = p.activity_id
		WHERE a.user_id = $1 AND p.duration_seconds = $2
		ORDER BY a.started_at`,
		userID, durationSeconds,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PowerCurveHistory{}
	for rows.Next() {
		var h PowerCurveHistory
		if err := rows.Scan(&h.ActivityID, &h.StartedAt, &h.Watts); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
