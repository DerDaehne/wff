package analyze

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The endurance trend answers "is my aerobic base getting better?" over weeks
// (#619), where a single ride's efficiency (efficiency.go) only answers it for
// that ride.
//
// The hard part is not the arithmetic, it is the selection. Efficiency rises on
// its own when a ride is shorter, flatter, or had a tailwind — a chart that
// simply lines up every ride would mostly show route differences while reading
// like a fitness curve. That is exactly the kind of misleading number #600 set
// out to remove. So most of this file decides which rides may be compared with
// each other at all, and refuses out loud when too few survive.

// ComparableRide is one ride reduced to what the comparison needs. The values
// all come from the activities row — no sample scan, because a trend over weeks
// does not need per-second resolution, and the small difference against the
// sample-weighted figure is the same for every ride in the series.
type ComparableRide struct {
	Start               time.Time
	MovingSeconds       int
	DistanceMeters      float64
	ElevationGainMeters float64
	AvgHeartRate        float64
	AvgPower            *float64
	NormalizedPower     *float64
	IntensityFactor     *float64
}

// EnduranceWeek is one week's efficiency, expressed as something a rider can
// picture: how much speed (or power) came out per 100 heartbeats.
type EnduranceWeek struct {
	Start time.Time `json:"start"`
	Rides int       `json:"rides"`
	Value float64   `json:"value"`
}

// EnduranceTrend is the series plus what it means. Unit travels with the data
// because the scale differs between the power and the speed variant, and the
// two must never be drawn on the same axis.
type EnduranceTrend struct {
	Weeks      []EnduranceWeek `json:"weeks"`
	FromPower  bool            `json:"from_power"`
	Unit       string          `json:"unit"`
	Statements []Statement     `json:"statements"`
}

const (
	// comparableBand is how far a ride may sit from the median before it stops
	// being comparable — half as long to twice as long. Relative to the rider's
	// own median rather than an absolute window, because "a normal ride" is 45
	// minutes for one person and three hours for another.
	comparableBand = 2.0
	// enduranceBlockDays is the window compared against the one before it,
	// matching the four-week blocks the weekly view already uses.
	enduranceBlockDays = 28
	// enduranceMinRidesPerBlock — under three comparable rides per block a
	// single good day decides the verdict.
	enduranceMinRidesPerBlock = 3
	// enduranceMeaningfulPct is where a change stops being noise. Efficiency
	// figures move a percent or two between rides on their own; a few percent
	// across four weeks is the smallest change worth calling a direction.
	enduranceMeaningfulPct = 3.0
	// beatsReference expresses the ratio per 100 heartbeats instead of per one
	// — same trend, but a number with a size a person can picture.
	beatsReference = 100
)

// WeeklyEnduranceTrend loads the rider's recent rides and derives the trend.
//
// Everything it needs is already on the activities row (average heart rate,
// normalised power, distance, moving time) — the per-second sample scan that
// efficiency.go does for a single ride would multiply by every ride here for a
// resolution a weekly chart cannot show anyway.
func WeeklyEnduranceTrend(ctx context.Context, pool *pgxpool.Pool, userID int64) (EnduranceTrend, error) {
	rows, err := pool.Query(ctx, `
		SELECT started_at, moving_seconds, coalesce(distance_meters, 0),
		       coalesce(elevation_gain_meters, 0), coalesce(avg_heart_rate, 0),
		       avg_power_watts, normalized_power_watts, intensity_factor
		FROM activities
		WHERE user_id = $1 AND started_at > now() - make_interval(weeks => $2)
		ORDER BY started_at`,
		userID, progressWeeks,
	)
	if err != nil {
		return EnduranceTrend{}, err
	}
	defer rows.Close()

	var rides []ComparableRide
	for rows.Next() {
		var r ComparableRide
		if err := rows.Scan(&r.Start, &r.MovingSeconds, &r.DistanceMeters, &r.ElevationGainMeters,
			&r.AvgHeartRate, &r.AvgPower, &r.NormalizedPower, &r.IntensityFactor); err != nil {
			return EnduranceTrend{}, err
		}
		rides = append(rides, r)
	}
	if err := rows.Err(); err != nil {
		return EnduranceTrend{}, err
	}

	return EnduranceTrendOf(rides, time.Now()), nil
}

