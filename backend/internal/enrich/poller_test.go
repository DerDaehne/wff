package enrich_test

// Integration test against a real Postgres instance. Skipped if DATABASE_URL
// is unset — see backend/README.md for the scratch-cluster invocation.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/enrich"
	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/ingest"
	"github.com/DerDaehne/wff/internal/openmeteo"
)

// TestPollerRetriesUntilDataAvailable simulates the real-world steady state
// this whole feature exists for: ERA5 not having data yet on the first
// attempt (the normal case, not an edge case — see arch-wff-enrichment).
// A failed attempt must not crash and must remain a FindIncomplete
// candidate; once the (fake) weather data becomes available, the next
// attempt — i.e. what the ticker-driven poller would do on its next tick —
// must complete it.
func TestPollerRetriesUntilDataAvailable(t *testing.T) {
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
		fmt.Sprintf("poller-test-%d", stamp), "Poller Test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	raw := fitfixture.ValidActivity(uint32(stamp%1_000_000+1), created, 10)
	act, err := fitparse.Parse(raw)
	if err != nil {
		t.Fatalf("fitparse.Parse: %v", err)
	}
	activityID, err := ingest.Store(ctx, pool, userID, act, ingest.ExternalUID(act.FileID, raw))
	if err != nil {
		t.Fatalf("ingest.Store: %v", err)
	}

	var dataAvailable atomic.Bool // flips true to simulate ERA5 catching up
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if dataAvailable.Load() {
			w.Write([]byte(`{"hourly":{"time":["2026-07-20T08:00"],"temperature_2m":[19.5],"wind_speed_10m":[4.0],"wind_direction_10m":[90],"precipitation":[0.2]}}`))
			return
		}
		w.Write([]byte(`{"hourly":{"time":["2026-07-20T08:00"],"temperature_2m":[null],"wind_speed_10m":[null],"wind_direction_10m":[null],"precipitation":[null]}}`))
	}))
	defer server.Close()
	client := openmeteo.New(server.URL)

	// First attempt: data not yet available. Must not error, must not
	// complete, and the activity must remain a FindIncomplete candidate.
	result, err := enrich.Activity(ctx, pool, client, activityID)
	if err != nil {
		t.Fatalf("first enrich.Activity: %v", err)
	}
	if result.Complete() {
		t.Fatalf("first attempt reported complete, want incomplete (data not yet available)")
	}
	candidates, err := enrich.FindIncomplete(ctx, pool)
	if err != nil {
		t.Fatalf("FindIncomplete: %v", err)
	}
	if !containsID(candidates, activityID) {
		t.Fatalf("FindIncomplete = %v, want it to include activity %d after a not-yet-available attempt", candidates, activityID)
	}

	// Simulate ERA5 catching up, then simulate the poller's next tick.
	dataAvailable.Store(true)
	result, err = enrich.Activity(ctx, pool, client, activityID)
	if err != nil {
		t.Fatalf("second enrich.Activity: %v", err)
	}
	if !result.Complete() {
		t.Fatalf("second attempt result = %+v, want complete now that data is available", result)
	}
	candidates, err = enrich.FindIncomplete(ctx, pool)
	if err != nil {
		t.Fatalf("FindIncomplete after completion: %v", err)
	}
	if containsID(candidates, activityID) {
		t.Fatalf("FindIncomplete still lists activity %d after it was fully enriched", activityID)
	}
}

func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
