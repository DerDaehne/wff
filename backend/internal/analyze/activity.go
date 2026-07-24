package analyze

import (
	"context"

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
	heartRates, err := loadSampleFloats(ctx, pool, activityID, "heart_rate")
	if err != nil {
		return err
	}
	if len(heartRates) == 0 {
		return nil
	}
	var sum float64
	for _, hr := range heartRates {
		sum += hr
	}
	avgHeartRate := sum / float64(len(heartRates))

	hrMetrics := ComputeHRMetrics(avgHeartRate, elapsedSeconds, *lthrBpm)
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
