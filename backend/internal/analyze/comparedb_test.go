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

func TestCompareTrainingSuccessRespectsOptIn(t *testing.T) {
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

	// CompareTrainingSuccess queries every opted-in user in the whole
	// database, not just ones this test created — unlike the rest of this
	// package's tests, leftover rows from a previous run (or a crashed one)
	// would silently corrupt the very thing being asserted on. Clean up
	// unconditionally, not just on success.
	//
	// A plain defer, not t.Cleanup: t.Cleanup callbacks run only after the
	// test function itself has returned — by which point the deferred
	// pool.Close() above has already fired. Registering this defer AFTER
	// pool.Close() runs it FIRST (LIFO), while the pool is still open.
	stamp := time.Now().UnixNano()
	var createdUserIDs []int64
	defer func() {
		if len(createdUserIDs) == 0 {
			return
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1)`, createdUserIDs); err != nil {
			t.Logf("cleanup: could not delete test users: %v", err)
		}
	}()

	makeUser := func(name string, optedIn bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (username, display_name, compare_opt_in) VALUES ($1, $2, $3) RETURNING id`,
			fmt.Sprintf("compare-test-%d-%s", stamp, name), name, optedIn,
		).Scan(&id); err != nil {
			t.Fatalf("insert user %s: %v", name, err)
		}
		createdUserIDs = append(createdUserIDs, id)
		return id
	}
	insertRide := func(userID int64, daysAgo int, tss float64) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO activities
				(user_id, external_uid, sport, started_at, elapsed_seconds, moving_seconds, training_stress_score)
			VALUES ($1, $2, 'cycling', $3, 3600, 3600, $4)`,
			userID, fmt.Sprintf("compare-test-%d-%d-%d", stamp, userID, daysAgo),
			time.Now().Add(-time.Duration(daysAgo)*24*time.Hour), tss,
		); err != nil {
			t.Fatalf("insert activity for user %d: %v", userID, err)
		}
	}

	// Rising: a big effort five weeks ago has decayed, a bigger one recently
	// pushes CTL up — a real, improving trend.
	rising := makeUser("Rising", true)
	insertRide(rising, 35, 60)
	insertRide(rising, 1, 120)

	// Opted in, but never ridden with a computed TSS — honest nil, not zero.
	makeUser("NoHistory", true)

	// Never opted in — must not appear to anyone, and (in its own request)
	// must not see anyone else either.
	notParticipating := makeUser("NotParticipating", false)
	insertRide(notParticipating, 35, 200)
	insertRide(notParticipating, 1, 200)

	t.Run("a rider who has not opted in gets no list at all", func(t *testing.T) {
		result, err := CompareTrainingSuccess(ctx, pool, notParticipating)
		if err != nil {
			t.Fatalf("CompareTrainingSuccess: %v", err)
		}
		if result.OptedIn {
			t.Fatal("OptedIn = true for a rider who never opted in")
		}
		if len(result.Entries) != 0 {
			t.Fatalf("Entries = %v, want empty for a non-participant", result.Entries)
		}
	})

	t.Run("opted-in riders see each other but not the non-participant", func(t *testing.T) {
		result, err := CompareTrainingSuccess(ctx, pool, rising)
		if err != nil {
			t.Fatalf("CompareTrainingSuccess: %v", err)
		}
		if !result.OptedIn {
			t.Fatal("OptedIn = false for a rider who did opt in")
		}
		if len(result.Entries) != 2 {
			t.Fatalf("Entries = %v, want exactly the 2 opted-in riders", result.Entries)
		}

		byName := map[string]CompareEntry{}
		for _, e := range result.Entries {
			byName[e.DisplayName] = e
		}
		if _, ok := byName["NotParticipating"]; ok {
			t.Error("a non-participant appeared in an opted-in rider's comparison")
		}

		risingEntry, ok := byName["Rising"]
		if !ok || !risingEntry.IsYou {
			t.Errorf("Rising's own entry = %+v, want IsYou true", risingEntry)
		}
		if risingEntry.DeltaCTL == nil || *risingEntry.DeltaCTL <= 0 {
			t.Errorf("Rising's delta = %v, want a positive number (CTL rose)", risingEntry.DeltaCTL)
		}

		noHistoryEntry, ok := byName["NoHistory"]
		if !ok || noHistoryEntry.IsYou {
			t.Errorf("NoHistory's entry = %+v, want present and IsYou false", noHistoryEntry)
		}
		if noHistoryEntry.DeltaCTL != nil {
			t.Errorf("NoHistory's delta = %v, want nil — no TSS-based rides at all", *noHistoryEntry.DeltaCTL)
		}

		// The rider with a real (positive) delta must sort ahead of the one
		// with no answer at all.
		if result.Entries[0].DisplayName != "Rising" {
			t.Errorf("Entries[0] = %q, want Rising first (nil deltas sort last)", result.Entries[0].DisplayName)
		}
	})
}
