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

func TestObservedMaxPicksTheHardestRideOnRecord(t *testing.T) {
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
		fmt.Sprintf("maxhr-test-%d", stamp), "Max HR Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	insertRide := func(daysAgo int, maxHR *int) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds, max_heart_rate)
			VALUES ($1, $2, 'cycling', now() - make_interval(days => $3), 3600, 3600, $4) RETURNING id`,
			userID, fmt.Sprintf("maxhr-test-%d-%d", stamp, daysAgo), daysAgo, maxHR,
		).Scan(&id); err != nil {
			t.Fatalf("insert activity: %v", err)
		}
		return id
	}

	lower, higher := 170, 185
	insertRide(10, &lower)
	wantID := insertRide(3, &higher)
	insertRide(1, nil) // uploaded but no HR strap that day

	got, err := ObservedMax(ctx, pool, userID)
	if err != nil {
		t.Fatalf("ObservedMax: %v", err)
	}
	if got == nil {
		t.Fatal("ObservedMax returned nil, want the 185 bpm ride")
	}
	if got.Bpm != higher {
		t.Errorf("Bpm = %d, want %d", got.Bpm, higher)
	}
	if got.ActivityID != wantID {
		t.Errorf("ActivityID = %d, want %d — the ride that actually recorded 185 bpm", got.ActivityID, wantID)
	}
}

func TestObservedMaxIsNilWithoutAnyHeartRateData(t *testing.T) {
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
		fmt.Sprintf("maxhr-none-%d", stamp), "No HR Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	got, err := ObservedMax(ctx, pool, userID)
	if err != nil {
		t.Fatalf("ObservedMax: %v", err)
	}
	if got != nil {
		t.Errorf("ObservedMax = %+v, want nil for a rider with no rides at all", got)
	}
}
