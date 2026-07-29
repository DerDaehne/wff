package analyze

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Week is one calendar week of riding, aggregated. Weeks rather than
// individual rides because a single ride says nothing about progress: one
// tailwind evening beats a good week of training on average speed. Over a
// week the conditions average out enough for a direction to show.
type Week struct {
	Start               time.Time `json:"start"`
	Rides               int       `json:"rides"`
	DistanceMeters      float64   `json:"distance_meters"`
	MovingSeconds       int       `json:"moving_seconds"`
	ElevationGainMeters float64   `json:"elevation_gain_meters"`
	// AvgSpeedKmh is distance over moving time for the whole week, not the
	// mean of the rides' averages — a 10 km spin would otherwise weigh as much
	// as a 100 km tour.
	AvgSpeedKmh float64 `json:"avg_speed_kmh"`
}

// Progress is the weekly history plus what it means, in the same shape the
// rest of the app uses (#618).
type Progress struct {
	Weeks      []Week      `json:"weeks"`
	Statements []Statement `json:"statements"`
	// Endurance is the same question one level deeper: not "how much did I
	// ride" but "does my heart do less work for it" (#619). Its own series,
	// because it deliberately ignores most rides.
	Endurance EnduranceTrend `json:"endurance"`
	// Zones is how the recent weeks were spread across the effort bands
	// (#621) — the one place where a distribution says more than any average.
	Zones ZoneDistribution `json:"zones"`
	// LifetimeDistanceMeters is every ride ever uploaded, not bounded by
	// progressWeeks — the dashboard's "auf einen Blick" figure (#655).
	LifetimeDistanceMeters float64 `json:"lifetime_distance_meters"`
	// CurrentStreakWeeks counts consecutive calendar weeks with at least one
	// ride, back from the most recent week that has one. A week with no ride
	// yet (including the current, still-open one) simply isn't in Weeks —
	// see currentStreakWeeks for how a gap ends the streak. Capped at
	// progressWeeks by the query window; a longer streak just reports 16.
	CurrentStreakWeeks int `json:"current_streak_weeks"`
}

const (
	// progressWeeks is how much history to return. A season's worth is enough
	// to see a direction without the chart turning into a hairline.
	progressWeeks = 16
	// trendBlockWeeks compares the last four weeks against the four before
	// them. Fewer would be noise; more would react too slowly to matter.
	trendBlockWeeks = 4
)

// WeeklyProgress aggregates the rider's last weeks and derives the plain
// statements that go with them.
//
// The weeks come from the database (date_trunc does the calendar work), the
// wording from here — same split as everywhere else in this package.
func WeeklyProgress(ctx context.Context, pool *pgxpool.Pool, userID int64) (Progress, error) {
	rows, err := pool.Query(ctx, `
		SELECT date_trunc('week', started_at)::date AS week_start,
		       count(*),
		       coalesce(sum(distance_meters), 0),
		       coalesce(sum(moving_seconds), 0),
		       coalesce(sum(elevation_gain_meters), 0)
		FROM activities
		WHERE user_id = $1 AND started_at > now() - make_interval(weeks => $2)
		GROUP BY week_start
		ORDER BY week_start`,
		userID, progressWeeks,
	)
	if err != nil {
		return Progress{}, err
	}
	defer rows.Close()

	var progress Progress
	for rows.Next() {
		var w Week
		if err := rows.Scan(&w.Start, &w.Rides, &w.DistanceMeters, &w.MovingSeconds, &w.ElevationGainMeters); err != nil {
			return Progress{}, err
		}
		if w.MovingSeconds > 0 {
			w.AvgSpeedKmh = w.DistanceMeters / float64(w.MovingSeconds) * 3.6
		}
		progress.Weeks = append(progress.Weeks, w)
	}
	if err := rows.Err(); err != nil {
		return Progress{}, err
	}
	progress.CurrentStreakWeeks = currentStreakWeeks(progress.Weeks)

	if err := pool.QueryRow(ctx,
		`SELECT coalesce(sum(distance_meters), 0) FROM activities WHERE user_id = $1`,
		userID,
	).Scan(&progress.LifetimeDistanceMeters); err != nil {
		return Progress{}, err
	}

	progress.Statements = progressStatements(progress.Weeks)

	progress.Endurance, err = WeeklyEnduranceTrend(ctx, pool, userID)
	if err != nil {
		return Progress{}, err
	}

	var lthrBpm *int
	var birthYear *int
	if err := pool.QueryRow(ctx, `SELECT lthr_bpm, birth_year FROM users WHERE id = $1`, userID).
		Scan(&lthrBpm, &birthYear); err != nil {
		return Progress{}, err
	}
	observedMax, err := ObservedMax(ctx, pool, userID)
	if err != nil {
		return Progress{}, err
	}
	effectiveLthr, assumed := EffectiveLTHR(lthrBpm, observedMax, birthYear)
	progress.Zones, err = RecentZones(ctx, pool, userID, effectiveLthr, assumed)
	if err != nil {
		return Progress{}, err
	}
	return progress, nil
}

