package analyze

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ObservedMaxHR is the highest heart rate any ride has recorded, with the
// ride it came from. It is not a maximal-effort test result — just the
// hardest thing seen so far — and must be labelled as exactly that wherever
// it appears (#624).
type ObservedMaxHR struct {
	Bpm        int       `json:"bpm"`
	ActivityID int64     `json:"activity_id"`
	RiddenAt   time.Time `json:"ridden_at"`
}

const (
	// observedMaxPlausibleShare — below this share of the age-predicted
	// maximum, the rider has simply never gone hard enough for the value to
	// stand in for a real threshold test.
	observedMaxPlausibleShare = 0.80
	// observedMaxCeilingShare — above this share of the age-predicted
	// maximum, the reading is more likely a sensor artefact (an optical strap
	// glitching on a bump, a dropped connection) than a real effort. A
	// genuine outlier this far past prediction is rare; treating it as noise
	// costs far less than the alternative, where one bad reading becomes the
	// permanent "hardest ever" and quietly inflates every zone boundary and
	// every HR-based IF/TSS built on it from then on.
	observedMaxCeilingShare = 1.10
	// absoluteMaxPlausibleBpm is the fallback ceiling for a rider with no
	// birth year configured, so there's nothing to predict a maximum from —
	// plausible() otherwise accepts anything in that case. No recorded human
	// heart rate has approached this; it exists purely to catch instrument
	// noise, not to second-guess a real outlier effort.
	absoluteMaxPlausibleBpm = 230
	// assumedLTHRShareOfMax is the commonly used rule of thumb for where
	// threshold heart rate sits relative to maximum. Looser than a real
	// 20-minute test, but the only way to offer zones before one has ever
	// been ridden.
	assumedLTHRShareOfMax = 0.92
)

// ageMaxHeartRate is the Tanaka formula (208 − 0.7 × age): a closer fit
// across adult ages than the older "220 − age" rule of thumb. Only used here
// to sanity-check an observed value — never shown to the rider as a number
// of its own.
func ageMaxHeartRate(age int) float64 {
	return 208 - 0.7*float64(age)
}

// ObservedMax finds the hardest heart rate on record for a rider, or nil if
// no ride has ever recorded one.
func ObservedMax(ctx context.Context, pool *pgxpool.Pool, userID int64) (*ObservedMaxHR, error) {
	var m ObservedMaxHR
	err := pool.QueryRow(ctx, `
		SELECT max_heart_rate, id, started_at FROM activities
		WHERE user_id = $1 AND max_heart_rate IS NOT NULL
		ORDER BY max_heart_rate DESC, started_at DESC
		LIMIT 1`,
		userID,
	).Scan(&m.Bpm, &m.ActivityID, &m.RiddenAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// plausible reports whether the observed value is a believable stand-in for
// a real maximal-effort test at the age the rider had when it was recorded —
// not just the hardest ride so far, but hard enough to mean something, and
// not so far past what's physiologically likely that it's probably a sensor
// glitch instead. Without a birth year there is no age-based band to check
// against, so only the absolute ceiling applies.
func (m ObservedMaxHR) plausible(birthYear *int) bool {
	if m.Bpm > absoluteMaxPlausibleBpm {
		return false
	}
	age, ok := ageAt(m.RiddenAt, birthYear)
	if !ok {
		return true
	}
	predicted := ageMaxHeartRate(age)
	return float64(m.Bpm) >= observedMaxPlausibleShare*predicted &&
		float64(m.Bpm) <= observedMaxCeilingShare*predicted
}

// AssumedLTHR derives a stand-in threshold heart rate from the observed
// maximum, for riders who have never ridden a real threshold test. False
// when there is no observed max yet, or when it is not plausible enough to
// build zones on (#624) — offering zones off a lazy day's ceiling would call
// an easy ride "hard".
func AssumedLTHR(m *ObservedMaxHR, birthYear *int) (bpm int, ok bool) {
	if m == nil || !m.plausible(birthYear) {
		return 0, false
	}
	return int(float64(m.Bpm)*assumedLTHRShareOfMax + 0.5), true
}

// EffectiveLTHR is the threshold heart rate zones get built on: the rider's
// own configured value always wins; only in its absence does the hardest
// pulse ever recorded stand in, and only when it looks like a real effort.
// Assumed marks that second case, so a page built on it can say so.
func EffectiveLTHR(lthrBpm *int, observedMax *ObservedMaxHR, birthYear *int) (bpm *int, assumed bool) {
	if lthrBpm != nil {
		return lthrBpm, false
	}
	if v, ok := AssumedLTHR(observedMax, birthYear); ok {
		return &v, true
	}
	return nil, false
}
