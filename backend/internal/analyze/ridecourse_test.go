package analyze

import (
	"math"
	"strings"
	"testing"
	"time"
)

var rideStart = time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)

// track builds samples along a straight line from a start point, one every 5
// seconds, optionally climbing at a fixed grade.
func track(startLat, startLon, dLat, dLon float64, n int, startAlt, altStep float64, from time.Time) []CourseSample {
	out := make([]CourseSample, 0, n)
	for i := 0; i < n; i++ {
		lat := startLat + dLat*float64(i)
		lon := startLon + dLon*float64(i)
		alt := startAlt + altStep*float64(i)
		t := from.Add(time.Duration(i*5) * time.Second)
		out = append(out, CourseSample{Time: t, Lat: &lat, Lon: &lon, AltitudeMeters: &alt})
	}
	return out
}

func wind(speed, direction float64, hours ...time.Time) []WindBucket {
	var out []WindBucket
	for _, h := range hours {
		s, d := speed, direction
		out = append(out, WindBucket{Hour: h.Truncate(time.Hour), SpeedMps: &s, DirectionDeg: &d})
	}
	return out
}

// The whole point of #606: an out-and-back into a steady wind must not average
// itself out. The hourly headwind column does exactly that; per-segment
// projection must report roughly half the distance against and half with.
func TestCourseOutAndBackDoesNotCancelWind(t *testing.T) {
	// East for ~1.1 km, then back west along the same line.
	out := track(48.1372, 11.5755, 0, 0.00015, 100, 500, 0, rideStart)
	backStart := rideStart.Add(500 * time.Second)
	back := track(48.1372, 11.5755+0.00015*99, 0, -0.00015, 100, 500, 0, backStart)

	// Wind FROM the east at 6 m/s: headwind on the way out, tailwind returning.
	stats := Course(append(out, back...), wind(6, 90, rideStart))

	if !stats.HasWind {
		t.Fatal("wind not evaluated at all")
	}
	if stats.HeadwindShare < 0.4 || stats.HeadwindShare > 0.6 {
		t.Errorf("headwind share %.2f, want ~0.5 — hourly averaging bug is back", stats.HeadwindShare)
	}
	if stats.TailwindShare < 0.4 || stats.TailwindShare > 0.6 {
		t.Errorf("tailwind share %.2f, want ~0.5", stats.TailwindShare)
	}
	if got := math.Round(stats.WindFromDeg); got != 90 {
		t.Errorf("wind direction %v, want 90", got)
	}
}

func TestCourseWindClassification(t *testing.T) {
	east := track(48.1372, 11.5755, 0, 0.00015, 60, 500, 0, rideStart)

	// Riding east into wind from the east.
	if s := Course(east, wind(6, 90, rideStart)); s.HeadwindShare < 0.99 {
		t.Errorf("pure headwind ride: share %.2f, want 1.0", s.HeadwindShare)
	}
	// Same ride, wind from the west: tailwind.
	if s := Course(east, wind(6, 270, rideStart)); s.TailwindShare < 0.99 {
		t.Errorf("pure tailwind ride: share %.2f, want 1.0", s.TailwindShare)
	}
	// Wind from the north while riding east: crosswind.
	if s := Course(east, wind(6, 0, rideStart)); s.CrosswindShare < 0.99 {
		t.Errorf("pure crosswind ride: share %.2f, want 1.0", s.CrosswindShare)
	}
	// A breeze must not be dressed up as wind at all.
	if s := Course(east, wind(0.4, 90, rideStart)); s.HasWind {
		t.Error("0.4 m/s counted as noticeable wind")
	}
	// No weather stored at all.
	if s := Course(east, nil); s.HasWind {
		t.Error("wind reported without any weather data")
	}
}

func TestCourseTerrain(t *testing.T) {
	// ~1.7 km climbing 100 m => ~6 % average grade.
	climb := track(48.1372, 11.5755, 0.00015, 0, 100, 500, 1.0, rideStart)
	stats := Course(climb, nil)

	if !stats.HasTerrain {
		t.Fatal("terrain not evaluated despite elevation data")
	}
	if stats.ElevationGainMeters < 90 || stats.ElevationGainMeters > 110 {
		t.Errorf("gain %.0f m, want ~99", stats.ElevationGainMeters)
	}
	if stats.ClimbDistanceShare < 0.9 {
		t.Errorf("climb share %.2f on a pure climb, want ~1.0", stats.ClimbDistanceShare)
	}
	if stats.SteepestGradePct < 4 || stats.SteepestGradePct > 8 {
		t.Errorf("steepest sustained grade %.1f %%, want ~6", stats.SteepestGradePct)
	}
}

