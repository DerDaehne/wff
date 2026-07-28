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

func TestActivityZoneSharesKeepsRidesSeparate(t *testing.T) {
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
		fmt.Sprintf("zoneshares-test-%d", stamp), "Zone Shares Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	insertRide := func(daysAgo int, bpm int) int64 {
		t.Helper()
		start := time.Now().Add(-time.Duration(daysAgo) * 24 * time.Hour)
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
			VALUES ($1, $2, 'cycling', $3, 900, 900) RETURNING id`,
			userID, fmt.Sprintf("zoneshares-test-%d-%d", stamp, daysAgo), start,
		).Scan(&id); err != nil {
			t.Fatalf("insert activity: %v", err)
		}
		for i := range 900 {
			if _, err := pool.Exec(ctx,
				`INSERT INTO samples (activity_id, time, heart_rate) VALUES ($1, $2, $3)`,
				id, start.Add(time.Duration(i)*time.Second), bpm,
			); err != nil {
				t.Fatalf("insert sample: %v", err)
			}
		}
		return id
	}

	// At LTHR 160: 140 bpm (87.5 %) sits in "endurance", 155 bpm (96.9 %) in
	// "threshold".
	easyID := insertRide(2, 140)
	hardID := insertRide(1, 155)

	shares, err := ActivityZoneShares(ctx, pool, []int64{easyID, hardID}, new(160), false)
	if err != nil {
		t.Fatalf("ActivityZoneShares: %v", err)
	}

	dominantZone := func(id int64) string {
		t.Helper()
		z, ok := shares[id]
		if !ok {
			t.Fatalf("no shares for activity %d", id)
		}
		best := ZoneShare{}
		for _, s := range z.Zones {
			if s.Share > best.Share {
				best = s
			}
		}
		return best.Key
	}

	if got := dominantZone(easyID); got != "endurance" {
		t.Errorf("easy ride dominant zone = %q, want endurance", got)
	}
	if got := dominantZone(hardID); got != "threshold" {
		t.Errorf("hard ride dominant zone = %q, want threshold — the two rides must not bleed into each other", got)
	}
}
