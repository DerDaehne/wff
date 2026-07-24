package analyze

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Activity computes and persists NP/IF/TSS for one activity from its power
// samples and the owning user's FTP, writing into the existing
// activities.normalized_power_watts/intensity_factor/training_stress_score
// columns (from #547). A no-op — not an error — if there are no power
// samples (candidate for the HR fallback, see ticket "HR-Fallback") or the
// user hasn't configured an FTP yet.
func Activity(ctx context.Context, pool *pgxpool.Pool, activityID int64) error {
	var userID int64
	var elapsedSeconds int
	if err := pool.QueryRow(ctx,
		`SELECT user_id, elapsed_seconds FROM activities WHERE id = $1`, activityID,
	).Scan(&userID, &elapsedSeconds); err != nil {
		return err
	}

	var ftpWatts *int
	if err := pool.QueryRow(ctx,
		`SELECT ftp_watts FROM users WHERE id = $1`, userID,
	).Scan(&ftpWatts); err != nil {
		return err
	}
	if ftpWatts == nil {
		return nil
	}

	rows, err := pool.Query(ctx,
		`SELECT power_watts FROM samples WHERE activity_id = $1 AND power_watts IS NOT NULL ORDER BY time`,
		activityID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var powerWatts []float64
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return err
		}
		powerWatts = append(powerWatts, float64(p))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(powerWatts) == 0 {
		return nil
	}

	metrics := ComputePowerMetrics(powerWatts, elapsedSeconds, *ftpWatts)
	if metrics == nil {
		return nil
	}

	_, err = pool.Exec(ctx, `
		UPDATE activities SET
			normalized_power_watts = $2,
			intensity_factor = $3,
			training_stress_score = $4
		WHERE id = $1`,
		activityID, metrics.NormalizedPowerWatts, metrics.IntensityFactor, metrics.TSS,
	)
	return err
}
