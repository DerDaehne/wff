package analyze

import (
	"strings"
	"testing"
)

// steadyRide builds a ride at a constant speed where heart rate drifts from
// hrStart to hrEnd — the drift is what decoupling measures.
func steadyRide(minutes int, speedMps, hrStart, hrEnd float64) []EffortSample {
	samples := make([]EffortSample, minutes)
	for i := range samples {
		hr := hrStart + (hrEnd-hrStart)*float64(i)/float64(minutes-1)
		speed, beat := speedMps, hr
		samples[i] = EffortSample{Seconds: 60, SpeedMps: &speed, HeartRateBpm: &beat}
	}
	return samples
}

func TestEfficiencyDetectsSteadyRide(t *testing.T) {
	// Two hours at constant speed and constant heart rate: nothing decoupled.
	e, ok := EfficiencyOf(steadyRide(120, 8, 140, 140), f64(0.65), 1.02)
	if !ok {
		t.Fatal("a steady two-hour aerobic ride should be evaluated")
	}
	if e.DecouplingPct > 0.5 {
		t.Errorf("decoupling %.2f %% on a flat heart rate, want ~0", e.DecouplingPct)
	}
	if e.FromPower {
		t.Error("speed-based ride reported as power-based")
	}
	if !strings.Contains(efficiencyStatement(e).Text, "gleichmäßig durchgezogen") {
		t.Errorf("statement did not credit a steady ride: %q", efficiencyStatement(e).Text)
	}
}

func TestEfficiencyDetectsDrift(t *testing.T) {
	// 140 -> 160 over two hours means half-means of ~145 and ~155, i.e. ~6.5 %
	// decoupling: real drift, but the mild kind. The bands have to tell that
	// apart from a ride that actually fell apart.
	mild, ok := EfficiencyOf(steadyRide(120, 8, 140, 160), f64(0.65), 1.02)
	if !ok {
		t.Fatal("ride should be evaluated")
	}
	if mild.DecouplingPct < 5 || mild.DecouplingPct > 10 {
		t.Errorf("decoupling %.1f %%, want ~6.5 for a 20 bpm drift", mild.DecouplingPct)
	}
	if !strings.Contains(efficiencyStatement(mild).Text, "etwas mehr arbeiten") {
		t.Errorf("mild drift got the wrong band: %q", efficiencyStatement(mild).Text)
	}

	// 140 -> 175 pushes it past 10 %: the endurance gave out.
	severe, ok := EfficiencyOf(steadyRide(120, 8, 140, 175), f64(0.65), 1.02)
	if !ok {
		t.Fatal("ride should be evaluated")
	}
	if severe.DecouplingPct <= 10 {
		t.Errorf("decoupling %.1f %%, want above 10 for a 35 bpm drift", severe.DecouplingPct)
	}
	if !strings.Contains(efficiencyStatement(severe).Text, "nach oben gewandert") {
		t.Errorf("severe drift not called out: %q", efficiencyStatement(severe).Text)
	}
}

// The refusals are the point of this feature: outside a steady aerobic ride
// both numbers describe nothing, and a decoupling figure from an interval
// session would be read as a fitness verdict.
func TestEfficiencyRefusesWhereItMeansNothing(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		if _, ok := EfficiencyOf(steadyRide(20, 8, 140, 150), f64(0.65), 1.02); ok {
			t.Error("evaluated a 20-minute ride")
		}
	})

	t.Run("not aerobic", func(t *testing.T) {
		if _, ok := EfficiencyOf(steadyRide(120, 8, 140, 150), f64(0.92), 1.02); ok {
			t.Error("evaluated a threshold effort as an aerobic ride")
		}
	})

	t.Run("not steady", func(t *testing.T) {
		if _, ok := EfficiencyOf(steadyRide(120, 8, 140, 150), f64(0.65), 1.25); ok {
			t.Error("evaluated an interval session, where the two halves are different workouts")
		}
	})

	t.Run("no heart rate", func(t *testing.T) {
		samples := steadyRide(120, 8, 140, 150)
		for i := range samples {
			samples[i].HeartRateBpm = nil
		}
		if _, ok := EfficiencyOf(samples, f64(0.65), 1.02); ok {
			t.Error("produced a heart-rate ratio without heart rate")
		}
	})
}

// Halves must be split by elapsed time, not by sample count: a ride recorded
// densely at the start would otherwise compare 40 minutes against 80.
func TestEfficiencySplitsByTimeNotSampleCount(t *testing.T) {
	speed, hr := 8.0, 140.0
	var samples []EffortSample
	// 30 minutes recorded every 10 s, then 30 minutes recorded every 60 s.
	for range 180 {
		s, h := speed, hr
		samples = append(samples, EffortSample{Seconds: 10, SpeedMps: &s, HeartRateBpm: &h})
	}
	for range 30 {
		s, h := speed, hr
		samples = append(samples, EffortSample{Seconds: 60, SpeedMps: &s, HeartRateBpm: &h})
	}

	first, second := splitAt(samples, 1800)
	var firstSeconds, secondSeconds float64
	for _, s := range first {
		firstSeconds += s.Seconds
	}
	for _, s := range second {
		secondSeconds += s.Seconds
	}
	if firstSeconds < 1790 || firstSeconds > 1810 {
		t.Errorf("first half %.0f s, want ~1800 — split by sample index instead of time?", firstSeconds)
	}
	if secondSeconds < 1790 || secondSeconds > 1810 {
		t.Errorf("second half %.0f s, want ~1800", secondSeconds)
	}
}

func TestEfficiencyPrefersPowerOverSpeed(t *testing.T) {
	samples := steadyRide(120, 8, 140, 140)
	for i := range samples {
		watts := 180.0
		samples[i].PowerWatts = &watts
	}
	e, ok := EfficiencyOf(samples, f64(0.65), 1.02)
	if !ok {
		t.Fatal("ride should be evaluated")
	}
	if !e.FromPower {
		t.Error("power data present but the speed variant was used")
	}
	// 180 W / 140 bpm ≈ 1.29
	if e.Factor < 1.2 || e.Factor > 1.4 {
		t.Errorf("factor %.2f, want ~1.29 (180 W / 140 bpm)", e.Factor)
	}
	if !strings.Contains(efficiencyStatement(e).Metric, "Leistung und Puls") {
		t.Errorf("metric should name its source: %q", efficiencyStatement(e).Metric)
	}
}
