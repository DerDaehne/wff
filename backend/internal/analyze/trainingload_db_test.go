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
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTrainingLoadSumsMultipleRidesPerDay(t *testing.T) {
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
		fmt.Sprintf("load-test-%d", stamp), "Load Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	// Two rides on the same UTC calendar day: a morning ride and an evening
	// ride, TSS 40 and 60 - must be combined into a single day's TSS=100.
	// This is also the user's first-ever ride day, so it's series[0]
	// regardless of how many (decaying, TSS=0) days follow up to today.
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	insertActivityWithTSS(t, ctx, pool, userID, fmt.Sprintf("morning-%d", stamp), day.Add(7*time.Hour), 40)
	insertActivityWithTSS(t, ctx, pool, userID, fmt.Sprintf("evening-%d", stamp), day.Add(18*time.Hour), 60)

	series, err := analyze.TrainingLoad(ctx, pool, userID)
	if err != nil {
		t.Fatalf("TrainingLoad: %v", err)
	}
	if len(series) == 0 {
		t.Fatal("TrainingLoad returned an empty series")
	}
	if !series[0].Date.Equal(day) {
		t.Fatalf("series[0].Date = %v, want %v (the first ride day)", series[0].Date, day)
	}
	if series[0].TSS != 100 {
		t.Fatalf("series[0].TSS = %v, want 100 (40+60 summed)", series[0].TSS)
	}
}

func insertActivityWithTSS(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int64, externalUID string, startedAt time.Time, tss float64) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds, training_stress_score)
		VALUES ($1, $2, 'cycling', $3, 3600, 3600, $4)`,
		userID, externalUID, startedAt, tss,
	)
	if err != nil {
		t.Fatalf("insert activity with TSS: %v", err)
	}
}
