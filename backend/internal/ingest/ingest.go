// Package ingest persists a parsed fitparse.Activity into activities/samples,
// including the dedup check. No HTTP concerns here (see the upload endpoint
// ticket).
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDuplicateActivity = errors.New("activity already exists for this user")

const (
	uniqueViolation  = "23505"
	deadlockDetected = "40P01"
	maxStoreAttempts = 3
)

// ExternalUID derives a deterministic per-user dedup key from a FIT FileId.
// Device-based (SerialNumber+TimeCreated) rather than a content hash: a
// re-export of the same recording (e.g. a repeat Sigma Cloud export) is
// guaranteed to carry the same SerialNumber+TimeCreated even if the encoded
// bytes differ slightly, but is not guaranteed to hash identically.
// Falls back to a content hash only when the device didn't set a serial
// number at all (e.g. manually assembled files).
func ExternalUID(fileID fitparse.FileID, rawContent []byte) string {
	if fileID.SerialNumber != 0 {
		return fmt.Sprintf("%d-%d", fileID.SerialNumber, fileID.TimeCreated.Unix())
	}
	sum := sha256.Sum256(rawContent)
	return hex.EncodeToString(sum[:])
}

// dedupeSamplesByTime drops any sample whose timestamp repeats an earlier
// one in the same file, keeping the first occurrence.
//
// (activity_id, time) is the samples table's primary key, and CopyFrom has
// no ON CONFLICT to fall back on the way a plain INSERT would — a single
// repeated timestamp fails the whole batch (#700, a real device: two Record
// messages both rounding to the same second around a pause/resume). Losing
// one second of one field's worth of data to an admittedly arbitrary
// first-wins rule is a fair trade against rejecting the entire ride.
func dedupeSamplesByTime(samples []fitparse.Sample) []fitparse.Sample {
	seen := make(map[time.Time]bool, len(samples))
	out := make([]fitparse.Sample, 0, len(samples))
	for _, s := range samples {
		if seen[s.Time] {
			continue
		}
		seen[s.Time] = true
		out = append(out, s)
	}
	return out
}