// EnduranceTrendOf selects the comparable rides, aggregates them by week and
// says which way things are going. `now` is a parameter so the honesty rules
// can be tested without waiting for a calendar.
func EnduranceTrendOf(rides []ComparableRide, now time.Time) EnduranceTrend {
	steady := filterSteadyAerobic(rides)

	// Power and speed efficiency are different quantities in different units —
	// mixing them into one series would produce a step the moment a power meter
	// is fitted or forgotten. Whichever there are more of wins; the rest drop.
	withPower := 0
	for _, r := range steady {
		if r.NormalizedPower != nil || r.AvgPower != nil {
			withPower++
		}
	}
	fromPower := withPower*2 >= len(steady) && withPower > 0

	var sameSource []ComparableRide
	for _, r := range steady {
		if hasPower(r) == fromPower {
			sameSource = append(sameSource, r)
		}
	}

	comparable := filterComparableShape(sameSource, fromPower)

	trend := EnduranceTrend{FromPower: fromPower, Unit: "km/h je 100 Schläge"}
	if fromPower {
		trend.Unit = "Watt je 100 Schläge"
	}
	trend.Weeks = weeklyEfficiency(comparable, fromPower)
	trend.Statements = enduranceStatements(comparable, fromPower, now)
	return trend
}

func hasPower(r ComparableRide) bool {
	return r.NormalizedPower != nil || r.AvgPower != nil
}

// filterSteadyAerobic applies the same rules a single ride has to pass in
// efficiency.go: long enough for the aerobic system to settle, below threshold,
// and not an interval session.
func filterSteadyAerobic(rides []ComparableRide) []ComparableRide {
	var out []ComparableRide
	for _, r := range rides {
		if r.AvgHeartRate <= 0 || r.MovingSeconds < efficiencyMinSeconds {
			continue
		}
		if r.IntensityFactor != nil && *r.IntensityFactor > aerobicMaxIF(hasPower(r)) {
			continue
		}
		if r.NormalizedPower != nil && r.AvgPower != nil && *r.AvgPower > 0 &&
			*r.NormalizedPower / *r.AvgPower > efficiencyMaxVariability {
			continue
		}
		if efficiencyOf(r, hasPower(r)) <= 0 {
			continue
		}
		out = append(out, r)
	}
	return out
}

