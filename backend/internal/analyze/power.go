// Package analyze computes training metrics (NP/IF/TSS, CTL/ATL/TSB,
// insights) per arch-wff-analyze. This file holds the pure power-based math
// (no DB access) so it's trivially unit-testable against hand-calculated
// reference values.
package analyze

import "math"

// PowerMetrics holds the computed Normalized Power / Intensity Factor /
// Training Stress Score for one activity.
type PowerMetrics struct {
	NormalizedPowerWatts float64
	IntensityFactor      float64
	TSS                  float64
}

// NormalizedPower computes NP from a power time series (1 Hz, as produced
// by fitparse). Uses a 30-second rolling average, raises it to the 4th
// power, averages, then takes the 4th root. Rides shorter than 30 samples
// degrade to a single window covering everything available — NP reduces to
// a plain average for very short rides, which is the documented, accepted
// simplification (see arch-wff-analyze).
func NormalizedPower(powerWatts []float64) float64 {
	n := len(powerWatts)
	if n == 0 {
		return 0
	}
	window := 30
	if window > n {
		window = n
	}

	var sumFourthPower, windowSum float64
	var count int
	for i, p := range powerWatts {
		windowSum += p
		if i >= window {
			windowSum -= powerWatts[i-window]
		}
		if i >= window-1 {
			avg := windowSum / float64(window)
			sumFourthPower += avg * avg * avg * avg
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return math.Pow(sumFourthPower/float64(count), 0.25)
}

// ComputePowerMetrics derives NP/IF/TSS. Returns nil if there's nothing to
// compute from (no power samples or no FTP configured) — never a bogus
// number, never an error for what is a perfectly normal state (see
// arch-wff-analyze on the FTP-optional contract).
//
// durationSeconds must be the duration the power samples actually cover
// (moving/recording time), not the ride's wall-clock elapsed time. A device
// with auto-pause enabled stops writing samples during a stop, so NP is
// already computed over moving time only — multiplying it by the longer
// elapsed time (which includes those stops) would overstate TSS by exactly
// elapsed/moving for every ride with a red light or a café stop in it. The
// heart-rate path (ComputeHRMetrics) gets this right by construction, since
// its time comes from the gaps between samples rather than a stored elapsed
// figure — this parameter keeps the power path on the same footing.
func ComputePowerMetrics(powerWatts []float64, durationSeconds int, ftpWatts int) *PowerMetrics {
	if len(powerWatts) == 0 || ftpWatts <= 0 || durationSeconds <= 0 {
		return nil
	}
	np := NormalizedPower(powerWatts)
	ftp := float64(ftpWatts)
	ifactor := np / ftp
	tss := float64(durationSeconds) * np * ifactor / (ftp * 3600) * 100
	return &PowerMetrics{NormalizedPowerWatts: np, IntensityFactor: ifactor, TSS: tss}
}
