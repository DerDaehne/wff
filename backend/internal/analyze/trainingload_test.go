package analyze

import (
	"testing"
	"time"
)

func TestComputeSeriesHandCalculated(t *testing.T) {
	// Chosen so the fractions come out clean by hand:
	// Day1 TSS=42 -> ctl1 = 42/42 = 1 exactly, atl1 = 42/7 = 6 exactly.
	// Day2 TSS=0  -> ctl2 = 41/42, atl2 = 36/7.
	// Day3 TSS=0  -> ctl3 = (41/42)^2 = 1681/1764, atl3 = (36/7)*(6/7) = 216/49.
	day1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	day3 := day2.AddDate(0, 0, 1)

	tssByDay := map[time.Time]float64{day1: 42}
	series := computeSeries(tssByDay, day1, day3)

	if len(series) != 3 {
		t.Fatalf("len(series) = %d, want 3", len(series))
	}

	checkDay(t, series[0], day1, 42, 1, 6, 0)
	checkDay(t, series[1], day2, 0, 41.0/42.0, 36.0/7.0, -5)
	checkDay(t, series[2], day3, 0, 1681.0/1764.0, 216.0/49.0, -175.0/42.0)
}

func checkDay(t *testing.T, got DayLoad, wantDate time.Time, wantTSS, wantCTL, wantATL, wantTSB float64) {
	t.Helper()
	if !got.Date.Equal(wantDate) {
		t.Errorf("Date = %v, want %v", got.Date, wantDate)
	}
	if !almostEqual(got.TSS, wantTSS, 1e-9) {
		t.Errorf("%v: TSS = %v, want %v", wantDate, got.TSS, wantTSS)
	}
	if !almostEqual(got.CTL, wantCTL, 1e-9) {
		t.Errorf("%v: CTL = %v, want %v", wantDate, got.CTL, wantCTL)
	}
	if !almostEqual(got.ATL, wantATL, 1e-9) {
		t.Errorf("%v: ATL = %v, want %v", wantDate, got.ATL, wantATL)
	}
	if !almostEqual(got.TSB, wantTSB, 1e-9) {
		t.Errorf("%v: TSB = %v, want %v", wantDate, got.TSB, wantTSB)
	}
}

func TestComputeSeriesRestDaysKeepDecaying(t *testing.T) {
	// A block of TSS, then several rest days (TSS=0, no map entry at all -
	// same as an explicit 0): CTL/ATL must keep decaying every day, not
	// freeze or skip.
	day1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lastDay := day1.AddDate(0, 0, 4) // 5 days total
	tssByDay := map[time.Time]float64{day1: 100}

	series := computeSeries(tssByDay, day1, lastDay)
	if len(series) != 5 {
		t.Fatalf("len(series) = %d, want 5", len(series))
	}
	for i := 1; i < len(series); i++ {
		if series[i].CTL >= series[i-1].CTL {
			t.Errorf("day %d: CTL = %v, want strictly less than previous day's %v (rest day must keep decaying)", i, series[i].CTL, series[i-1].CTL)
		}
		if series[i].ATL >= series[i-1].ATL {
			t.Errorf("day %d: ATL = %v, want strictly less than previous day's %v", i, series[i].ATL, series[i-1].ATL)
		}
	}
}