// Store persists a parsed activity and its samples for userID in a single
// transaction. Returns ErrDuplicateActivity if (userID, externalUID) already
// exists; the caller decides how to surface that (e.g. HTTP 409).
//
// Concurrent inserts into a TimescaleDB hypertable that both need to create
// the same new chunk can deadlock (SQLSTATE 40P01, confirmed via the #557
// end-to-end regression run: two test processes uploading around the same
// time reliably reproduced it). Deadlock is retried from scratch a few
// times, per Postgres's own recommended handling of 40P01 — the transaction
// did no partial work (it never committed), so a clean retry is safe.
func Store(ctx context.Context, pool *pgxpool.Pool, userID int64, act *fitparse.Activity, externalUID string) (activityID int64, err error) {
	for attempt := 1; attempt <= maxStoreAttempts; attempt++ {
		activityID, err = storeOnce(ctx, pool, userID, act, externalUID)
		if err == nil || errors.Is(err, ErrDuplicateActivity) {
			return activityID, err
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != deadlockDetected {
			return 0, err
		}
	}
	return 0, err
}

func storeOnce(ctx context.Context, pool *pgxpool.Pool, userID int64, act *fitparse.Activity, externalUID string) (activityID int64, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO activities (
			user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds,
			distance_meters, elevation_gain_meters, avg_power_watts, max_power_watts,
			normalized_power_watts, avg_heart_rate, max_heart_rate, avg_cadence, max_cadence,
			intensity_factor, training_stress_score,
			total_descent_meters, avg_grade_percent, avg_pos_grade_percent, avg_neg_grade_percent,
			max_pos_grade_percent, max_neg_grade_percent, threshold_power_watts,
			total_calories_kcal, metabolic_calories_kcal,
			avg_left_right_balance_percent, avg_left_right_balance_right_leg,
			avg_left_torque_effectiveness_percent, avg_right_torque_effectiveness_percent,
			avg_left_pedal_smoothness_percent, avg_right_pedal_smoothness_percent,
			avg_combined_pedal_smoothness_percent
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33)
		RETURNING id`,
		userID, externalUID, act.Sport, act.StartedAt, act.ElapsedSeconds, act.MovingSeconds,
		act.DistanceMeters, act.ElevationGainMeters, act.AvgPowerWatts, act.MaxPowerWatts,
		act.NormalizedPowerWatts, act.AvgHeartRate, act.MaxHeartRate, act.AvgCadence, act.MaxCadence,
		act.IntensityFactor, act.TrainingStressScore,
		act.TotalDescentMeters, act.AvgGradePercent, act.AvgPosGradePercent, act.AvgNegGradePercent,
		act.MaxPosGradePercent, act.MaxNegGradePercent, act.ThresholdPowerWatts,
		act.TotalCaloriesKcal, act.MetabolicCaloriesKcal,
		act.AvgLeftRightBalancePercent, act.AvgLeftRightBalanceRightLeg,
		act.AvgLeftTorqueEffectivenessPercent, act.AvgRightTorqueEffectivenessPercent,
		act.AvgLeftPedalSmoothnessPercent, act.AvgRightPedalSmoothnessPercent,
		act.AvgCombinedPedalSmoothnessPercent,
	).Scan(&activityID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return 0, ErrDuplicateActivity
		}
		return 0, err
	}

	if len(act.Samples) > 0 {
		samples := dedupeSamplesByTime(act.Samples)
		rows := make([][]any, len(samples))
		for i, s := range samples {
			rows[i] = []any{
				activityID, s.Time, s.Lat, s.Lon, s.AltitudeMeters, s.PowerWatts, s.HeartRate, s.Cadence, s.SpeedMps, s.TemperatureCelsius,
				s.GradePercent, s.CaloriesKcal, s.LeftRightBalancePercent, s.LeftRightBalanceRightLeg,
				s.LeftTorqueEffectivenessPercent, s.RightTorqueEffectivenessPercent,
				s.LeftPedalSmoothnessPercent, s.RightPedalSmoothnessPercent, s.CombinedPedalSmoothnessPercent,
				s.GpsAccuracyMeters, s.Resistance,
			}
		}
		_, err = tx.CopyFrom(ctx,
			pgx.Identifier{"samples"},
			[]string{
				"activity_id", "time", "lat", "lon", "altitude_meters", "power_watts", "heart_rate", "cadence", "speed_mps", "temperature_celsius",
				"grade_percent", "calories_kcal", "left_right_balance_percent", "left_right_balance_right_leg",
				"left_torque_effectiveness_percent", "right_torque_effectiveness_percent",
				"left_pedal_smoothness_percent", "right_pedal_smoothness_percent", "combined_pedal_smoothness_percent",
				"gps_accuracy_meters", "resistance",
			},
			pgx.CopyFromRows(rows),
		)
		if err != nil {
			return 0, err
		}
	}

	if len(act.Laps) > 0 {
		rows := make([][]any, len(act.Laps))
		for i, l := range act.Laps {
			rows[i] = []any{
				activityID, i, l.StartedAt, l.ElapsedSeconds, l.DistanceMeters,
				l.AvgPowerWatts, l.MaxPowerWatts, l.AvgHeartRate, l.MaxHeartRate, l.AvgSpeedMps, l.MaxSpeedMps,
			}
		}
		_, err = tx.CopyFrom(ctx,
			pgx.Identifier{"laps"},
			[]string{
				"activity_id", "lap_index", "started_at", "elapsed_seconds", "distance_meters",
				"avg_power_watts", "max_power_watts", "avg_heart_rate", "max_heart_rate", "avg_speed_mps", "max_speed_mps",
			},
			pgx.CopyFromRows(rows),
		)
		if err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return activityID, nil
}
