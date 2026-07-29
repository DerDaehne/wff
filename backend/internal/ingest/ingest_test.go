package ingest_test

// Integration test against a real Postgres instance (migrations 000001-000004
// applied). Skipped if DATABASE_URL is unset — see backend/README.md for the
// scratch-cluster invocation (same pattern as internal/auth/e2e_test.go).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/ingest"
)

func TestStoreAndDedup(t *testing.T) {
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
	err = pool.QueryRow(ctx,
		`INSERT INTO users (username, display_name) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("ingest-test-%d", stamp), "Ingest Test",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	raw := fitfixture.ValidActivity(999888, created, 20)
	act, err := fitparse.Parse(raw)
	if err != nil {
		t.Fatalf("fitparse.Parse: %v", err)
	}
	externalUID := ingest.ExternalUID(act.FileID, raw)

	activityID, err := ingest.Store(ctx, pool, userID, act, externalUID)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if activityID == 0 {
		t.Fatalf("Store returned activityID = 0")
	}

	var sampleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM samples WHERE activity_id = $1`, activityID).Scan(&sampleCount); err != nil {
		t.Fatalf("count samples: %v", err)
	}
	if sampleCount != len(act.Samples) {
		t.Fatalf("sample count in DB = %d, want %d", sampleCount, len(act.Samples))
	}

	var lapCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM laps WHERE activity_id = $1`, activityID).Scan(&lapCount); err != nil {
		t.Fatalf("count laps: %v", err)
	}
	if lapCount != len(act.Laps) {
		t.Fatalf("lap count in DB = %d, want %d", lapCount, len(act.Laps))
	}

	// Second insert of the same activity must be rejected, not duplicated.
	_, err = ingest.Store(ctx, pool, userID, act, externalUID)
	if !errors.Is(err, ingest.ErrDuplicateActivity) {
		t.Fatalf("second Store() = %v, want ErrDuplicateActivity", err)
	}

	var activityCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM activities WHERE user_id = $1 AND external_uid = $2`,
		userID, externalUID,
	).Scan(&activityCount); err != nil {
		t.Fatalf("count activities: %v", err)
	}
	if activityCount != 1 {
		t.Fatalf("activities row count = %d, want 1 (no duplicate row)", activityCount)
	}

	if err := pool.QueryRow(ctx, `SELECT count(*) FROM samples WHERE activity_id = $1`, activityID).Scan(&sampleCount); err != nil {
		t.Fatalf("count samples after rejected duplicate: %v", err)
	}
	if sampleCount != len(act.Samples) {
		t.Fatalf("sample count after rejected duplicate = %d, want unchanged %d (no partial insert leaked)", sampleCount, len(act.Samples))
	}

	if err := pool.QueryRow(ctx, `SELECT count(*) FROM laps WHERE activity_id = $1`, activityID).Scan(&lapCount); err != nil {
		t.Fatalf("count laps after rejected duplicate: %v", err)
	}
	if lapCount != len(act.Laps) {
		t.Fatalf("lap count after rejected duplicate = %d, want unchanged %d (no partial insert leaked)", lapCount, len(act.Laps))
	}
}

func TestExternalUIDFallback(t *testing.T) {
	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	rawA := fitfixture.ValidActivity(0, created, 5) // SerialNumber 0 -> hash fallback
	rawB := fitfixture.ValidActivity(0, created, 6) // different content -> different hash

	actA, err := fitparse.Parse(rawA)
	if err != nil {
		t.Fatalf("Parse rawA: %v", err)
	}
	actB, err := fitparse.Parse(rawB)
	if err != nil {
		t.Fatalf("Parse rawB: %v", err)
	}

	uidA := ingest.ExternalUID(actA.FileID, rawA)
	uidARepeat := ingest.ExternalUID(actA.FileID, rawA)
	uidB := ingest.ExternalUID(actB.FileID, rawB)

	if uidA != uidARepeat {
		t.Fatalf("ExternalUID not deterministic: %q != %q", uidA, uidARepeat)
	}
	if uidA == uidB {
		t.Fatalf("ExternalUID collided for different content: %q", uidA)
	}
}
