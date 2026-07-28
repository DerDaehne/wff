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
