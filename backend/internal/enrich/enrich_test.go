package enrich_test

// Integration test against a real Postgres instance (migrations
// 000001-000006 applied) and a local fake Open-Meteo server. Skipped if
// DATABASE_URL is unset — see backend/README.md for the scratch-cluster
// invocation.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/enrich"
	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/ingest"
	"github.com/DerDaehne/wff/internal/openmeteo"
)

func TestActivityEnrichesAvailableBuckets(t *testing.T) {
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
		fmt.Sprintf("enrich-test-%d", stamp), "Enrich Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	raw := fitfixture.ValidActivity(uint32(stamp%1_000_000+1), created, 10) // ~10s of records, all within one UTC hour
	act, err := fitparse.Parse(raw)
	if err != nil {
		t.Fatalf("fitparse.Parse: %v", err)
	}
	activityID, err := ingest.Store(ctx, pool, userID, act, ingest.ExternalUID(act.FileID, raw))
	if err != nil {
		t.Fatalf("ingest.Store: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hourly":{"time":["2026-07-20T08:00"],"temperature_2m":[19.5],"wind_speed_10m":[4.0],"wind_direction_10m":[90],"precipitation":[0.2]}}`))
	}))
	defer server.Close()

	client := openmeteo.New(server.URL)
	result, err := enrich.Activity(ctx, pool, client, activityID)
	if err != nil {
		t.Fatalf("enrich.Activity: %v", err)
	}
	if result.BucketsAttempted != 1 || result.BucketsEnriched != 1 || !result.Complete() {
		t.Fatalf("result = %+v, want 1/1 complete", result)
	}

	var temp, windSpeed, windDir, precip float64
	var headwind *float64
	if err := pool.QueryRow(ctx,
		`SELECT temperature_celsius, wind_speed_mps, wind_direction_deg, precipitation_mm, headwind_mps
		 FROM enrichment WHERE activity_id = $1`, activityID,
	).Scan(&temp, &windSpeed, &windDir, &precip, &headwind); err != nil {
		t.Fatalf("query enrichment row: %v", err)
	}
	if temp != 19.5 || windSpeed != 4.0 || windDir != 90 || precip != 0.2 {
		t.Fatalf("enrichment row = temp=%v wind=%v dir=%v precip=%v, want 19.5/4.0/90/0.2", temp, windSpeed, windDir, precip)
	}

	// Re-run: must upsert, not duplicate.
	if _, err := enrich.Activity(ctx, pool, client, activityID); err != nil {
		t.Fatalf("second enrich.Activity: %v", err)
	}
	var rowCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM enrichment WHERE activity_id = $1`, activityID).Scan(&rowCount); err != nil {
		t.Fatalf("count enrichment rows: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("enrichment row count after re-run = %d, want 1 (upsert, no duplicate)", rowCount)
	}
}

func TestActivityWithoutGPSIsSkipped(t *testing.T) {
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
		fmt.Sprintf("enrich-indoor-%d", stamp), "Enrich Indoor Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	// A trainer/indoor ride: manually insert an activity with samples that
	// have no GPS at all (fitfixture always sets GPS on 2/3 of records, so
	// build the activity row directly instead of going through the full
	// ingest path).
	var activityID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds)
		VALUES ($1, $2, 'cycling', now(), 600, 600) RETURNING id`,
		userID, fmt.Sprintf("indoor-%d", stamp),
	).Scan(&activityID); err != nil {
		t.Fatalf("insert indoor activity: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO samples (activity_id, time, power_watts, heart_rate) VALUES ($1, now(), 200, 140)`,
		activityID,
	); err != nil {
		t.Fatalf("insert indoor sample: %v", err)
	}

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	result, err := enrich.Activity(ctx, pool, openmeteo.New(server.URL), activityID)
	if err != nil {
		t.Fatalf("enrich.Activity: %v", err)
	}
	if result.BucketsAttempted != 0 {
		t.Fatalf("result = %+v, want 0 buckets attempted for a GPS-less activity", result)
	}
	if called {
		t.Fatalf("openmeteo was called for a GPS-less activity, should have been skipped entirely")
	}
}
