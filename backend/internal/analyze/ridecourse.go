package analyze

import (
	"math"
	"time"

	"github.com/DerDaehne/wff/internal/enrich"
)

// CourseSample is one GPS/speed/elevation reading, as much of it as the device
// recorded. Course tolerates gaps: samples without a position are skipped for
// geometry, samples without elevation are skipped for grade.
type CourseSample struct {
	Time           time.Time
	Lat, Lon       *float64
	AltitudeMeters *float64
	SpeedMps       *float64
}

// WindBucket is the stored weather for one UTC hour of the ride (table
// `enrichment`). Only the raw wind vector is needed here — the headwind column
// written by internal/enrich is a per-hour projection and is deliberately NOT
// reused, see Course.
type WindBucket struct {
	Hour         time.Time
	SpeedMps     *float64
	DirectionDeg *float64
}

// CourseStats is what the route itself says about a ride, with no power meter
// and no heart-rate strap involved: how fast, how hilly, and how the wind sat
// relative to the direction actually travelled.
type CourseStats struct {
	// DistanceMeters is what the GPS track measures. It is NOT authoritative
	// for what to show the rider — the device's own `activities.distance_meters`
	// is (wheel sensor, and present even where GPS dropped out). This value
	// exists so the shares below have a denominator and as a fallback.
	DistanceMeters float64

	ElevationGainMeters float64
	ClimbDistanceShare  float64 // share of distance at >= climbGradePct
	SteepestGradePct    float64 // steepest sustained stretch, not a single spike
	HasTerrain          bool

	// BestClimb is the ride's most significant continuous ascent. Present only
	// when the ride actually contains one worth talking about.
	BestClimb *Climb

	HeadwindShare  float64 // shares of DISTANCE, not of time or of samples
	TailwindShare  float64
	CrosswindShare float64
	// HeadwindOnClimbShare is the share of *climbing* distance ridden into the
	// wind — the combination riders feel most.
	HeadwindOnClimbShare float64
	MeanWindSpeedMps     float64
	WindFromDeg          float64
	HasWind              bool
}

// Climb is one continuous ascent, described the way a rider talks about it:
// how long, how steep, and how fast they got up it.
//
// VAM (Velocità Ascensionale Media, vertical metres per hour) is the most
// useful number WFF can give a rider with no sensors at all — it needs only
// elevation and time, and it is directly comparable between rides and between
// riders.
type Climb struct {
	DistanceMeters float64
	GainMeters     float64
	GradePct       float64
	Seconds        float64
	VAM            float64 // vertical metres per hour
}

// RelativePowerWkg applies Ferrari's approximation, the standard back-of-the-
// envelope conversion from climbing speed to relative power:
//
//	W/kg ≈ VAM / (200 + 10 × grade%)
//
// It deliberately ignores bike weight, rolling resistance and air drag, which
// is why it only holds on real climbs: below about 5 % gradient air resistance
// dominates and the number becomes meaningless. Returns false there rather
// than a figure that looks precise and isn't.
func (c Climb) RelativePowerWkg() (float64, bool) {
	if c.GradePct < minGradeForPowerEstimate || c.VAM <= 0 {
		return 0, false
	}
	return c.VAM / (200 + 10*c.GradePct), true
}

const (
	// minClimbMeters / minClimbGainMeters: below this it is a rise in the road,
	// not a climb worth reporting as "your best ascent".
	minClimbMeters     = 500
	minClimbGainMeters = 30
	// minGradeForPowerEstimate — Ferrari's formula assumes gravity dominates.
	// Below ~5 % air resistance does, and the estimate stops meaning anything.
	minGradeForPowerEstimate = 5.0

	// gradeWindowMeters smooths elevation before any grade is derived. Raw
	// consecutive samples give absurd gradients (a 1 m barometric wobble over a
	// 3 m step is 30 %), so grade is only ever computed over a window.
	gradeWindowMeters = 100
	// sustainedClimbMeters is how long a stretch must be to count as "the
	// steepest ramp" rather than a momentary kick.
	sustainedClimbMeters = 200
	// climbGradePct is where "uphill" starts. Below this it's undulating road,
	// not a climb.
	climbGradePct = 1.5
	// noticeableWindMps — below this the wind direction is noise, and calling a
	// ride "into the wind" would be dishonest.
	noticeableWindMps = 1.5
	// headwindCos: a segment counts as into/with the wind when the projection
	// is at least half the wind's strength, i.e. within 60° of head-on.
	headwindCos = 0.5
)

