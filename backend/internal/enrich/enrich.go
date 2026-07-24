// Package enrich buckets an activity's samples by UTC hour, derives a
// headwind component per bucket from the rider's heading, fetches weather
// via internal/openmeteo, and upserts into the enrichment table. No HTTP or
// scheduling concerns here (see the async-trigger ticket).
package enrich

import (
	"context"
	"math"
	"time"

	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Result reports how many of an activity's hour-buckets got real weather
// data this run. BucketsAttempted > BucketsEnriched means some buckets are
// still waiting on ERA5 (the normal case for recent rides) — the caller
// (async-trigger poller) uses this to decide whether to retry later.
type Result struct {
	BucketsAttempted int
	BucketsEnriched  int
}

func (r Result) Complete() bool {
	return r.BucketsAttempted > 0 && r.BucketsEnriched == r.BucketsAttempted
}

// Activity enriches one activity. Safe to call repeatedly: already-enriched
// buckets are upserted with the same values, not-yet-available buckets are
// simply retried. An activity with no GPS samples at all (e.g. an indoor
// trainer ride) has zero buckets and is a no-op, not an error.
func Activity(ctx context.Context, pool *pgxpool.Pool, client *openmeteo.Client, activityID int64) (Result, error) {
	buckets, err := loadBuckets(ctx, pool, activityID)
	if err != nil {
		return Result{}, err
	}
	if len(buckets) == 0 {
		return Result{}, nil
	}

	points := make([]openmeteo.Point, len(buckets))
	for i, b := range buckets {
		points[i] = openmeteo.Point{Lat: b.lat, Lon: b.lon, HourBucket: b.hour}
	}

	weather, err := client.Fetch(ctx, points)
	if err != nil {
		return Result{}, err
	}

	result := Result{BucketsAttempted: len(buckets)}
	for i, b := range buckets {
		w := weather[i]
		if w.TemperatureCelsius == nil && w.WindSpeedMps == nil && w.WindDirectionDeg == nil && w.PrecipitationMm == nil {
			continue // not yet in the ERA5 archive - candidate for the next poll
		}
		headwind := headwindComponent(b.bearing, w.WindDirectionDeg, w.WindSpeedMps)
		if err := upsertBucket(ctx, pool, activityID, b, w, headwind); err != nil {
			return result, err
		}
		result.BucketsEnriched++
	}
	return result, nil
}

type bucket struct {
	hour     time.Time
	lat, lon float64
	bearing  *float64 // nil if fewer than 2 GPS samples in this bucket
}

func loadBuckets(ctx context.Context, pool *pgxpool.Pool, activityID int64) ([]bucket, error) {
	rows, err := pool.Query(ctx,
		`SELECT time, lat, lon FROM samples WHERE activity_id = $1 AND lat IS NOT NULL AND lon IS NOT NULL ORDER BY time`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type point struct {
		t        time.Time
		lat, lon float64
	}
	byHour := make(map[time.Time][]point)
	var order []time.Time
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.t, &p.lat, &p.lon); err != nil {
			return nil, err
		}
		hour := p.t.UTC().Truncate(time.Hour)
		if _, ok := byHour[hour]; !ok {
			order = append(order, hour)
		}
		byHour[hour] = append(byHour[hour], p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	buckets := make([]bucket, 0, len(order))
	for _, hour := range order {
		pts := byHour[hour]
		b := bucket{hour: hour, lat: pts[0].lat, lon: pts[0].lon}
		if len(pts) >= 2 {
			first, last := pts[0], pts[len(pts)-1]
			brg := bearingDeg(first.lat, first.lon, last.lat, last.lon)
			b.bearing = &brg
		}
		buckets = append(buckets, b)
	}
	return buckets, nil
}

func upsertBucket(ctx context.Context, pool *pgxpool.Pool, activityID int64, b bucket, w openmeteo.PointResult, headwind *float64) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO enrichment (
			activity_id, hour_bucket, lat, lon,
			temperature_celsius, wind_speed_mps, wind_direction_deg, precipitation_mm,
			headwind_mps, fetched_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, now())
		ON CONFLICT (activity_id, hour_bucket) DO UPDATE SET
			lat = EXCLUDED.lat,
			lon = EXCLUDED.lon,
			temperature_celsius = EXCLUDED.temperature_celsius,
			wind_speed_mps = EXCLUDED.wind_speed_mps,
			wind_direction_deg = EXCLUDED.wind_direction_deg,
			precipitation_mm = EXCLUDED.precipitation_mm,
			headwind_mps = EXCLUDED.headwind_mps,
			fetched_at = now()`,
		activityID, b.hour, b.lat, b.lon,
		w.TemperatureCelsius, w.WindSpeedMps, w.WindDirectionDeg, w.PrecipitationMm,
		headwind,
	)
	return err
}

// bearingDeg is the standard great-circle initial bearing from (lat1,lon1)
// to (lat2,lon2), in degrees clockwise from north (0-360).
func bearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	Δλ := (lon2 - lon1) * math.Pi / 180
	y := math.Sin(Δλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(Δλ)
	θ := math.Atan2(y, x) * 180 / math.Pi
	return math.Mod(θ+360, 360)
}

// headwindComponent projects wind velocity onto the rider's direction of
// travel: positive = headwind, negative = tailwind. Nil if the bearing or
// weather data isn't available.
func headwindComponent(bearingDeg, windDirectionDeg, windSpeedMps *float64) *float64 {
	if bearingDeg == nil || windDirectionDeg == nil || windSpeedMps == nil {
		return nil
	}
	relative := (*windDirectionDeg - *bearingDeg) * math.Pi / 180
	v := *windSpeedMps * math.Cos(relative)
	return &v
}
