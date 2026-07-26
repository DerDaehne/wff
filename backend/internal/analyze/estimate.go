package analyze

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The 20-minute test is the established way to arrive at FTP without a lab:
// ride 20 minutes as hard as you can hold, take 95 % of the average power.
// The same window's average heart rate stands in for LTHR.
//
// The point here is that the effort does NOT have to be a deliberate test.
// Any ride containing a hard, sustained 20 minutes carries the same
// information, which is how intervals.icu derives its "Auto FTP" — and it is
// the only way a rider who has never heard of FTP will ever get one.
const (
	estimateWindow    = 20 * time.Minute
	ftpFromTwentyMin  = 0.95
	lthrFromTwentyMin = 0.95
	// estimateHistoryDays bounds how far back to look. Beyond a season the
	// number says more about who you used to be than who you are.
	estimateHistoryDays = 90
)

// Estimate is a value derived from real rides, together with where it came
// from. The provenance is not decoration: a suggested FTP that the rider
// cannot trace back to a ride is indistinguishable from a made-up number.
type Estimate struct {
	// Value is already the derived threshold (95 % applied), not the raw
	// 20-minute average.
	Value      int       `json:"value"`
	Best20Min  int       `json:"best_20min"`
	ActivityID int64     `json:"activity_id"`
	RiddenAt   time.Time `json:"ridden_at"`
}

// Estimates holds what could be derived for one rider. A nil field means the
// history does not support that estimate — never a zero value.
type Estimates struct {
	FTPWatts *Estimate `json:"ftp_watts"`
	LTHRBpm  *Estimate `json:"lthr_bpm"`
}

// EstimateThresholds scans a rider's recent rides for the best sustained
// 20-minute effort and derives FTP and LTHR from it.
//
// ponytail: re-scans the raw samples of up to 90 days of rides per call. That
// is seconds of arithmetic for a hobby rider's volume, and it keeps the
// estimate honest when older rides are edited or deleted. Cache it in a column
// if the profile page ever gets slow.
func EstimateThresholds(ctx context.Context, pool *pgxpool.Pool, userID int64) (Estimates, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, started_at FROM activities
		WHERE user_id = $1 AND started_at > now() - make_interval(days => $2)
		ORDER BY started_at DESC`,
		userID, estimateHistoryDays,
	)
	if err != nil {
		return Estimates{}, err
	}
	type ride struct {
		id        int64
		startedAt time.Time
	}
	var rides []ride
	for rows.Next() {
		var r ride
		if err := rows.Scan(&r.id, &r.startedAt); err != nil {
			rows.Close()
			return Estimates{}, err
		}
		rides = append(rides, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Estimates{}, err
	}

	var out Estimates
	for _, r := range rides {
		powers, heartRates, times, err := loadEffortSamples(ctx, pool, r.id)
		if err != nil {
			return Estimates{}, err
		}

		if best, ok := bestWindowAverage(powers, times, estimateWindow); ok {
			if out.FTPWatts == nil || int(best+0.5) > out.FTPWatts.Best20Min {
				out.FTPWatts = &Estimate{
					Value:      int(best*ftpFromTwentyMin + 0.5),
					Best20Min:  int(best + 0.5),
					ActivityID: r.id,
					RiddenAt:   r.startedAt,
				}
			}
		}
		if best, ok := bestWindowAverage(heartRates, times, estimateWindow); ok {
			if out.LTHRBpm == nil || int(best+0.5) > out.LTHRBpm.Best20Min {
				out.LTHRBpm = &Estimate{
					Value:      int(best*lthrFromTwentyMin + 0.5),
					Best20Min:  int(best + 0.5),
					ActivityID: r.id,
					RiddenAt:   r.startedAt,
				}
			}
		}
	}
	return out, nil
}

func loadEffortSamples(ctx context.Context, pool *pgxpool.Pool, activityID int64) (powers, heartRates []*float64, times []time.Time, err error) {
	rows, err := pool.Query(ctx, `
		SELECT time, power_watts, heart_rate FROM samples
		WHERE activity_id = $1 ORDER BY time`,
		activityID,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t time.Time
		var p, hr *int
		if err := rows.Scan(&t, &p, &hr); err != nil {
			return nil, nil, nil, err
		}
		times = append(times, t)
		powers = append(powers, intPtrToFloat(p))
		heartRates = append(heartRates, intPtrToFloat(hr))
	}
	return powers, heartRates, times, rows.Err()
}

func intPtrToFloat(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

// bestWindowAverage finds the highest average over any window of at least
// `window` real time. It walks the samples rather than assuming a fixed
// sampling rate — devices record at 1 Hz, every few seconds, or "smart"
// (irregular), and a pause mid-ride leaves a gap of minutes.
//
// Weighted by the time each sample covers, so an irregularly sampled ride
// cannot inflate its average by having more readings during the hard part.
func bestWindowAverage(values []*float64, times []time.Time, window time.Duration) (float64, bool) {
	if len(values) != len(times) || len(values) < 2 {
		return 0, false
	}

	// Prefix sums of value*duration and of duration, over samples that have a
	// value. A gap where the device recorded nothing simply contributes no
	// duration, so a paused ride cannot fake a 20-minute effort.
	n := len(values)
	weighted := make([]float64, n)
	elapsed := make([]float64, n)
	for i := 1; i < n; i++ {
		seconds := times[i].Sub(times[i-1]).Seconds()
		// A gap longer than a minute is a stop, not a sample interval: it must
		// not count towards the window, otherwise standing at a traffic light
		// "extends" a 5-minute effort into 20.
		if seconds <= 0 || seconds > 60 {
			seconds = 0
		}
		weighted[i] = weighted[i-1]
		elapsed[i] = elapsed[i-1] + seconds
		if values[i] != nil {
			weighted[i] += *values[i] * seconds
		} else if seconds > 0 {
			// A sample without a reading breaks the window: averaging over a
			// stretch where the meter dropped out would overstate the effort.
			return bestWindowAverageSplit(values, times, window)
		}
	}

	wantSeconds := window.Seconds()
	best, found := 0.0, false
	start := 0
	for end := 1; end < n; end++ {
		// Shrink only while the window would STILL be long enough afterwards.
		// Shrinking until it is merely "no longer too long" overshoots: with a
		// sampling interval that doesn't divide the window evenly (18 s steps
		// against 20 minutes) every span lands just below the threshold and no
		// window is ever found. Real device data hit exactly that; 1 Hz test
		// data did not.
		for start+1 < end && elapsed[end]-elapsed[start+1] >= wantSeconds {
			start++
		}
		if span := elapsed[end] - elapsed[start]; span >= wantSeconds {
			if avg := (weighted[end] - weighted[start]) / span; !found || avg > best {
				best, found = avg, true
			}
		}
	}
	return best, found
}

// bestWindowAverageSplit handles rides where the sensor dropped out partway:
// each continuous stretch of real readings is searched on its own.
func bestWindowAverageSplit(values []*float64, times []time.Time, window time.Duration) (float64, bool) {
	best, found := 0.0, false
	start := 0
	for i := 0; i <= len(values); i++ {
		if i < len(values) && values[i] != nil {
			continue
		}
		if i-start >= 2 {
			if avg, ok := bestWindowAverage(values[start:i], times[start:i], window); ok && (!found || avg > best) {
				best, found = avg, true
			}
		}
		start = i + 1
	}
	return best, found
}
