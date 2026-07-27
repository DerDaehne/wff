package analyze

import (
	"strings"
	"testing"
	"time"
)

var trendNow = time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

// trendRide is a ride that passes every selection rule: an hour, ridden calmly,
// with heart rate recorded. Tests then break exactly one property of it, so a
// failure names the rule that changed.
func trendRide(daysAgo int, distanceMeters, avgHR float64) ComparableRide {
	return ComparableRide{
		Start:               trendNow.AddDate(0, 0, -daysAgo),
		MovingSeconds:       3600,
		DistanceMeters:      distanceMeters,
		ElevationGainMeters: 300,
		AvgHeartRate:        avgHR,
	}
}

func trendWithPower(r ComparableRide, np, avg float64) ComparableRide {
	r.NormalizedPower, r.AvgPower = &np, &avg
	return r
}

func TestEfficiencyPer100Beats(t *testing.T) {
	// 30 km in an hour at 150 bpm: 30 km/h ÷ 150 × 100 = 20 km/h per 100 beats.
	got := efficiencyOf(trendRide(1, 30000, 150), false)
	if got < 19.99 || got > 20.01 {
		t.Errorf("speed efficiency = %.3f, want 20", got)
	}

	// 210 W at 140 bpm: 150 W per 100 beats.
	got = efficiencyOf(trendWithPower(trendRide(1, 30000, 140), 210, 200), true)
	if got < 149.99 || got > 150.01 {
		t.Errorf("power efficiency = %.3f, want 150", got)
	}
}

func TestSelectionDropsRidesThatWouldFakeProgress(t *testing.T) {
	base := trendRide(3, 30000, 150)

	tooShort := base
	tooShort.MovingSeconds = 1500 // 25 min — the two halves say nothing about endurance

	tooHard := base
	if_ := 0.92
	tooHard.IntensityFactor = &if_

	ragged := trendWithPower(base, 260, 200) // NP/avg 1.30: an interval session

	noPulse := base
	noPulse.AvgHeartRate = 0

	for name, ride := range map[string]ComparableRide{
		"under half an hour":  tooShort,
		"above threshold":     tooHard,
		"interval session":    ragged,
		"no heart rate":       noPulse,
		"no distance and pow": {Start: base.Start, MovingSeconds: 3600, AvgHeartRate: 150},
	} {
		if got := filterSteadyAerobic([]ComparableRide{ride}); len(got) != 0 {
			t.Errorf("%s: kept %d rides, want it dropped — it would move the trend without fitness changing", name, len(got))
		}
	}

	if got := filterSteadyAerobic([]ComparableRide{base}); len(got) != 1 {
		t.Fatalf("a calm hour with heart rate was dropped (%d kept) — nothing would ever qualify", len(got))
	}
}

func TestDurationOutliersDropOut(t *testing.T) {
	rides := []ComparableRide{
		trendRide(20, 30000, 150),
		trendRide(14, 30000, 150),
		trendRide(7, 30000, 150),
	}
	// A five-hour tour among one-hour rides: heart rate drifts over that
	// distance, so its efficiency is lower for reasons that have nothing to do
	// with fitness.
	tour := trendRide(3, 140000, 150)
	tour.MovingSeconds = 18000
	tour.ElevationGainMeters = 1400

	got := filterComparableShape(append(rides, tour), false)
	if len(got) != 3 {
		t.Fatalf("kept %d rides, want the three comparable ones without the tour", len(got))
	}
	for _, r := range got {
		if r.MovingSeconds == 18000 {
			t.Error("the five-hour tour was compared against one-hour rides")
		}
	}
}

func TestClimbingOnlyDisqualifiesWithoutPower(t *testing.T) {
	flat := []ComparableRide{
		trendRide(20, 30000, 150),
		trendRide(14, 30000, 150),
		trendRide(7, 30000, 150),
	}
	// 1200 m of climbing over 30 km: speed per heartbeat collapses on that,
	// while watts per heartbeat do not care.
	mountain := trendRide(3, 30000, 150)
	mountain.ElevationGainMeters = 1200

	if got := filterComparableShape(append(flat, mountain), false); len(got) != 3 {
		t.Errorf("without power: kept %d, want the mountain ride dropped", len(got))
	}
	if got := filterComparableShape(append(flat, mountain), true); len(got) != 4 {
		t.Errorf("with power: kept %d, want all four — watts are watts uphill", len(got))
	}
}

