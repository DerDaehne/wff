package analyze

import "testing"

func TestComputeHRMetricsHandCalculated(t *testing.T) {
	// avgHR 150 at LTHR 165 for 1h: IF_hr = 150/165 = 0.90909..., TSS =
	// 1h * IF_hr^2 * 100 = 82.6446...
	metrics := ComputeHRMetrics(150, 3600, 165)
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
	atThreshold := ComputeHRMetrics(165, 3600, 165)
	if atThreshold == nil || !almostEqual(atThreshold.TSS, 100, 1e-9) {
		t.Errorf("1h at LTHR: TSS = %v, want 100", atThreshold)
	}
}

func TestComputeHRMetricsNilWithoutInputs(t *testing.T) {
	if got := ComputeHRMetrics(0, 3600, 165); got != nil {
		t.Errorf("zero avgHR: got %v, want nil", got)
	}
	if got := ComputeHRMetrics(150, 3600, 0); got != nil {
		t.Errorf("zero LTHR: got %v, want nil", got)
	}
	if got := ComputeHRMetrics(150, 0, 165); got != nil {
		t.Errorf("zero duration: got %v, want nil", got)
	}
}
