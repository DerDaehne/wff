package analyze

import (
	"testing"
	"time"
)

// ramp builds a sample series at the given interval: `values[i]` held for
// `hold[i]` seconds each, in order. A nil value means the sensor recorded
// nothing at that point.
func series(start time.Time, intervalSeconds int, values ...*float64) ([]*float64, []time.Time) {
	times := make([]time.Time, len(values))
	for i := range values {
		times[i] = start.Add(time.Duration(i*intervalSeconds) * time.Second)
	}
	return values, times
}

func repeat(v float64, n int) []*float64 {
	out := make([]*float64, n)
	for i := range out {
		f := v
		out[i] = &f
	}
	return out
}

var testStart = time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)

func TestBestWindowAverageFindsTheHardStretch(t *testing.T) {
	// 10 min easy, 20 min hard, 10 min easy — at 1 Hz.
	var vals []*float64
	vals = append(vals, repeat(150, 600)...)
	vals = append(vals, repeat(280, 1200)...)
	vals = append(vals, repeat(150, 600)...)
	values, times := series(testStart, 1, vals...)

	got, ok := bestWindowAverage(values, times, 20*time.Minute)
	if !ok {
		t.Fatal("no 20-minute window found in a 40-minute ride")
	}
	if got < 275 || got > 281 {
		t.Errorf("best 20 min = %.1f W, want ~280", got)
	}
}

func TestBestWindowAverageNeedsTwentyMinutes(t *testing.T) {
	// A 15-minute ride cannot produce a 20-minute average, however hard it was.
	values, times := series(testStart, 1, repeat(300, 900)...)
	if _, ok := bestWindowAverage(values, times, 20*time.Minute); ok {
		t.Error("20-minute estimate produced from a 15-minute ride")
	}
}

// A ride paused at a traffic light must not have the pause count towards the
// window — otherwise five hard minutes plus a long stop become a "20-minute
// effort".
func TestBestWindowAverageIgnoresStops(t *testing.T) {
	hard := repeat(320, 300) // 5 minutes at 1 Hz
	values := append([]*float64{}, hard...)
	times := make([]time.Time, len(values))
	for i := range values {
		times[i] = testStart.Add(time.Duration(i) * time.Second)
	}
	// One sample 30 minutes later: the rider stood still in between.
	tail := repeat(320, 300)
	for i, v := range tail {
		values = append(values, v)
		times = append(times, testStart.Add(35*time.Minute+time.Duration(i)*time.Second))
	}

	if _, ok := bestWindowAverage(values, times, 20*time.Minute); ok {
		t.Error("a 30-minute stop was counted as riding time")
	}
}

// Devices record at whatever interval they like, and 20 minutes is rarely a
// whole multiple of it. Found on real data: an 18-second interval produced no
// window at all, because every candidate span landed just under 1200 s.
func TestBestWindowAverageWithUnevenSampleInterval(t *testing.T) {
	for _, interval := range []int{7, 11, 18, 23} {
		// 40 minutes of riding at a steady 250 W.
		count := (40 * 60) / interval
		values, times := series(testStart, interval, repeat(250, count)...)

		got, ok := bestWindowAverage(values, times, 20*time.Minute)
		if !ok {
			t.Errorf("%ds interval: no 20-minute window in a 40-minute ride", interval)
			continue
		}
		if got < 249 || got > 251 {
			t.Errorf("%ds interval: best 20 min = %.1f W, want ~250", interval, got)
		}
	}
}

func TestBestWindowAverageWeightsBySampleInterval(t *testing.T) {
	// Same effort, recorded every 5 seconds instead of every second: the
	// average must not change.
	values, times := series(testStart, 5, repeat(240, 300)...) // 25 minutes
	got, ok := bestWindowAverage(values, times, 20*time.Minute)
	if !ok {
		t.Fatal("no window found at a 5-second sampling rate")
	}
	if got < 239 || got > 241 {
		t.Errorf("best 20 min = %.1f W, want 240 regardless of sample rate", got)
	}
}

// A power meter that drops out mid-ride must not average across the gap.
func TestBestWindowAverageHandlesSensorDropout(t *testing.T) {
	var vals []*float64
	vals = append(vals, repeat(200, 600)...) // 10 min with data
	vals = append(vals, make([]*float64, 300)...)
	vals = append(vals, repeat(300, 1500)...) // 25 min with data
	values, times := series(testStart, 1, vals...)

	got, ok := bestWindowAverage(values, times, 20*time.Minute)
	if !ok {
		t.Fatal("no window found despite a 25-minute stretch of real data")
	}
	// Only the second stretch is long enough; it must not be diluted by the
	// gap or by the earlier easy riding.
	if got < 295 || got > 305 {
		t.Errorf("best 20 min = %.1f W, want ~300 (second stretch only)", got)
	}
}

func TestEstimateDerivation(t *testing.T) {
	// The derivation itself: 95 % of the 20-minute best.
	best20 := 280.0
	if got := int(best20*ftpFromTwentyMin + 0.5); got != 266 {
		t.Errorf("FTP from 280 W = %d, want 266", got)
	}
	bestHR := 175.0
	if got := int(bestHR*lthrFromTwentyMin + 0.5); got != 166 {
		t.Errorf("LTHR from 175 bpm = %d, want 166", got)
	}
}