// Course derives route statistics from raw samples plus the stored hourly wind
// vectors.
//
// The wind part is the point of this function: internal/enrich projects wind
// onto the average heading of a whole hour, so an out-and-back ride averages
// to roughly no wind at all even when it was a fight out and a push home.
// Here the same stored vector is projected onto EACH segment's own heading and
// weighted by distance, which recovers "60 % of the way into the wind".
//
// ponytail: O(n) over all samples on every request, no caching. A ride is a
// few thousand samples and this is arithmetic; add a materialised column if
// ride detail ever gets hot.
func Course(samples []CourseSample, winds []WindBucket) CourseStats {
	var stats CourseStats

	windByHour := map[time.Time]WindBucket{}
	var windSpeedSum, windDirSinSum, windDirCosSum float64
	for _, w := range winds {
		windByHour[w.Hour.UTC().Truncate(time.Hour)] = w
		if w.SpeedMps != nil && w.DirectionDeg != nil {
			windSpeedSum += *w.SpeedMps
			// Directions are angles: averaging 350° and 10° arithmetically
			// gives 180°, the exact opposite. Average the unit vectors.
			rad := *w.DirectionDeg * math.Pi / 180
			windDirSinSum += math.Sin(rad)
			windDirCosSum += math.Cos(rad)
		}
	}

	segments := buildSegments(samples)
	if len(segments) == 0 {
		return stats
	}

	var headwindDistance, tailwindDistance, crosswindDistance float64
	var climbDistance, headwindClimbDistance float64
	for i := range segments {
		s := &segments[i]
		stats.DistanceMeters += s.meters

		wind, ok := windByHour[s.time.UTC().Truncate(time.Hour)]
		if ok && wind.SpeedMps != nil && *wind.SpeedMps >= noticeableWindMps {
			bearing := s.bearing
			component := enrich.HeadwindComponent(&bearing, wind.DirectionDeg, wind.SpeedMps)
			if component != nil {
				stats.HasWind = true
				switch cos := *component / *wind.SpeedMps; {
				case cos >= headwindCos:
					headwindDistance += s.meters
					s.intoWind = true
				case cos <= -headwindCos:
					tailwindDistance += s.meters
				default:
					crosswindDistance += s.meters
				}
			}
		}
	}

	if stats.DistanceMeters > 0 {
		if stats.HasWind {
			stats.HeadwindShare = headwindDistance / stats.DistanceMeters
			stats.TailwindShare = tailwindDistance / stats.DistanceMeters
			stats.CrosswindShare = crosswindDistance / stats.DistanceMeters
			if n := len(winds); n > 0 {
				stats.MeanWindSpeedMps = windSpeedSum / float64(n)
				stats.WindFromDeg = math.Mod(math.Atan2(windDirSinSum, windDirCosSum)*180/math.Pi+360, 360)
			}
		}
	}

	// Terrain needs elevation, which plenty of devices don't record.
	windows := buildGradeWindows(segments)
	if len(windows) > 0 {
		stats.HasTerrain = true
		for _, w := range windows {
			if w.gainMeters > 0 {
				stats.ElevationGainMeters += w.gainMeters
			}
			if w.gradePct >= climbGradePct {
				climbDistance += w.meters
				headwindClimbDistance += w.intoWindMeters
			}
		}
		if stats.DistanceMeters > 0 {
			stats.ClimbDistanceShare = climbDistance / stats.DistanceMeters
		}
		if climbDistance > 0 {
			stats.HeadwindOnClimbShare = headwindClimbDistance / climbDistance
		}
		stats.SteepestGradePct = steepestSustainedGrade(windows)
		stats.BestClimb = bestClimb(windows)
	}

	return stats
}

type segment struct {
	time     time.Time
	seconds  float64
	meters   float64
	bearing  float64
	altFrom  *float64
	altTo    *float64
	intoWind bool
}

