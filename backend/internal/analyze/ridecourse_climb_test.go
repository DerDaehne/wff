package analyze

import (
	"math"
	"testing"
	"time"
)

// climbTrack builds a straight climb: n points, `metersPerStep` apart on the
// ground, gaining `gainPerStep` each, one sample every `secondsPerStep`.
func climbTrack(n int, lonStep, gainPerStep float64, secondsPerStep int, from time.Time) []CourseSample {
	out := make([]CourseSample, 0, n)
	for i := range n {
		lat, lon := 48.1372, 11.5755+lonStep*float64(i)
		alt := 500 + gainPerStep*float64(i)
		out = append(out, CourseSample{
			Time:           from.Add(time.Duration(i*secondsPerStep) * time.Second),
			Lat:            &lat,
			Lon:            &lon,
			AltitudeMeters: &alt,
		})
	}
	return out
}

// A real climb: ~3 km at ~8 %, ridden in 20 minutes => 720 m gain, VAM 2160.
func TestBestClimbReportsVAM(t *testing.T) {
	// 0.0004° lon ≈ 30 m at 48°N; 100 steps ≈ 3 km. 7.2 m gain per step = 24 %…
	// too steep, so use 2.4 m per 30 m = 8 %.
	samples := climbTrack(101, 0.0004, 2.4, 12, testStart)
	stats := Course(samples, nil)

	if stats.BestClimb == nil {
		t.Fatal("no climb found on a 3 km, 8 % ascent")
	}
	c := *stats.BestClimb
	if c.GradePct < 7 || c.GradePct > 9 {
		t.Errorf("grade %.1f %%, want ~8", c.GradePct)
	}
	if c.GainMeters < 220 || c.GainMeters > 250 {
		t.Errorf("gain %.0f m, want ~240", c.GainMeters)
	}
	// 240 m in 1200 s = 720 m/h.
	if c.VAM < 690 || c.VAM > 750 {
		t.Errorf("VAM %.0f m/h, want ~720", c.VAM)
	}
}

// Ferrari's approximation, and its refusal to answer on shallow ground.
func TestRelativePowerFromVAM(t *testing.T) {
	// The textbook example: 1000 m/h at 6 % ≈ 3.85 W/kg.
	c := Climb{VAM: 1000, GradePct: 6}
	wkg, ok := c.RelativePowerWkg()
	if !ok {
		t.Fatal("no estimate for a 6 % climb")
	}
	if math.Abs(wkg-3.846) > 0.01 {
		t.Errorf("W/kg = %.3f, want ~3.846", wkg)
	}

	// Same speed, steeper: more of the effort goes into lifting, so the same
	// VAM means LESS relative power.
	steeper, _ := Climb{VAM: 1000, GradePct: 10}.RelativePowerWkg()
	if steeper >= wkg {
		t.Errorf("10 %% gave %.2f W/kg, must be below the 6 %% value %.2f", steeper, wkg)
	}

	// Below 5 % air resistance dominates and the formula stops meaning
	// anything — it must decline rather than answer.
	if _, ok := (Climb{VAM: 1000, GradePct: 3}).RelativePowerWkg(); ok {
		t.Error("estimate produced for a 3 % grade, where the formula does not hold")
	}
}

func TestBestClimbIgnoresRollingTerrain(t *testing.T) {
	// A gentle rise: 3 km at 1 %, well under the climb threshold.
	samples := climbTrack(101, 0.0004, 0.3, 12, testStart)
	if c := Course(samples, nil).BestClimb; c != nil {
		t.Errorf("rolling terrain reported as a climb: %+v", *c)
	}

	// Short and steep, but only ~300 m long: a ramp, not the ride's climb.
	short := climbTrack(11, 0.0004, 2.4, 12, testStart)
	if c := Course(short, nil).BestClimb; c != nil {
		t.Errorf("300 m ramp reported as a climb: %+v", *c)
	}
}

// A stop on the climb must not drag VAM towards zero.
func TestBestClimbIgnoresStoppedTime(t *testing.T) {
	samples := climbTrack(101, 0.0004, 2.4, 12, testStart)
	// Halfway up, the rider stands still for ten minutes.
	for i := 50; i < len(samples); i++ {
		samples[i].Time = samples[i].Time.Add(10 * time.Minute)
	}

	c := Course(samples, nil).BestClimb
	if c == nil {
		t.Fatal("climb lost entirely because of a stop")
	}
	if c.VAM < 690 || c.VAM > 780 {
		t.Errorf("VAM %.0f m/h — the ten-minute stop was counted as climbing time", c.VAM)
	}
}
