package analyze

// Integration test against a real Postgres instance. Skipped if DATABASE_URL
// is unset — see backend/README.md for the scratch-cluster invocation.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/db"
)

func TestComputePowerCurveIndependentOfFTP(t *testing.T) {
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
	// Deliberately no FTP configured — the whole point of #592 is that the
	// power curve does not wait on one.
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, display_name) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("powercurve-test-%d", stamp), "Power Curve Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	var activityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
		VALUES ($1, $2, 'cycling', $3, 40, 40) RETURNING id`,
		userID, fmt.Sprintf("powercurve-test-%d", stamp), start,
	).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	// 40 one-second samples at a constant 200 W — the best average for every
	// window that fits is trivially 200, and no window longer than 40 s
	// should produce a point at all.
	for i := range 40 {
		if _, err := pool.Exec(ctx,
			`INSERT INTO samples (activity_id, time, power_watts) VALUES ($1, $2, 200)`,
			activityID, start.Add(time.Duration(i)*time.Second),
		); err != nil {
			t.Fatalf("insert sample: %v", err)
		}
	}

	if err := Activity(ctx, pool, activityID); err != nil {
		t.Fatalf("Activity: %v", err)
	}

	rows, err := pool.Query(ctx,
		`SELECT duration_seconds, watts FROM power_curve_points WHERE activity_id = $1 ORDER BY duration_seconds`,
		activityID,
	)
	if err != nil {
		t.Fatalf("query power_curve_points: %v", err)
	}
	defer rows.Close()

	got := map[int]int{}
	for rows.Next() {
		var duration, watts int
		if err := rows.Scan(&duration, &watts); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[duration] = watts
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	for _, duration := range []int{5, 30} {
		watts, ok := got[duration]
		if !ok {
			t.Errorf("no power-curve point for %d s, want one (constant 200 W ride, 40 s long)", duration)
			continue
		}
		if watts != 200 {
			t.Errorf("power-curve point for %d s = %d W, want 200", duration, watts)
		}
	}
	for _, duration := range []int{60, 300, 600, 1200, 3600} {
		if watts, ok := got[duration]; ok {
			t.Errorf("power-curve point for %d s = %d W, want none — the ride is only 40 s long", duration, watts)
		}
	}
}

func TestComputePowerCurveWritesNothingWithoutPowerData(t *testing.T) {
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
		`INSERT INTO users (username, display_name) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("powercurve-nopower-test-%d", stamp), "No Power Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	var activityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
		VALUES ($1, $2, 'cycling', $3, 40, 40) RETURNING id`,
		userID, fmt.Sprintf("powercurve-nopower-test-%d", stamp), start,
	).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	for i := range 40 {
		if _, err := pool.Exec(ctx,
			`INSERT INTO samples (activity_id, time, heart_rate) VALUES ($1, $2, 140)`,
			activityID, start.Add(time.Duration(i)*time.Second),
		); err != nil {
			t.Fatalf("insert sample: %v", err)
		}
	}

	if err := ComputePowerCurve(ctx, pool, activityID); err != nil {
		t.Fatalf("ComputePowerCurve: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM power_curve_points WHERE activity_id = $1`, activityID,
	).Scan(&count); err != nil {
		t.Fatalf("count power_curve_points: %v", err)
	}
	if count != 0 {
		t.Errorf("power_curve_points count = %d, want 0 for a ride with no power samples", count)
	}
}

func TestPowerCurveOverTimeOrdersByDateAndIsolatesUsers(t *testing.T) {
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
	makeUser := func(name string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (username, display_name) VALUES ($1, $2) RETURNING id`,
			fmt.Sprintf("powercurve-history-%d-%s", stamp, name), name,
		).Scan(&id); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		return id
	}
	makeRide := func(userID int64, daysAgo int, watts1200 int) int64 {
		t.Helper()
		var activityID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
			VALUES ($1, $2, 'cycling', $3, 1200, 1200) RETURNING id`,
			userID, fmt.Sprintf("powercurve-history-%d-%d-%d", stamp, userID, daysAgo),
			time.Now().Add(-time.Duration(daysAgo)*24*time.Hour),
		).Scan(&activityID); err != nil {
			t.Fatalf("insert activity: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO power_curve_points (activity_id, duration_seconds, watts) VALUES ($1, 1200, $2)`,
			activityID, watts1200,
		); err != nil {
			t.Fatalf("insert power_curve_point: %v", err)
		}
		return activityID
	}

	rider := makeUser("Rider")
	older := makeRide(rider, 20, 220)
	newer := makeRide(rider, 5, 250)

	other := makeUser("Other")
	makeRide(other, 5, 999) // must never appear in rider's history

	history, err := PowerCurveOverTime(ctx, pool, rider, 1200)
	if err != nil {
		t.Fatalf("PowerCurveOverTime: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want 2 (the other rider's ride must not appear)", len(history))
	}
	if history[0].ActivityID != older || history[1].ActivityID != newer {
		t.Fatalf("history = %+v, want the older ride first (chronological)", history)
	}
	if history[0].Watts != 220 || history[1].Watts != 250 {
		t.Errorf("watts = [%d, %d], want [220, 250]", history[0].Watts, history[1].Watts)
	}
}

func TestValidPowerCurveDuration(t *testing.T) {
	if !ValidPowerCurveDuration(1200) {
		t.Error("1200 (20 min) should be a valid duration — it's one of the fixed windows written")
	}
	if ValidPowerCurveDuration(90) {
		t.Error("90 is not one of the fixed windows and should be rejected")
	}
}