func buildSegments(samples []CourseSample) []segment {
	var segments []segment
	var prev *CourseSample
	for i := range samples {
		s := samples[i]
		if s.Lat == nil || s.Lon == nil {
			continue
		}
		if prev != nil {
			meters := haversineMeters(*prev.Lat, *prev.Lon, *s.Lat, *s.Lon)
			// Sub-metre steps are GPS jitter, not travel: their bearing is
			// random and would smear the wind shares.
			if meters >= 1 {
				seconds := s.Time.Sub(prev.Time).Seconds()
				// A gap of minutes is a stop, not riding time — counting it
				// would drag a climb's VAM down to nothing.
				if seconds < 0 || seconds > 60 {
					seconds = 0
				}
				segments = append(segments, segment{
					time:    prev.Time,
					seconds: seconds,
					meters:  meters,
					bearing: enrich.BearingDeg(*prev.Lat, *prev.Lon, *s.Lat, *s.Lon),
					altFrom: prev.AltitudeMeters,
					altTo:   s.AltitudeMeters,
				})
			}
		}
		sample := s
		prev = &sample
	}
	return segments
}

type gradeWindow struct {
	meters         float64
	seconds        float64
	gainMeters     float64
	gradePct       float64
	intoWindMeters float64
}

// buildGradeWindows accumulates segments into ~gradeWindowMeters chunks and
// derives one grade per chunk. Returns nothing when no elevation was recorded.
func buildGradeWindows(segments []segment) []gradeWindow {
	var windows []gradeWindow
	var open gradeWindow
	var start *float64
	var end *float64

	flush := func() {
		if open.meters <= 0 || start == nil || end == nil {
			open, start, end = gradeWindow{}, nil, nil
			return
		}
		open.gainMeters = *end - *start
		open.gradePct = open.gainMeters / open.meters * 100
		windows = append(windows, open)
		open, start, end = gradeWindow{}, nil, nil
	}

	for _, s := range segments {
		if s.altFrom == nil || s.altTo == nil {
			continue
		}
		if start == nil {
			start = s.altFrom
		}
		end = s.altTo
		open.meters += s.meters
		open.seconds += s.seconds
		if s.intoWind {
			open.intoWindMeters += s.meters
		}
		if open.meters >= gradeWindowMeters {
			flush()
		}
	}
	flush()
	return windows
}

// bestClimb picks the ride's most significant continuous ascent: consecutive
// uphill windows, scored by height gained rather than by length or steepness
// alone — that is what a rider means by "the big climb" of a ride.
//
// A single flat window is allowed to sit inside a climb (a short flat bit or a
// hairpin doesn't end it), but two in a row do.
func bestClimb(windows []gradeWindow) *Climb {
	var best *Climb
	var run gradeWindow
	flatWindows := 0

	closeRun := func() {
		if run.meters >= minClimbMeters && run.gainMeters >= minClimbGainMeters && run.seconds > 0 {
			climb := Climb{
				DistanceMeters: run.meters,
				GainMeters:     run.gainMeters,
				GradePct:       run.gainMeters / run.meters * 100,
				Seconds:        run.seconds,
				VAM:            run.gainMeters / (run.seconds / 3600),
			}
			if best == nil || climb.GainMeters > best.GainMeters {
				best = &climb
			}
		}
		run, flatWindows = gradeWindow{}, 0
	}

	for _, w := range windows {
		if w.gradePct >= climbGradePct {
			run.meters += w.meters
			run.seconds += w.seconds
			run.gainMeters += w.gainMeters
			flatWindows = 0
			continue
		}
		if run.meters > 0 && w.gradePct > -1 && flatWindows == 0 {
			// One level stretch inside the climb: carry it along.
			run.meters += w.meters
			run.seconds += w.seconds
			run.gainMeters += w.gainMeters
			flatWindows++
			continue
		}
		closeRun()
	}
	closeRun()
	return best
}

// steepestSustainedGrade finds the steepest stretch of at least
// sustainedClimbMeters, so a single steep driveway doesn't get reported as the
// ride's hardest climb.
func steepestSustainedGrade(windows []gradeWindow) float64 {
	steepest := 0.0
	for i := range windows {
		var meters, gain float64
		for j := i; j < len(windows); j++ {
			meters += windows[j].meters
			gain += windows[j].gainMeters
			if meters >= sustainedClimbMeters {
				if grade := gain / meters * 100; grade > steepest {
					steepest = grade
				}
				break
			}
		}
	}
	return steepest
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	Δφ := φ2 - φ1
	Δλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return 2 * earthRadius * math.Asin(math.Min(1, math.Sqrt(a)))
}

// compassName turns a meteorological wind direction into the words a rider
// would use ("aus Westen").
func compassName(deg float64) string {
	names := []string{
		"Norden", "Nordosten", "Osten", "Südosten",
		"Süden", "Südwesten", "Westen", "Nordwesten",
	}
	return names[int(math.Mod(deg+22.5+360, 360)/45)]
}
