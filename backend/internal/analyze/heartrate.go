package analyze

import "math"

// MaxSampleGapSeconds is where a sampling interval stops being one and becomes
// a stop. Anything longer is a traffic light, a photo or a café, and it belongs
// to no part of the ride — every calculation that weighs samples by time uses
// this same limit so they can't disagree about how long a ride was.
const MaxSampleGapSeconds = 60

// HRMetrics holds the heart-rate-based Intensity Factor / TSS estimate used
// when a ride has no power data at all.
type HRMetrics struct {
	IntensityFactor float64
	TSS             float64
}

// HRPoint is one heart-rate reading and the stretch of time it stands for.
type HRPoint struct {
	Bpm     float64
	Seconds float64
}

// ComputeHRMetrics estimates TSS from the heart-rate trace relative to LTHR
// (lactate threshold heart rate).
//
// TSS = hours * IF^2 * 100 is the algebraically exact simplification of the
// power formula once NP = IF*FTP is substituted in — see arch-wff-analyze.
// Here IF_hr stands in for NP/FTP as the intensity proxy.
//
// The intensity is integrated over the trace rather than taken from the ride's
// average heart rate (#622). Averaging first flattens an interval session into
// "that was easy": ten hard minutes and fifty calm ones come out as one calm
// hour, and the whole fitness/fatigue/form chain hangs off that number.
// Squaring is convex, so summing the squares can only ever exceed the square of
// the sum — the load rises exactly where the ride was ragged, and a perfectly
// steady ride lands on the same number as before. That equality is the point:
// the scale stays the one every threshold in insights.go was calibrated on,
// unlike an Edwards or Banister TRIMP, which would need an invented constant to
// map onto it.
//
// Time comes from the gaps between samples, so a stop is not counted as time
// spent at the average intensity.
//
// ponytail: no rolling average, unlike normalised power's 30 s window. Heart
// rate already lags the effort, so a 20-second attack shows up late and
// blunted either way. Smooth first if that ever needs to be sharper.
func ComputeHRMetrics(trace []HRPoint, lthrBpm int) *HRMetrics {
	if lthrBpm <= 0 {
		return nil
	}
	var seconds, weighted float64
	for _, p := range trace {
		if p.Bpm <= 0 || p.Seconds <= 0 {
			continue
		}
		ratio := p.Bpm / float64(lthrBpm)
		weighted += ratio * ratio * p.Seconds
		seconds += p.Seconds
	}
	if seconds <= 0 {
		return nil
	}

	// The root mean square of the ratio: the steady intensity that would have
	// cost the same. Reporting the plain average here instead would contradict
	// the TSS next to it.
	ifactor := math.Sqrt(weighted / seconds)
	return &HRMetrics{
		IntensityFactor: ifactor,
		TSS:             seconds / 3600 * ifactor * ifactor * 100,
	}
}