func TestPowerAndSpeedRidesAreNeverMixed(t *testing.T) {
	rides := []ComparableRide{
		trendWithPower(trendRide(40, 30000, 150), 200, 190),
		trendWithPower(trendRide(33, 30000, 150), 200, 190),
		trendWithPower(trendRide(26, 30000, 150), 200, 190),
		trendWithPower(trendRide(12, 30000, 150), 210, 200),
		trendWithPower(trendRide(8, 30000, 150), 210, 200),
		trendWithPower(trendRide(4, 30000, 150), 210, 200),
		trendRide(6, 30000, 150), // same rider, power meter left at home
	}

	trend := EnduranceTrendOf(rides, trendNow)
	if !trend.FromPower {
		t.Fatal("trend fell back to speed although most rides have power")
	}
	total := 0
	for _, w := range trend.Weeks {
		total += w.Rides
	}
	if total != 6 {
		t.Errorf("counted %d rides in the series, want 6 — the speed-only ride must not join a power series", total)
	}
}

func TestWeeklyValueIsWeightedByRidingTime(t *testing.T) {
	short := trendRide(3, 20000, 100) // 20 km/h, 100 bpm → 20 per 100 beats
	long := trendRide(3, 46000, 100)  // 23 km/h over two hours → 23
	long.MovingSeconds = 7200
	long.ElevationGainMeters = 600

	weeks := weeklyEfficiency([]ComparableRide{short, long}, false)
	if len(weeks) != 1 {
		t.Fatalf("got %d weeks, want both rides in one", len(weeks))
	}
	// (20×3600 + 23×7200) / 10800 = 22 — the two-hour ride counts double.
	if weeks[0].Value < 21.99 || weeks[0].Value > 22.01 {
		t.Errorf("week value = %.3f, want 22", weeks[0].Value)
	}
}

func TestTooFewComparableRidesRefusesInsteadOfGuessing(t *testing.T) {
	rides := []ComparableRide{
		trendRide(40, 30000, 150),
		trendRide(35, 30000, 150),
		trendRide(30, 30000, 150),
		trendRide(5, 30000, 150), // only one in the recent block
	}

	trend := EnduranceTrendOf(rides, trendNow)
	if len(trend.Statements) != 1 || trend.Statements[0].Kind != "hint_history" {
		t.Fatalf("statements = %+v, want a single honest refusal", trend.Statements)
	}
	if !strings.Contains(trend.Statements[0].Text, "vergleichbar") {
		t.Error("the refusal doesn't say what makes rides comparable — that's the only useful part of it")
	}
	// The series itself may still be drawn; only the verdict is withheld.
	if len(trend.Weeks) == 0 {
		t.Error("no weeks returned — the chart should still show what there is")
	}
}

func TestImprovementIsReportedOnlyWhenItExceedsNoise(t *testing.T) {
	// Same route, same heart rate, 8 % more speed in the recent block.
	var rides []ComparableRide
	for _, d := range []int{50, 45, 40, 35} {
		rides = append(rides, trendRide(d, 30000, 150))
	}
	for _, d := range []int{20, 14, 7, 2} {
		rides = append(rides, trendRide(d, 32400, 150))
	}

	trend := EnduranceTrendOf(rides, trendNow)
	if len(trend.Statements) != 1 {
		t.Fatalf("statements = %+v, want one verdict", trend.Statements)
	}
	s := trend.Statements[0]
	if s.Kind != "endurance_trend" {
		t.Errorf("kind = %q, want endurance_trend — it must not share a heading with the weekly figures", s.Kind)
	}
	if !strings.Contains(s.Text, "mehr heraus") {
		t.Errorf("text = %q, want it to report the improvement", s.Text)
	}
	if !strings.Contains(s.Metric, "vergleichbare Fahrten") {
		t.Errorf("metric = %q, want the number of rides the verdict rests on", s.Metric)
	}

	// A 1 % difference is inside the noise and must not be sold as progress.
	var flat []ComparableRide
	for _, d := range []int{50, 45, 40, 35} {
		flat = append(flat, trendRide(d, 30000, 150))
	}
	for _, d := range []int{20, 14, 7, 2} {
		flat = append(flat, trendRide(d, 30300, 150))
	}
	if s := EnduranceTrendOf(flat, trendNow).Statements[0]; !strings.Contains(s.Text, "unverändert") {
		t.Errorf("text = %q, want 1 %% treated as unchanged", s.Text)
	}
}
