package analyze

// HRMetrics holds the heart-rate-based Intensity Factor / TSS estimate used
// when a ride has no power data at all.
type HRMetrics struct {
	IntensityFactor float64
	TSS             float64
}

// ComputeHRMetrics estimates TSS from average heart rate relative to LTHR
// (lactate threshold heart rate). This isn't a rough approximation: TSS =
// hours * IF^2 * 100 is the algebraically exact simplification of the power
// formula once NP = IF*FTP is substituted in — see arch-wff-analyze. Here
// IF_hr = avgHR/LTHR stands in for NP/FTP as the intensity proxy. Less
// precise than the power path in practice (cardiac drift, no rolling-average
// smoothing over the ride), but exact within that substitution.
func ComputeHRMetrics(avgHeartRateBpm float64, elapsedSeconds int, lthrBpm int) *HRMetrics {
	if avgHeartRateBpm <= 0 || elapsedSeconds <= 0 || lthrBpm <= 0 {
		return nil
	}
	ifactor := avgHeartRateBpm / float64(lthrBpm)
	hours := float64(elapsedSeconds) / 3600
	tss := hours * ifactor * ifactor * 100
	return &HRMetrics{IntensityFactor: ifactor, TSS: tss}
}