// Distance and speed must come from what the device recorded, not from the GPS
// track: a wheel sensor keeps counting through tunnels and dropouts, and it is
// the figure the rider saw on the head unit.
func TestDistanceAndSpeedPreferTheRecordedValue(t *testing.T) {
	recorded := 42300.0
	f := RideFacts{
		DistanceMeters: &recorded,
		MovingSeconds:  5400,
		ElapsedSeconds: 6000,
		Course:         &CourseStats{DistanceMeters: 3000}, // GPS lost most of the ride
	}
	if got := f.distanceMeters(); got != recorded {
		t.Errorf("distance %v, want the recorded %v", got, recorded)
	}
	if got := f.avgSpeedKmh(); got < 28 || got > 28.5 {
		t.Errorf("avg speed %.2f km/h, want ~28.2 (42.3 km in 1:30 moving)", got)
	}

	// No recorded distance: fall back to the track rather than saying nothing.
	f.DistanceMeters = nil
	if got := f.distanceMeters(); got != 3000 {
		t.Errorf("fallback distance %v, want the GPS 3000", got)
	}

	// Moving time missing: elapsed is the honest second choice.
	f.MovingSeconds = 0
	if got := f.avgSpeedKmh(); got <= 0 {
		t.Errorf("no speed derived from elapsed time: %v", got)
	}
}

// A single steep step must not be sold as "the steepest ramp" of the ride.
func TestCourseSteepestGradeIgnoresSpikes(t *testing.T) {
	flat := track(48.1372, 11.5755, 0.00015, 0, 100, 500, 0, rideStart)
	// One sample jumps 8 m — a barometric glitch, not a wall.
	spiked := 508.0
	flat[50].AltitudeMeters = &spiked

	if got := Course(flat, nil).SteepestGradePct; got > 3 {
		t.Errorf("spike reported as %.1f %% sustained grade", got)
	}
}

func TestCourseWithoutGPS(t *testing.T) {
	var samples []CourseSample
	for i := 0; i < 20; i++ {
		samples = append(samples, CourseSample{Time: rideStart.Add(time.Duration(i*5) * time.Second)})
	}
	stats := Course(samples, wind(6, 90, rideStart))
	if stats.DistanceMeters != 0 || stats.HasWind || stats.HasTerrain {
		t.Errorf("statistics invented without any position data: %+v", stats)
	}
}

// The whole reason #606 exists: a bike with no power meter and no HR strap
// must still get real statements, not just the FTP hint.
func TestRideStoryWithoutPowerOrHeartRate(t *testing.T) {
	course := &CourseStats{
		DistanceMeters:      42000,
		ElevationGainMeters: 620, ClimbDistanceShare: 0.35,
		SteepestGradePct: 7.4, HasTerrain: true,
		HeadwindShare: 0.55, TailwindShare: 0.3, CrosswindShare: 0.15,
		HeadwindOnClimbShare: 0.6, MeanWindSpeedMps: 4.2, WindFromDeg: 270, HasWind: true,
	}
	story := RideStory(RideFacts{
		DistanceMeters:      f64(42000),
		ElevationGainMeters: f64(620), // 42 km / 620 m => ~14.8 hm per km
		ElapsedSeconds:      6200,
		MovingSeconds:       6000,
		Course:              course,
	})

	k := kinds(story)
	if k["pace"] == "" {
		t.Error("no pace statement despite speed and distance")
	}
	if !strings.Contains(k["pace"], "Wind, Steigung, Untergrund und Rad") {
		t.Errorf("pace statement missing its own caveat: %q", k["pace"])
	}
	if k["hint_profile"] == "" {
		t.Error("FTP hint disappeared — it should still be offered")
	}
	if !strings.Contains(story.Headline, "Hügelige Ausfahrt") {
		t.Errorf("headline %q ignores the profile", story.Headline)
	}

	var context []string
	for _, s := range story.Statements {
		if s.Kind == "context" {
			context = append(context, s.Text+" | "+s.Metric)
		}
	}
	if len(context) != 2 {
		t.Fatalf("want wind + terrain context, got %d: %v", len(context), context)
	}
	if !strings.Contains(context[0], "Anstiegen") {
		t.Errorf("headwind-on-climbs case not called out: %q", context[0])
	}
	if !strings.Contains(context[0], "aus Westen") {
		t.Errorf("wind direction not named: %q", context[0])
	}
	if !strings.Contains(context[1], "35 %") || !strings.Contains(context[1], "7,4 %") {
		t.Errorf("terrain statement lost its numbers: %q", context[1])
	}

	// More than three statements total is the acceptance criterion: several
	// real readings, not one apology.
	if len(story.Statements) < 4 {
		t.Errorf("only %d statements for a ride without power/HR: %+v", len(story.Statements), story.Statements)
	}
}

func TestPaceStatementComparesToOwnSpeeds(t *testing.T) {
	base := RideFacts{
		// 30 km in one hour of moving time = 30 km/h.
		DistanceMeters: f64(30000),
		ElapsedSeconds: 3600, MovingSeconds: 3600,
	}

	base.PriorSpeedsKmh = []float64{24, 25, 26} // median 25, ratio 1.2
	if got := kinds(RideStory(base))["pace"]; !strings.Contains(got, "schneller als") {
		t.Errorf("faster-than-usual not stated: %q", got)
	}

	base.PriorSpeedsKmh = []float64{34, 35, 36} // median 35, ratio 0.86
	if got := kinds(RideStory(base))["pace"]; !strings.Contains(got, "langsamer als") {
		t.Errorf("slower-than-usual not stated: %q", got)
	}

	base.PriorSpeedsKmh = []float64{29, 30} // too few
	got := kinds(RideStory(base))["pace"]
	if strings.Contains(got, "schneller als") || strings.Contains(got, "langsamer als") {
		t.Errorf("compared against two rides: %q", got)
	}
}
