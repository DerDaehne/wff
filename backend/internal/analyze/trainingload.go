package analyze

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ctlAlpha = 1.0 / 42 // CTL time constant: 42 days
	atlAlpha = 1.0 / 7  // ATL time constant: 7 days
)

// DayLoad is one day's point in a training-load series: the TSS accrued
// that day (0 on rest days) plus the resulting CTL/ATL and the TSB the
// rider had entering the day.
type DayLoad struct {
	Date time.Time `json:"date"` // UTC calendar day, midnight
	TSS  float64   `json:"tss"`
	CTL  float64   `json:"ctl"`
	ATL  float64   `json:"atl"`
	TSB  float64   `json:"tsb"`
}

// TrainingLoad computes the daily CTL/ATL/TSB series for a user, from their
// first ride with a computed TSS through today (inclusive) — on-the-fly,
// not materialized (see arch-wff-analyze). Rest days count as TSS=0 and
// still decay CTL/ATL; that's the normal case, not a special one. TSB for a
// given day reflects CTL/ATL as of the *previous* day — the form the rider
// had entering that day, before it's own training stress is absorbed.
func TrainingLoad(ctx context.Context, pool *pgxpool.Pool, userID int64) ([]DayLoad, error) {
	tssByDay, days, err := loadDailyTSS(ctx, pool, userID)
	if err != nil {
		return nil, err
	}
	if len(days) == 0 {
		return nil, nil
	}

	firstDay := days[0]
	today := time.Now().UTC().Truncate(24 * time.Hour)
	return computeSeries(tssByDay, firstDay, today), nil
}

// computeSeries is the pure recursion (no DB access), factored out so it's
// directly unit-testable against hand-calculated reference sequences.
func computeSeries(tssByDay map[time.Time]float64, firstDay, lastDay time.Time) []DayLoad {
	var series []DayLoad
	var ctl, atl float64
	for day := firstDay; !day.After(lastDay); day = day.AddDate(0, 0, 1) {
		tsb := ctl - atl // form entering this day, before today's TSS below

		tss := tssByDay[day]
		ctl = ctl*(1-ctlAlpha) + tss*ctlAlpha
		atl = atl*(1-atlAlpha) + tss*atlAlpha

		series = append(series, DayLoad{Date: day, TSS: tss, CTL: ctl, ATL: atl, TSB: tsb})
	}
	return series
}

// loadDailyTSS sums training_stress_score per UTC calendar day (multiple
// rides on the same day are combined into one day's load) and returns the
// sorted list of days that have at least one computed TSS.
func loadDailyTSS(ctx context.Context, pool *pgxpool.Pool, userID int64) (map[time.Time]float64, []time.Time, error) {
	rows, err := pool.Query(ctx, `
		SELECT (started_at AT TIME ZONE 'UTC')::date AS day, sum(training_stress_score)
		FROM activities
		WHERE user_id = $1 AND training_stress_score IS NOT NULL
		GROUP BY 1
		ORDER BY 1`,
		userID,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	tssByDay := make(map[time.Time]float64)
	var days []time.Time
	for rows.Next() {
		var day time.Time
		var tss float64
		if err := rows.Scan(&day, &tss); err != nil {
			return nil, nil, err
		}
		day = day.UTC().Truncate(24 * time.Hour)
		tssByDay[day] = tss
		days = append(days, day)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return tssByDay, days, nil
}
