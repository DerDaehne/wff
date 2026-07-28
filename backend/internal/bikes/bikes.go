// Package bikes tracks a rider's bikes and chain-wear reminders (#637).
// Odometer readings are never a stored running counter — they're summed from
// activities.distance_meters on read, so there is no second place a number
// can drift out of sync with the rides it's supposed to describe.
package bikes

import "time"

// Bike is one of a rider's bikes, with its computed odometer and chain
// status. DistanceKm and ChainDueKm are derived on every read, not stored.
type Bike struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Active          bool       `json:"active"`
	RetiredAt       *time.Time `json:"retired_at"`
	DistanceKm      float64    `json:"distance_km"`
	ChainIntervalKm float64    `json:"chain_interval_km"`
	// ChainDueKm is how many km remain before the chain reminder fires —
	// negative once the interval has been exceeded, so the frontend can
	// render "in 400 km" and "400 km overdue" with the same number and a
	// sign check, rather than two different fields.
	ChainDueKm float64 `json:"chain_due_km"`
}

// chainDueKm is the distance left before the configured interval is up,
// measured from the bike's own odometer reading at the last chain change —
// not from a calendar date, because wear follows distance, not time.
func chainDueKm(totalKm, intervalKm, replacedAtKm float64) float64 {
	return replacedAtKm + intervalKm - totalKm
}