// filterComparableShape drops rides whose shape differs too much from the
// rider's normal one.
//
// Duration always matters: heart rate drifts upward over hours, so a two-hour
// ride and a forty-minute ride produce different efficiency figures at
// identical fitness. Climbing only matters without a power meter — watts are
// watts uphill, but speed per heartbeat collapses on a climb.
func filterComparableShape(rides []ComparableRide, fromPower bool) []ComparableRide {
	if len(rides) == 0 {
		return nil
	}

	durations := make([]float64, 0, len(rides))
	climbRates := make([]float64, 0, len(rides))
	for _, r := range rides {
		durations = append(durations, float64(r.MovingSeconds))
		climbRates = append(climbRates, climbRateOf(r))
	}
	medianDuration := medianOf(durations)
	medianClimb := medianOf(climbRates)

	var out []ComparableRide
	for _, r := range rides {
		if !withinBand(float64(r.MovingSeconds), medianDuration) {
			continue
		}
		// A flat median makes the ratio meaningless (everything is "twice as
		// hilly as nothing"), so the profile check only applies once there is
		// enough climbing for it to describe anything.
		if !fromPower && medianClimb > 1 && !withinBand(climbRateOf(r), medianClimb) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// climbRateOf is metres of climbing per kilometre — the shape of the route,
// independent of how far it went.
func climbRateOf(r ComparableRide) float64 {
	if r.DistanceMeters <= 0 {
		return 0
	}
	return r.ElevationGainMeters / (r.DistanceMeters / 1000)
}

func withinBand(value, median float64) bool {
	if median <= 0 {
		return true
	}
	return value >= median/comparableBand && value <= median*comparableBand
}

// efficiencyOf is output per 100 heartbeats: km/h with speed, watts with power.
func efficiencyOf(r ComparableRide, fromPower bool) float64 {
	if r.AvgHeartRate <= 0 {
		return 0
	}
	var output float64
	if fromPower {
		switch {
		case r.NormalizedPower != nil:
			output = *r.NormalizedPower
		case r.AvgPower != nil:
			output = *r.AvgPower
		}
	} else if r.MovingSeconds > 0 {
		output = r.DistanceMeters / float64(r.MovingSeconds) * 3.6
	}
	if output <= 0 {
		return 0
	}
	return output / r.AvgHeartRate * beatsReference
}

// weeklyEfficiency averages by riding time, not by ride count — a two-hour ride
// says more about endurance than a half-hour one.
func weeklyEfficiency(rides []ComparableRide, fromPower bool) []EnduranceWeek {
	type bucket struct {
		valueSum, secondsSum float64
		rides                int
	}
	buckets := map[time.Time]*bucket{}
	for _, r := range rides {
		start := weekStart(r.Start)
		b := buckets[start]
		if b == nil {
			b = &bucket{}
			buckets[start] = b
		}
		seconds := float64(r.MovingSeconds)
		b.valueSum += efficiencyOf(r, fromPower) * seconds
		b.secondsSum += seconds
		b.rides++
	}

	out := make([]EnduranceWeek, 0, len(buckets))
	for start, b := range buckets {
		if b.secondsSum == 0 {
			continue
		}
		out = append(out, EnduranceWeek{Start: start, Rides: b.rides, Value: b.valueSum / b.secondsSum})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// weekStart snaps to Monday, matching date_trunc('week', …) in the weekly view
// so both charts break the year at the same places.
func weekStart(t time.Time) time.Time {
	t = t.UTC()
	offset := (int(t.Weekday()) + 6) % 7 // Monday = 0
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -offset)
}

// enduranceStatements compares the last four weeks against the four before
// them, or explains why it won't.
func enduranceStatements(rides []ComparableRide, fromPower bool, now time.Time) []Statement {
	recentFrom := now.AddDate(0, 0, -enduranceBlockDays)
	earlierFrom := now.AddDate(0, 0, -enduranceBlockDays*2)

	var recent, earlier []ComparableRide
	for _, r := range rides {
		switch {
		case r.Start.After(recentFrom):
			recent = append(recent, r)
		case r.Start.After(earlierFrom):
			earlier = append(earlier, r)
		}
	}

	if len(recent) < enduranceMinRidesPerBlock || len(earlier) < enduranceMinRidesPerBlock {
		return []Statement{{
			Kind: "hint_history",
			Text: fmt.Sprintf(
				"Für einen Ausdauer-Vergleich zählen nur Fahrten, die untereinander vergleichbar sind: "+
					"ruhig gefahren, mindestens eine halbe Stunde, mit Puls aufgezeichnet und von "+
					"ähnlicher Länge. Davon liegen aus den letzten vier Wochen %d vor und aus den vier "+
					"davor %d — nötig sind je %d. Mehr ruhige Grundlagenfahrten in ähnlichem Umfang "+
					"füllen das von allein.",
				len(recent), len(earlier), enduranceMinRidesPerBlock),
		}}
	}

	recentValue := timeWeightedEfficiency(recent, fromPower)
	earlierValue := timeWeightedEfficiency(earlier, fromPower)
	if recentValue <= 0 || earlierValue <= 0 {
		return nil
	}
	changePct := (recentValue - earlierValue) / earlierValue * 100

	output := "Tempo"
	if fromPower {
		output = "Leistung"
	}

	var text string
	switch {
	case changePct >= enduranceMeaningfulPct:
		text = fmt.Sprintf(
			"Dein Herz leistet für dasselbe %s weniger Arbeit als vor zwei Monaten — bei gleichem Puls "+
				"kommen jetzt rund %s %% mehr heraus. Das ist genau das, was Grundlagentraining bewirkt, "+
				"und es zeigt sich nur an vergleichbaren Fahrten.",
			output, decimal(changePct, 0))
	case changePct <= -enduranceMeaningfulPct:
		text = fmt.Sprintf(
			"Bei gleichem Puls kommt zurzeit rund %s %% weniger %s heraus als vor zwei Monaten. Das "+
				"passiert nach einer Pause, in einer harten Trainingsphase oder wenn du müde fährst — "+
				"und es geht mit ruhigen, regelmäßigen Fahrten wieder zurück.",
			decimal(-changePct, 0), output)
	default:
		text = fmt.Sprintf(
			"Dein Verhältnis von %s zu Puls ist seit zwei Monaten praktisch unverändert. Für einen "+
				"Sprung braucht es entweder mehr ruhige Kilometer oder gezielt härtere Einheiten.",
			output)
	}

	unit := "km/h"
	if fromPower {
		unit = "W"
	}
	return []Statement{{
		Text: text,
		Metric: fmt.Sprintf("%s %s je 100 Schläge · davor %s · %d vergleichbare Fahrten",
			decimal(recentValue, 1), unit, decimal(earlierValue, 1), len(recent)+len(earlier)),
		Metrics: []Stat{
			{Value: decimal(recentValue, 1), Unit: unit + "/100 Schläge", Label: "Jetzt"},
			{Value: decimal(earlierValue, 1), Unit: unit + "/100 Schläge", Label: "Davor"},
		},
		// Its own kind, not "trend": this answers whether the aerobic base is
		// growing, which is a different question from whether the weekly figures
		// are — and the two sit next to each other on the dashboard.
		Kind: "endurance_trend",
	}}
}

func timeWeightedEfficiency(rides []ComparableRide, fromPower bool) float64 {
	var valueSum, secondsSum float64
	for _, r := range rides {
		seconds := float64(r.MovingSeconds)
		valueSum += efficiencyOf(r, fromPower) * seconds
		secondsSum += seconds
	}
	if secondsSum == 0 {
		return 0
	}
	return valueSum / secondsSum
}
