package analyze

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// compareWindowDays is how far back the training-success comparison looks —
// four weeks, the same block size weekly.go's trend view uses: long enough
// to smell like a pattern, short enough to still describe how someone
// trains right now.
const compareWindowDays = 28

// CompareEntry is one opted-in rider's relative training-success change —
// never an absolute figure like kilometres, on purpose (#642): CTL is
// already normalised to each rider's own capacity, so "the same percentage
// jump" means something comparable across two people who train completely
// different volumes.
type CompareEntry struct {
	DisplayName string `json:"display_name"`
	IsYou       bool   `json:"is_you"`
	// DeltaCTL is nil when the rider doesn't have four weeks of TSS-based
	// history yet — an honest absence, not a zero pretending to be a real
	// answer.
	DeltaCTL *float64 `json:"delta_ctl"`
}

// CompareResponse is what the comparison endpoint answers. OptedIn is false
// exactly when the caller themselves hasn't opted in — the view is
// symmetric: seeing requires being seen, so there is nothing to show a
// rider who hasn't agreed to that trade.
type CompareResponse struct {
	OptedIn bool           `json:"opted_in"`
	Entries []CompareEntry `json:"entries"`
}

// CompareTrainingSuccess answers "how is everyone's training going" for
// riders who opted in — relative to each rider's own last four weeks, never
// as a raw-volume ranking (#642).
func CompareTrainingSuccess(ctx context.Context, pool *pgxpool.Pool, callerID int64) (CompareResponse, error) {
	var callerOptedIn bool
	if err := pool.QueryRow(ctx,
		`SELECT compare_opt_in FROM users WHERE id = $1`, callerID,
	).Scan(&callerOptedIn); err != nil {
		return CompareResponse{}, err
	}
	if !callerOptedIn {
		return CompareResponse{OptedIn: false, Entries: []CompareEntry{}}, nil
	}

	rows, err := pool.Query(ctx, `SELECT id, display_name FROM users WHERE compare_opt_in = true`)
	if err != nil {
		return CompareResponse{}, err
	}
	type participant struct {
		id   int64
		name string
	}
	var participants []participant
	for rows.Next() {
		var p participant
		if err := rows.Scan(&p.id, &p.name); err != nil {
			rows.Close()
			return CompareResponse{}, err
		}
		participants = append(participants, p)
	}
	if err := rows.Err(); err != nil {
		return CompareResponse{}, err
	}
	rows.Close()

	entries := make([]CompareEntry, 0, len(participants))
	for _, p := range participants {
		series, err := TrainingLoad(ctx, pool, p.id)
		if err != nil {
			return CompareResponse{}, err
		}
		entries = append(entries, CompareEntry{
			DisplayName: p.name,
			IsYou:       p.id == callerID,
			DeltaCTL:    ctlDelta(series),
		})
	}

	// Highest improvement first; riders without enough history yet sort to
	// the end regardless of sign — they aren't "behind", they simply don't
	// have an answer.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].DeltaCTL == nil {
			return false
		}
		if entries[j].DeltaCTL == nil {
			return true
		}
		return *entries[i].DeltaCTL > *entries[j].DeltaCTL
	})

	return CompareResponse{OptedIn: true, Entries: entries}, nil
}

// ctlDelta is how much CTL moved over compareWindowDays — nil when the
// series doesn't yet span a full window (no TSS-based rides at all, or the
// rider's first one is more recent than the window).
func ctlDelta(series []DayLoad) *float64 {
	if len(series) == 0 {
		return nil
	}
	today := series[len(series)-1]
	targetDate := today.Date.AddDate(0, 0, -compareWindowDays)
	if series[0].Date.After(targetDate) {
		return nil
	}

	var before DayLoad
	for _, d := range series {
		if d.Date.After(targetDate) {
			break
		}
		before = d
	}
	delta := today.CTL - before.CTL
	return &delta
}
