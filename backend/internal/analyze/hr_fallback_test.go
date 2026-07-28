package analyze_test

// Integration test against a real Postgres instance. Skipped if DATABASE_URL
// is unset — see backend/README.md for the scratch-cluster invocation.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/analyze"
	"github.com/DerDaehne/wff/internal/db"
)

func almostEqual(a, b, tolerance float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}

func TestActivityHRFallbackWhenNoPowerData(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping live-Postgres integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	stamp := time.Now().UnixNano()
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, display_name, lthr_bpm) VALUES ($1, $2, $3) RETURNING id`,
		fmt.Sprintf("hr-fallback-%d", stamp), "HR Fallback Test", 165,
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	// An activity with heart-rate samples but no power at all (e.g. an
	// older HR-only device) — built directly, since fitfixture always
	// includes power.
	var activityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
		VALUES ($1, $2, 'cycling', now(), 3600, 3600) RETURNING id`,
		userID, fmt.Sprintf("hr-only-%d", stamp),
	).Scan(&activityID); err != nil {
		t.Fatalf("insert HR-only activity: %v", err)
	}
	// Constant 150 bpm, one sample per second for an hour. Per-second is what
	// a real recording looks like, and the load is now integrated over the
	// sample gaps rather than read off the ride's average.
	if _, err := pool.Exec(ctx, `
		INSERT INTO samples (activity_id, time, heart_rate)
		SELECT $1, now() + make_interval(secs => g), 150 FROM generate_series(0, 3600) g`,
		activityID,
	); err != nil {
		t.Fatalf("insert HR samples: %v", err)
	}

	if err := analyze.Activity(ctx, pool, activityID); err != nil {
		t.Fatalf("analyze.Activity: %v", err)
	}

	var np, ifactor, tss *float64
	if err := pool.QueryRow(ctx,
		`SELECT normalized_power_watts, intensity_factor, training_stress_score FROM activities WHERE id = $1`,
		activityID,
	).Scan(&np, &ifactor, &tss); err != nil {
		t.Fatalf("query activity metrics: %v", err)
	}
	if np != nil {
		t.Errorf("normalized_power_watts = %v, want nil (HR-only ride has no NP)", *np)
	}
	wantIF := 150.0 / 165.0
	if ifactor == nil || !almostEqual(*ifactor, wantIF, 1e-6) {
		t.Errorf("intensity_factor = %v, want ~%v", ifactor, wantIF)
	}
	wantTSS := wantIF * wantIF * 100
	if tss == nil || !almostEqual(*tss, wantTSS, 1e-6) {
		t.Errorf("training_stress_score = %v, want ~%v", tss, wantTSS)
	}
}

func TestActivityHRFallbackNoOpWithoutLTHR(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping live-Postgres integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	stamp := time.Now().UnixNano()
	var userID int64
	// No lthr_bpm configured.
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, display_name) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("hr-nolthr-%d", stamp), "No LTHR Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	var activityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
		VALUES ($1, $2, 'cycling', now(), 3600, 3600) RETURNING id`,
		userID, fmt.Sprintf("hr-nolthr-act-%d", stamp),
	).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO samples (activity_id, time, heart_rate) VALUES ($1, now(), 150)`, activityID,
	); err != nil {
		t.Fatalf("insert HR sample: %v", err)
	}

	if err := analyze.Activity(ctx, pool, activityID); err != nil {
		t.Fatalf("analyze.Activity: %v", err)
	}

	var tss *float64
	if err := pool.QueryRow(ctx,
		`SELECT training_stress_score FROM activities WHERE id = $1`, activityID,
	).Scan(&tss); err != nil {
		t.Fatalf("query training_stress_score: %v", err)
	}
	if tss != nil {
		t.Fatalf("training_stress_score = %v, want nil (no LTHR configured)", *tss)
	}
}

// A café stop must not be counted as half an hour at riding intensity, and a
// ride left on the old average-based number must be brought onto the new one
// without anybody touching it (#622).
func TestRecomputeHeartRateLoadDropsStopsAndFixesOldRows(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping live-Postgres integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	stamp := time.Now().UnixNano()
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, display_name, lthr_bpm) VALUES ($1, $2, $3) RETURNING id`,
		fmt.Sprintf("hr-stop-%d", stamp), "HR Stop Test", 165,
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	// Two half-hours of riding at 150 bpm with a 30-minute stop in between,
	// and a deliberately wrong TSS already stored — as if the row had been
	// scored by the old formula.
	var activityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at,
			elapsed_seconds, moving_seconds, avg_heart_rate, training_stress_score)
		VALUES ($1, $2, 'cycling', now(), 9000, 3600, 150, 999) RETURNING id`,
		userID, fmt.Sprintf("hr-stop-act-%d", stamp),
	).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO samples (activity_id, time, heart_rate)
		SELECT $1::bigint, now() + make_interval(secs => g), 150 FROM generate_series(0, 1800) g
		UNION ALL
		SELECT $1::bigint, now() + make_interval(secs => g + 3600), 150 FROM generate_series(0, 1800) g`,
		activityID,
	); err != nil {
		t.Fatalf("insert HR samples: %v", err)
	}

	if err := analyze.RecomputeHeartRateLoad(ctx, pool); err != nil {
		t.Fatalf("analyze.RecomputeHeartRateLoad: %v", err)
	}

	var tss *float64
	if err := pool.QueryRow(ctx,
		`SELECT training_stress_score FROM activities WHERE id = $1`, activityID,
	).Scan(&tss); err != nil {
		t.Fatalf("query training_stress_score: %v", err)
	}
	// 3600 recorded seconds, the 30-minute gap contributing nothing.
	ifactor := 150.0 / 165.0
	wantTSS := ifactor * ifactor * 100
	if tss == nil || !almostEqual(*tss, wantTSS, 1e-6) {
		t.Errorf("training_stress_score = %v, want ~%v (the stop must not count)", tss, wantTSS)
	}
}
