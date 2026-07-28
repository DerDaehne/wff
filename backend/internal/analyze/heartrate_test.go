package analyze

import "testing"

// steadyTrace is one bpm held for a number of seconds, one point per second.
func steadyTrace(bpm float64, seconds int) []HRPoint {
	trace := make([]HRPoint, seconds)
	for i := range trace {
		trace[i] = HRPoint{Bpm: bpm, Seconds: 1}
	}
	return trace
}

func TestComputeHRMetricsHandCalculated(t *testing.T) {
	// 150 bpm at LTHR 165 for 1h: IF_hr = 150/165 = 0.90909..., TSS =
	// 1h * IF_hr^2 * 100 = 82.6446...
	metrics := ComputeHRMetrics(steadyTrace(150, 3600), 165)
	if metrics == nil {
		t.Fatal("ComputeHRMetrics = nil, want a result")
	}
	wantIF := 150.0 / 165.0
	wantTSS := wantIF * wantIF * 100
	if !almostEqual(metrics.IntensityFactor, wantIF, 1e-9) {
		t.Errorf("IF = %v, want %v", metrics.IntensityFactor, wantIF)
	}
	if !almostEqual(metrics.TSS, wantTSS, 1e-9) {
		t.Errorf("TSS = %v, want %v", metrics.TSS, wantTSS)
	}

	// Riding exactly at LTHR for 1h must equal 100 TSS, same anchor point as
	// riding exactly at FTP for 1h in the power path.
	atThreshold := ComputeHRMetrics(steadyTrace(165, 3600), 165)
	if atThreshold == nil || !almostEqual(atThreshold.TSS, 100, 1e-9) {
		t.Errorf("1h at LTHR: TSS = %v, want 100", atThreshold)
	}
}

// The whole point of #622: an interval hour and a steady hour with the same
// average heart rate must no longer produce the same load.
func TestComputeHRMetricsIntervalsOutweighTheirAverage(t *testing.T) {
	const lthr = 165
	// Ten minutes at 180, fifty at 144 — average exactly 150.
	intervals := append(steadyTrace(180, 600), steadyTrace(144, 3000)...)

	var sum float64
	for _, p := range intervals {
		sum += p.Bpm
	}
	if avg := sum / float64(len(intervals)); !almostEqual(avg, 150, 1e-9) {
		t.Fatalf("test setup: average = %v, want 150", avg)
	}

	steady := ComputeHRMetrics(steadyTrace(150, 3600), lthr)
	ragged := ComputeHRMetrics(intervals, lthr)
	if steady == nil || ragged == nil {
		t.Fatal("ComputeHRMetrics = nil, want results")
	}
	if ragged.TSS <= steady.TSS {
		t.Errorf("interval TSS = %v, steady TSS = %v — the interval ride must count for more",
			ragged.TSS, steady.TSS)
	}
	// Hand-calculated: sqrt((600*(180/165)^2 + 3000*(144/165)^2) / 3600).
	wantIF := 0.9127200
	if !almostEqual(ragged.IntensityFactor, wantIF, 1e-6) {
		t.Errorf("interval IF = %v, want %v", ragged.IntensityFactor, wantIF)
	}
	// TSS = hours * IF^2 * 100 must still hold exactly, or the intensity shown
	// on the ride page and the load behind the fitness curve disagree.
	if want := ragged.IntensityFactor * ragged.IntensityFactor * 100; !almostEqual(ragged.TSS, want, 1e-9) {
		t.Errorf("TSS = %v, want %v (hours * IF^2 * 100)", ragged.TSS, want)
	}
}

func TestComputeHRMetricsNilWithoutInputs(t *testing.T) {
	if got := ComputeHRMetrics(steadyTrace(0, 3600), 165); got != nil {
		t.Errorf("zero bpm: got %v, want nil", got)
	}
	if got := ComputeHRMetrics(steadyTrace(150, 3600), 0); got != nil {
		t.Errorf("zero LTHR: got %v, want nil", got)
	}
	if got := ComputeHRMetrics(nil, 165); got != nil {
		t.Errorf("empty trace: got %v, want nil", got)
	}
}
