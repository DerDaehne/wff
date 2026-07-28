package analyze

// Integration test against a real Postgres instance. Skipped if DATABASE_URL
// is unset — see backend/README.md for the scratch-cluster invocation.
//
// The zone aggregation runs in SQL (window function, bucketing, gap filter),
// so it cannot be checked with the pure-Go tests next door — and it is exactly
// the kind of query where an off-by-one lands silently.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/db"
)

func TestZoneSecondsCountsTimeNotSamples(t *testing.T) {
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
		`INSERT INTO users (username, display_name, lthr_bpm) VALUES ($1, $2, 160) RETURNING id`,
		fmt.Sprintf("zones-test-%d", stamp), "Zones Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	start := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	var activityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
		VALUES ($1, $2, 'cycling', $3, 480, 480) RETURNING id`,
		userID, fmt.Sprintf("zones-test-%d", stamp), start,
	).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	// At LTHR 160 the edges are 129.6 / 144 / 150.4 / 160.
	insert := func(offset, count, bpm int) {
		t.Helper()
		for i := range count {
			if _, err := pool.Exec(ctx,
				`INSERT INTO samples (activity_id, time, heart_rate) VALUES ($1, $2, $3)`,
				activityID, start.Add(time.Duration(offset+i)*time.Second), bpm,
			); err != nil {
				t.Fatalf("insert sample: %v", err)
			}
		}
	}
	insert(0, 60, 120)   // recovery
	insert(60, 60, 140)  // endurance
	insert(420, 60, 155) // threshold, after a five-minute stop

	seconds, err := zoneSeconds(ctx, pool, []int64{activityID}, 160)
	if err != nil {
		t.Fatalf("zoneSeconds: %v", err)
	}

	// The first sample of the ride has no predecessor, so its second is not
	// counted anywhere; the step between two blocks belongs to the later one.
	want := []int{59, 60, 0, 59, 0}
	for i := range want {
		if seconds[i] != want[i] {
			t.Errorf("zone %d = %d s, want %d s (got %v)", i, seconds[i], want[i], seconds)
			break
		}
	}

	total := 0
	for _, s := range seconds {
		total += s
	}
	if total != 178 {
		t.Errorf("total = %d s, want 178 — the five-minute stop must not be counted as riding", total)
	}
}
