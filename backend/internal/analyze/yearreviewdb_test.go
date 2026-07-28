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

func TestYearReviewSumsAndPicksHighlights(t *testing.T) {
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
		fmt.Sprintf("yearreview-test-%d", stamp), "Year Review Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	insert := func(month int, distance, gain, tss float64) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO activities
				(user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds,
				 distance_meters, elevation_gain_meters, training_stress_score)
			VALUES ($1, $2, 'cycling', $3, 3600, 3600, $4, $5, $6) RETURNING id`,
			userID, fmt.Sprintf("yearreview-test-%d-%d", stamp, month),
			time.Date(2026, time.Month(month), 15, 10, 0, 0, 0, time.UTC),
			distance, gain, tss,
		).Scan(&id); err != nil {
			t.Fatalf("insert activity: %v", err)
		}
		return id
	}

	longestID := insert(3, 150_000, 500, 80)
	hardestID := insert(7, 50_000, 200, 250)
	insert(1, 30_000, 100, 50)

	// Also plant one ride the year before — must not leak into 2026's sums.
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities
			(user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds, distance_meters)
		VALUES ($1, $2, 'cycling', $3, 3600, 3600, 999999)`,
		userID, fmt.Sprintf("yearreview-test-%d-prevyear", stamp),
		time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
	); err != nil {
		t.Fatalf("insert prior-year activity: %v", err)
	}

	review, err := YearReviewFor(ctx, pool, userID, 2026)
	if err != nil {
		t.Fatalf("YearReviewFor: %v", err)
	}

	if review.RideCount != 3 {
		t.Errorf("RideCount = %d, want 3 (the 2025 ride must not be counted)", review.RideCount)
	}
	wantDistance := 150_000.0 + 50_000.0 + 30_000.0
	if review.DistanceMeters != wantDistance {
		t.Errorf("DistanceMeters = %v, want %v", review.DistanceMeters, wantDistance)
	}
	wantGain := 500.0 + 200.0 + 100.0
	if review.ElevationGainMeters != wantGain {
		t.Errorf("ElevationGainMeters = %v, want %v", review.ElevationGainMeters, wantGain)
	}

	if review.LongestRide == nil || review.LongestRide.ActivityID != longestID {
		t.Errorf("LongestRide = %+v, want activity %d", review.LongestRide, longestID)
	}
	if review.HardestRide == nil || review.HardestRide.ActivityID != hardestID {
		t.Errorf("HardestRide = %+v, want activity %d", review.HardestRide, hardestID)
	}

	empty, err := YearReviewFor(ctx, pool, userID, 2020)
	if err != nil {
		t.Fatalf("YearReviewFor empty year: %v", err)
	}
	if empty.RideCount != 0 || empty.HardestRide != nil || empty.LongestRide != nil {
		t.Errorf("empty year = %+v, want zero rides and no highlights", empty)
	}
}
