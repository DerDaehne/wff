package analyze

import "testing"

func almostEqual(a, b, tolerance float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}

func constantPower(watts float64, seconds int) []float64 {
	p := make([]float64, seconds)
	for i := range p {
		p[i] = watts
	}
	return p
}

func TestComputePowerMetricsConstantPowerReference(t *testing.T) {
	// Reference case from ticket #563: 1h at a constant 200W, FTP 250W ->
	// NP=200, IF=0.8, TSS=64 (hand-verified against the Coggan formulas).
	power := constantPower(200, 3600)
	metrics := ComputePowerMetrics(power, 3600, 250)
	if metrics == nil {
		t.Fatal("ComputePowerMetrics = nil, want a result")
	}
	if !almostEqual(metrics.NormalizedPowerWatts, 200, 1e-6) {
		t.Errorf("NP = %v, want 200", metrics.NormalizedPowerWatts)
	}
	if !almostEqual(metrics.IntensityFactor, 0.8, 1e-9) {
		t.Errorf("IF = %v, want 0.8", metrics.IntensityFactor)
	}
	if !almostEqual(metrics.TSS, 64, 1e-6) {
		t.Errorf("TSS = %v, want 64", metrics.TSS)
	}
}

func TestNormalizedPowerExceedsAverageForIntervals(t *testing.T) {
	// Alternating hard/easy intervals: NP must be strictly greater than the
	// plain average, since NP weights variability (the whole point of the
	// metric — a spiky ride is more stressful than its average suggests).
	power := make([]float64, 600) // 10 minutes
	for i := range power {
		if (i/60)%2 == 0 {
			power[i] = 350 // 1 min hard
		} else {
			power[i] = 100 // 1 min easy
		}
	}
	np := NormalizedPower(power)

	var sum float64
	for _, p := range power {
		sum += p
	}
	avg := sum / float64(len(power))

	if np <= avg {
		t.Fatalf("NP = %v, want > avgPower (%v) for a variable-intensity ride", np, avg)
	}
}

func TestNormalizedPowerShortRideDegradesToAverage(t *testing.T) {
	// Fewer than 30 samples: no full 30s window exists, NP must reduce to
	// the plain average (documented simplification).
	power := []float64{100, 200, 300}
	np := NormalizedPower(power)
	if !almostEqual(np, 200, 1e-9) {
		t.Errorf("short-ride NP = %v, want 200 (plain average)", np)
	}
}

func TestComputePowerMetricsNilWithoutFTPOrPower(t *testing.T) {
	if got := ComputePowerMetrics(nil, 3600, 250); got != nil {
		t.Errorf("no power samples: got %v, want nil", got)
	}
	if got := ComputePowerMetrics(constantPower(200, 100), 3600, 0); got != nil {
		t.Errorf("no FTP: got %v, want nil", got)
	}
}
