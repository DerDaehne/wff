package analyze

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MilestoneFacts are this rider's best-ever figures from before the current
// ride — never a comparison to other riders, only to their own history
// (#636). A nil field means no earlier ride had a comparable value, which is
// exactly the case where a new one cannot be a "record" yet.
type MilestoneFacts struct {
	LongestPriorMeters      *float64
	MostClimbingPriorMeters *float64
}

// PriorBests loads the rider's best-ever distance and climbing figures from
// strictly before the given moment — an unlimited MAX() over the whole
// history, not the last-30-rides window the pace/load comparisons use
// (comparisonStatement in ridestory.go): a lifetime record can sit far
// earlier than that window.
func PriorBests(ctx context.Context, pool *pgxpool.Pool, userID int64, beforeStartedAt time.Time) (MilestoneFacts, error) {
	var f MilestoneFacts
	err := pool.QueryRow(ctx, `
		SELECT max(distance_meters), max(elevation_gain_meters)
		FROM activities
		WHERE user_id = $1 AND started_at < $2`,
		userID, beforeStartedAt,
	).Scan(&f.LongestPriorMeters, &f.MostClimbingPriorMeters)
	return f, err
}

// milestoneStatements says which personal records this ride just set. Only
// ever a subset of two so far — VAM and weekly-volume records would need to
// re-derive figures that aren't stored columns across the whole history,
// which is a different cost profile; left for a later pass (#636).
func milestoneStatements(f RideFacts) []Statement {
	var out []Statement
	if s, ok := distanceMilestone(f); ok {
		out = append(out, s)
	}
	if s, ok := climbMilestone(f); ok {
		out = append(out, s)
	}
	return out
}

func distanceMilestone(f RideFacts) (Statement, bool) {
	meters := f.distanceMeters()
	prior := f.Milestones.LongestPriorMeters
	if meters <= 0 || prior == nil || meters <= *prior {
		return Statement{}, false
	}
	return Statement{
		Text: fmt.Sprintf(
			"Das war deine bisher längste Fahrt — %s km mehr als deine bisherige weiteste.",
			decimal((meters-*prior)/1000, 1)),
		Metric: fmt.Sprintf("%s km · bisher %s km", decimal(meters/1000, 1), decimal(*prior/1000, 1)),
		Kind:   "milestone",
	}, true
}

func climbMilestone(f RideFacts) (Statement, bool) {
	if f.ElevationGainMeters == nil {
		return Statement{}, false
	}
	gain := *f.ElevationGainMeters
	prior := f.Milestones.MostClimbingPriorMeters
	if gain <= 0 || prior == nil || gain <= *prior {
		return Statement{}, false
	}
	return Statement{
		Text: fmt.Sprintf(
			"So viele Höhenmeter wie heute hast du an einem Stück noch nie gesammelt — %d mehr als bisher.",
			int(gain-*prior+0.5)),
		Metric: fmt.Sprintf("%d Höhenmeter · bisher %d", int(gain+0.5), int(*prior+0.5)),
		Kind:   "milestone",
	}, true
}
