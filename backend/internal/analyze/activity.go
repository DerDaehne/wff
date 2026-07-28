package analyze

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Activity computes and persists TSS (and NP/IF where the power path
// applies) for one activity, writing into the existing
// activities.normalized_power_watts/intensity_factor/training_stress_score
// columns (from #547). Power is authoritative whenever it's present: the HR
// fallback only ever runs for a ride with zero power samples, never as a
// "better safe than sorry" double-check, and never to paper over a missing
// FTP (that stays NULL on purpose — see arch-wff-analyze) — a rider with a
// power meter but no FTP configured should fix that, not silently get a
// worse HR-based number instead.
func Activity(ctx context.Context, pool *pgxpool.Pool, activityID int64) error {
	var userID int64
	var elapsedSeconds int
	if err := pool.QueryRow(ctx,
		`SELECT user_id, elapsed_seconds FROM activities WHERE id = $1`, activityID,
	).Scan(&userID, &elapsedSeconds); err != nil {
		return err
	}

	var ftpWatts, lthrBpm *int
	if err := pool.QueryRow(ctx,
		`SELECT ftp_watts, lthr_bpm FROM users WHERE id = $1`, userID,
	).Scan(&ftpWatts, &lthrBpm); err != nil {
		return err
	}

	powerWatts, err := loadSampleFloats(ctx, pool, activityID, "power_watts")
	if err != nil {
		return err
	}

	if len(powerWatts) > 0 {
		if ftpWatts == nil {
			return nil
		}
		metrics := ComputePowerMetrics(powerWatts, elapsedSeconds, *ftpWatts)
		if metrics == nil {
			return nil
		}
		_, err := pool.Exec(ctx, `
			UPDATE activities SET
				normalized_power_watts = $2,
				intensity_factor = $3,
				training_stress_score = $4
			WHERE id = $1`,
			activityID, metrics.NormalizedPowerWatts, metrics.IntensityFactor, metrics.TSS,
		)
		return err
	}

	if lthrBpm == nil {
		return nil
	}
	trace, err := loadHeartRateTrace(ctx, pool, activityID)
	if err != nil {
		return err
	}
	if len(trace) == 0 {
		return nil
	}

	hrMetrics := ComputeHRMetrics(trace, *lthrBpm)
	if hrMetrics == nil {
		return nil
	}
	_, err = pool.Exec(ctx, `
		UPDATE activities SET
			intensity_factor = $2,
			training_stress_score = $3
		WHERE id = $1`,
		activityID, hrMetrics.IntensityFactor, hrMetrics.TSS,
	)
	return err
}

// loadHeartRateTrace reads the pulse readings with the stretch of time each one
// stands for — the gap to the previous sample, with stops dropped. The first
// reading of a ride covers nothing, because n samples describe n-1 intervals.
func loadHeartRateTrace(ctx context.Context, pool *pgxpool.Pool, activityID int64) ([]HRPoint, error) {
	rows, err := pool.Query(ctx, `
		SELECT heart_rate, extract(epoch FROM time - lag(time) OVER (ORDER BY time))
		FROM samples
		WHERE activity_id = $1 AND heart_rate IS NOT NULL
		ORDER BY time`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trace []HRPoint
	for rows.Next() {
		var bpm int
		var gap *float64
		if err := rows.Scan(&bpm, &gap); err != nil {
			return nil, err
		}
		if gap == nil || *gap <= 0 || *gap > MaxSampleGapSeconds {
			continue
		}
		trace = append(trace, HRPoint{Bpm: float64(bpm), Seconds: *gap})
	}
	return trace, rows.Err()
}

// RecomputeHeartRateLoad rewrites the training load of every ride that was
// scored from heart rate. It exists because the formula behind those numbers
// changed (#622): leaving old rides on the old one would put an invisible step
// in the fitness curve at the day of the upgrade, which reads as a training
// event that never happened. Power-based rides are left alone — their formula
// is unchanged, and recomputing them would be work for an identical result.
//
// ponytail: runs on every start and rescans the samples of every HR ride. One
// aggregate query per ride, a few hundred rides for a rider with years of
// history. Add a marker column if boot time ever becomes noticeable.
func RecomputeHeartRateLoad(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT id FROM activities
		WHERE normalized_power_watts IS NULL AND avg_heart_rate IS NOT NULL`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if err := Activity(ctx, pool, id); err != nil {
			return fmt.Errorf("activity %d: %w", id, err)
		}
	}
	return nil
}

// loadSampleFloats reads a single non-null numeric column from samples,
// ordered by time. column is always a fixed internal string (power_watts or
// heart_rate), never user input.
func loadSampleFloats(ctx context.Context, pool *pgxpool.Pool, activityID int64, column string) ([]float64, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+column+` FROM samples WHERE activity_id = $1 AND `+column+` IS NOT NULL ORDER BY time`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []float64
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, float64(v))
	}
	return values, rows.Err()
}
