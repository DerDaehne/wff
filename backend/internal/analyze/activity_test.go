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
	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/ingest"
)

func TestActivityPersistsPowerMetrics(t *testing.T) {
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
		`INSERT INTO users (username, display_name, ftp_watts) VALUES ($1, $2, $3) RETURNING id`,
		fmt.Sprintf("analyze-test-%d", stamp), "Analyze Test", 250,
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	raw := fitfixture.ValidActivity(uint32(stamp%1_000_000+1), created, 60)
	act, err := fitparse.Parse(raw)
	if err != nil {
		t.Fatalf("fitparse.Parse: %v", err)
	}
	activityID, err := ingest.Store(ctx, pool, userID, act, ingest.ExternalUID(act.FileID, raw))
	if err != nil {
		t.Fatalf("ingest.Store: %v", err)
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
	if np == nil || ifactor == nil || tss == nil {
		t.Fatalf("metrics = np=%v if=%v tss=%v, want all non-nil (fitfixture rides have power samples + FTP is set)", np, ifactor, tss)
	}
	if *np <= 0 || *ifactor <= 0 || *tss <= 0 {
		t.Fatalf("metrics = np=%v if=%v tss=%v, want all positive", *np, *ifactor, *tss)
	}
}

func TestActivityNoOpWithoutFTP(t *testing.T) {
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
	// No ftp_watts set for this user.
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, display_name) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("analyze-noftp-%d", stamp), "No FTP Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	raw := fitfixture.ValidActivity(uint32(stamp%1_000_000+1), created, 60)
	act, err := fitparse.Parse(raw)
	if err != nil {
		t.Fatalf("fitparse.Parse: %v", err)
	}
	activityID, err := ingest.Store(ctx, pool, userID, act, ingest.ExternalUID(act.FileID, raw))
	if err != nil {
		t.Fatalf("ingest.Store: %v", err)
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
		t.Fatalf("training_stress_score = %v, want nil (no FTP configured)", *tss)
	}
}