// currentStreakWeeks counts consecutive calendar weeks with at least one
// ride, walking backward from the most recent week present. Weeks are only
// in the slice at all when they had a ride (the query's GROUP BY drops
// empty ones) — so a gap shows up as a jump bigger than 7 days between
// neighbouring entries, not as a zero-ride row to skip over.
//
// Deliberately "consecutive weeks", not days: a streak defined per day fits
// someone training daily, not the 3-5 h/week hobbyist WFF is built for
// (#600) — one ride most weeks is already the realistic, motivating pattern
// to reward.
func currentStreakWeeks(weeks []Week) int {
	if len(weeks) == 0 {
		return 0
	}
	streak := 1
	for i := len(weeks) - 1; i > 0; i-- {
		if weeks[i].Start.Sub(weeks[i-1].Start) != 7*24*time.Hour {
			break
		}
		streak++
	}
	return streak
}

// progressStatements says which way things are going, in words.
//
// Split out from the query so it can be tested without a database — the
// thresholds and the honesty rules are the part worth pinning down.
func progressStatements(weeks []Week) []Statement {
	if len(weeks) < trendBlockWeeks*2 {
		return []Statement{{
			Kind: "hint_history",
			Text: fmt.Sprintf(
				"Für einen Vergleich über die Zeit brauchst du mindestens %d Wochen mit Fahrten, "+
					"bisher sind es %d. Das kommt von allein — fahr einfach weiter.",
				trendBlockWeeks*2, len(weeks)),
		}}
	}

	recent := weeks[len(weeks)-trendBlockWeeks:]
	earlier := weeks[len(weeks)-trendBlockWeeks*2 : len(weeks)-trendBlockWeeks]

	var out []Statement

	// Speed first: it is the number riders actually feel, and the reason this
	// view exists at all.
	recentSpeed, earlierSpeed := blockSpeed(recent), blockSpeed(earlier)
	if recentSpeed > 0 && earlierSpeed > 0 {
		delta := recentSpeed - earlierSpeed
		text := ""
		switch {
		case delta >= 0.5:
			text = fmt.Sprintf(
				"Du fährst inzwischen im Schnitt %s km/h schneller als vor zwei Monaten.",
				decimal(delta, 1))
		case delta <= -0.5:
			text = fmt.Sprintf(
				"Dein Schnitt liegt aktuell %s km/h unter dem von vor zwei Monaten.",
				decimal(-delta, 1))
		default:
			text = "Dein Tempo ist über die letzten Wochen etwa gleich geblieben."
		}
		// The caveat is not optional politeness: without it the app would sell
		// a run of tailwind or a flatter route as fitness.
		text += " Tempo hängt allerdings auch an Wind, Steigung und Strecke — über mehrere Wochen " +
			"gemittelt ist es ein guter Hinweis, aber kein Beweis."
		out = append(out, Statement{
			Text:   text,
			Metric: fmt.Sprintf("⌀ %s km/h zuletzt · %s km/h davor", decimal(recentSpeed, 1), decimal(earlierSpeed, 1)),
			Metrics: []Stat{
				{Value: decimal(recentSpeed, 1), Unit: "km/h", Label: "Zuletzt"},
				{Value: decimal(earlierSpeed, 1), Unit: "km/h", Label: "Davor"},
			},
			Kind: "trend",
		})
	}

	recentKm, earlierKm := blockDistanceKm(recent), blockDistanceKm(earlier)
	if earlierKm > 0 {
		switch ratio := recentKm / earlierKm; {
		case ratio >= 1.15:
			out = append(out, Statement{
				Text: "Du fährst deutlich mehr Kilometer als vor zwei Monaten — genau so wächst " +
					"Ausdauer.",
				Metric: fmt.Sprintf("%d km in 4 Wochen · davor %d km", int(recentKm+0.5), int(earlierKm+0.5)),
				Metrics: []Stat{
					{Value: fmt.Sprintf("%d", int(recentKm+0.5)), Unit: "km", Label: "Letzte 4 Wochen"},
					{Value: fmt.Sprintf("%d", int(earlierKm+0.5)), Unit: "km", Label: "Davor"},
				},
				Kind: "trend",
			})
		case ratio <= 0.85:
			out = append(out, Statement{
				Text: "Du bist zuletzt weniger unterwegs gewesen als davor. Kein Drama — aber wenn du " +
					"aufbauen willst, ist regelmäßiges Fahren der wirksamste Hebel.",
				Metric: fmt.Sprintf("%d km in 4 Wochen · davor %d km", int(recentKm+0.5), int(earlierKm+0.5)),
				Metrics: []Stat{
					{Value: fmt.Sprintf("%d", int(recentKm+0.5)), Unit: "km", Label: "Letzte 4 Wochen"},
					{Value: fmt.Sprintf("%d", int(earlierKm+0.5)), Unit: "km", Label: "Davor"},
				},
				Kind: "trend",
			})
		}
	}

	if len(out) == 0 {
		out = append(out, Statement{
			Text: "Umfang und Tempo bewegen sich seit Wochen im selben Rahmen. Für einen Sprung nach " +
				"oben braucht es entweder mehr Kilometer oder härtere Einheiten.",
			Kind: "trend",
		})
	}
	return out
}

func blockSpeed(weeks []Week) float64 {
	var meters float64
	var seconds int
	for _, w := range weeks {
		meters += w.DistanceMeters
		seconds += w.MovingSeconds
	}
	if seconds == 0 {
		return 0
	}
	return meters / float64(seconds) * 3.6
}

func blockDistanceKm(weeks []Week) float64 {
	var meters float64
	for _, w := range weeks {
		meters += w.DistanceMeters
	}
	return meters / 1000
}
