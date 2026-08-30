package analyze

import (
	"strings"
	"testing"
)

func statLabels(stats []Stat) []string {
	out := make([]string, len(stats))
	for i, s := range stats {
		out[i] = s.Label
	}
	return out
}

func fullRide() RideFacts {
	return RideFacts{
		DistanceMeters:      f64(42300),
		ElevationGainMeters: f64(620),
		TSS:                 f64(101),
		ElapsedSeconds:      5400,
		MovingSeconds:       5300,
	}
}

func TestHeadlineStatsDefaultOrder(t *testing.T) {
	got := statLabels(headlineStats(fullRide()))
	want := []string{"Distanz", "Dauer", "Anstieg"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("default order %v, want %v", got, want)
	}
}

// The whole point of #616: the rider's figure goes first.
func TestHeadlineStatsHonoursPreference(t *testing.T) {
	for _, tc := range []struct{ metric, wantFirst string }{
		{MetricSpeed, "⌀ Tempo"},
		{MetricDuration, "Dauer"},
		{MetricElevation, "Anstieg"},
		{MetricLoad, "Belastung"},
		{MetricDistance, "Distanz"},
	} {
		f := fullRide()
		f.PrimaryMetric = tc.metric
		got := statLabels(headlineStats(f))
		if len(got) == 0 || got[0] != tc.wantFirst {
			t.Errorf("preference %q: got %v, want %q first", tc.metric, got, tc.wantFirst)
		}
		if len(got) != headlineStatsLimit {
			t.Errorf("preference %q: %d figures, want %d", tc.metric, len(got), headlineStatsLimit)
		}
	}
}

// A preference must never produce an empty slot: this ride has no climbing
// and no training load, so the next available figure has to move up.
func TestHeadlineStatsSkipsMissingPreferredMetric(t *testing.T) {
	f := RideFacts{
		DistanceMeters: f64(20000),
		ElapsedSeconds: 3600,
		MovingSeconds:  3600,
		PrimaryMetric:  MetricElevation,
	}
	got := statLabels(headlineStats(f))
	if len(got) == 0 {
		t.Fatal("no figures at all")
	}
	if got[0] == "Anstieg" {
		t.Error("climbing shown for a ride that recorded none")
	}
	if got[0] != "Distanz" {
		t.Errorf("got %v, want the default order to take over", got)
	}
}

func TestHeadlineStatsNoDuplicateWhenPreferenceIsAlreadyFirst(t *testing.T) {
	f := fullRide()
	f.PrimaryMetric = MetricDistance
	got := statLabels(headlineStats(f))

	seen := map[string]bool{}
	for _, label := range got {
		if seen[label] {
			t.Errorf("figure %q listed twice: %v", label, got)
		}
		seen[label] = true
	}
}

func TestHeadlineStatsIgnoresUnknownPreference(t *testing.T) {
	f := fullRide()
	f.PrimaryMetric = "wattstunden-pro-mondphase"
	got := statLabels(headlineStats(f))
	if len(got) == 0 || got[0] != "Distanz" {
		t.Errorf("unknown preference should fall back to the default order, got %v", got)
	}
}

// The ride-detail grid (Nocturne reskin, 2026-08-30) shows every figure the
// ride has, unlike headlineStats' curated 2-3 — but in a fixed order, and it
// must still omit whatever the ride doesn't have data for rather than
// showing a placeholder.
func TestDetailStatsFixedOrderWithEverythingAvailable(t *testing.T) {
	f := fullRide()
	f.AvgHeartRate = f64(142)
	f.IntensityFactor = f64(0.74)
	f.Course = &CourseStats{DistanceMeters: 42300, HeadwindShare: 0.41}
	got := statLabels(detailStats(f))
	want := []string{"Distanz", "⌀ Tempo", "⌀ Puls", "Belastung", "Anstieg", "Intensität (IF)", "Gegenwind"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("detailStats order %v, want %v", got, want)
	}
}

func TestDetailStatsOmitsWhatIsUnavailable(t *testing.T) {
	f := RideFacts{DistanceMeters: f64(20000), ElapsedSeconds: 3600, MovingSeconds: 3600}
	got := detailStats(f)
	for _, s := range got {
		if s.Label == "⌀ Puls" || s.Label == "Intensität (IF)" || s.Label == "Gegenwind" || s.Label == "Abfall 2. Hälfte" {
			t.Errorf("detailStats included %q with no source data for it", s.Label)
		}
	}
}
