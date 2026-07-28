package analyze

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// YearReview is a season's rides summed up, plus the two rides worth calling
// out by name (#638) — a pure summary of figures already stored on
// `activities`, nothing newly derived. "Biggest progress" from the weekly
// endurance trend (#619) was in the original idea but is left out: that
// would mean deriving a new figure rather than summarizing an existing one,
// and a year-in-review is not the place to introduce one.
type YearReview struct {
	Year                int     `json:"year"`
	RideCount           int     `json:"ride_count"`
	DistanceMeters      float64 `json:"distance_meters"`
	ElevationGainMeters float64 `json:"elevation_gain_meters"`
	MovingSeconds       int     `json:"moving_seconds"`
	// HardestRide and LongestRide are nil when no ride in the year has the
	// underlying figure at all (e.g. TSS needs FTP/LTHR configured).
	HardestRide *RideHighlight `json:"hardest_ride,omitempty"`
	LongestRide *RideHighlight `json:"longest_ride,omitempty"`
}

// RideHighlight is one ride singled out, with the number that made it stand
// out — TSS for the hardest ride, kilometres for the longest.
type RideHighlight struct {
	ActivityID int64     `json:"activity_id"`
	StartedAt  time.Time `json:"started_at"`
	Value      float64   `json:"value"`
}

// YearReviewFor sums up a calendar year. A year with no rides yet — a fresh
// account, or a gap in riding — comes back with RideCount 0 and both
// highlights nil rather than an error: the honest answer to "how was your
// year" is sometimes "you didn't ride this year", not a 500.
func YearReviewFor(ctx context.Context, pool *pgxpool.Pool, userID int64, year int) (YearReview, error) {
	review := YearReview{Year: year}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)

	if err := pool.QueryRow(ctx, `
		SELECT count(*), coalesce(sum(distance_meters), 0), coalesce(sum(elevation_gain_meters), 0),
		       coalesce(sum(moving_seconds), 0)
		FROM activities
		WHERE user_id = $1 AND started_at >= $2 AND started_at < $3`,
		userID, start, end,
	).Scan(&review.RideCount, &review.DistanceMeters, &review.ElevationGainMeters, &review.MovingSeconds); err != nil {
		return YearReview{}, err
	}
	if review.RideCount == 0 {
		return review, nil
	}

	var err error
	review.HardestRide, err = rideHighlight(ctx, pool, userID, start, end, "training_stress_score")
	if err != nil {
		return YearReview{}, err
	}
	review.LongestRide, err = rideHighlight(ctx, pool, userID, start, end, "distance_meters")
	if err != nil {
		return YearReview{}, err
	}
	return review, nil
}

// rideHighlight picks the ride with the highest value in one column within
// the year. column is always a fixed internal string (training_stress_score
// or distance_meters), never user input.
func rideHighlight(ctx context.Context, pool *pgxpool.Pool, userID int64, start, end time.Time, column string) (*RideHighlight, error) {
	var h RideHighlight
	err := pool.QueryRow(ctx, `
		SELECT id, started_at, `+column+`
		FROM activities
		WHERE user_id = $1 AND started_at >= $2 AND started_at < $3 AND `+column+` IS NOT NULL
		ORDER BY `+column+` DESC LIMIT 1`,
		userID, start, end,
	).Scan(&h.ActivityID, &h.StartedAt, &h.Value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &h, nil
}
