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
			intensity_factor, training_stress_score
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id`,
		userID, externalUID, act.Sport, act.StartedAt, act.ElapsedSeconds, act.MovingSeconds,
		act.DistanceMeters, act.ElevationGainMeters, act.AvgPowerWatts, act.MaxPowerWatts,
		act.NormalizedPowerWatts, act.AvgHeartRate, act.MaxHeartRate, act.AvgCadence, act.MaxCadence,
		act.IntensityFactor, act.TrainingStressScore,
	).Scan(&activityID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return 0, ErrDuplicateActivity
		}
		return 0, err
	}

	if len(act.Samples) > 0 {
		rows := make([][]any, len(act.Samples))
		for i, s := range act.Samples {
			rows[i] = []any{activityID, s.Time, s.Lat, s.Lon, s.AltitudeMeters, s.PowerWatts, s.HeartRate, s.Cadence, s.SpeedMps, s.TemperatureCelsius}
		}
		_, err = tx.CopyFrom(ctx,
			pgx.Identifier{"samples"},
			[]string{"activity_id", "time", "lat", "lon", "altitude_meters", "power_watts", "heart_rate", "cadence", "speed_mps", "temperature_celsius"},
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
