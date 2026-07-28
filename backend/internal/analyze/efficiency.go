package analyze

import (
	"fmt"
	"math"
)

// Efficiency describes how much speed or power a rider got per heartbeat, and
// whether that held up to the end of the ride (#613).
//
// Efficiency Factor (EF) = normalised power / average heart rate. Rising EF at
// the same heart rate over weeks means the aerobic base is improving. Aerobic
// decoupling compares the first half of a ride against the second: under about
// 5 % of drop-off the endurance held, well above it the ride fell apart
// towards the end.
//
// Both numbers are worthless outside a steady, aerobic ride — which is why
// most of this file is about refusing to answer rather than about the formula.
type Efficiency struct {
	// Factor is power (or speed) per heart-rate beat. Unitless on purpose: it
	// only means something compared against the same rider's other rides.
	Factor float64
	// DecouplingPct is how much Factor fell from the first half to the second.
	// Positive means it fell.
	DecouplingPct float64
	// FromPower distinguishes NP/HR from speed/HR — the speed variant is
	// affected by wind and gradient and is the weaker of the two.
	FromPower bool
}

const (
	// efficiencyMinSeconds — under half an hour the two halves are too short
	// for the second one to say anything about endurance.
	efficiencyMinSeconds = 1800
	// efficiencyMaxIF — above this the ride was not aerobic, and EF stops
	// describing the aerobic base. The usual guidance is to evaluate
	// decoupling only on steady rides below threshold.
	efficiencyMaxIF = 0.85
	// efficiencyMaxVariability — normalised power well above average power
	// means a ragged, interval-like ride. NP/avg above this is not "steady"
	// in any useful sense, and comparing its halves compares two different
	// workouts.
	efficiencyMaxVariability = 1.10
	// decouplingThresholdPct is where drop-off stops being normal drift and
	// starts meaning the endurance gave out.
	decouplingThresholdPct = 5.0
)

// aerobicMaxIF is the ceiling above which a ride stops describing the aerobic
// base. Power and pulse need different numbers because they are different
// scales, not different opinions: an easy hour is IF 0.65 on power and 0.88 on
// pulse, so one ceiling for both silently drops every heart-rate ride out of
// the endurance analysis (#630).
func aerobicMaxIF(fromPower bool) float64 {
	if fromPower {
		return efficiencyMaxIF
	}
	return aerobicMaxRatioHR
}

// EffortSample is one reading for the efficiency calculation. Speed stands in
// for power when there is no power meter.
type EffortSample struct {
	Seconds      float64
	PowerWatts   *float64
	SpeedMps     *float64
	HeartRateBpm *float64
}

// EfficiencyOf computes EF and decoupling, or reports why it won't.
//
// The refusal reasons are returned rather than silently producing a number,
// because "no value" and "value you should ignore" look identical downstream —
// and a decoupling figure from an interval session would be read as a fitness
// verdict.
func EfficiencyOf(samples []EffortSample, intensityFactor *float64, variability float64) (Efficiency, bool) {
	var total float64
	for _, s := range samples {
		total += s.Seconds
	}
	if total < efficiencyMinSeconds {
		return Efficiency{}, false
	}
	// Not steady: comparing the halves of an interval session compares two
	// different workouts.
	if variability > efficiencyMaxVariability {
		return Efficiency{}, false
	}

	usesPower := false
	for _, s := range samples {
		if s.PowerWatts != nil {
			usesPower = true
			break
		}
	}
	// Not aerobic: EF describes the aerobic base, and a threshold effort is
	// not one. Which ceiling applies depends on where the intensity came from.
	if intensityFactor != nil && *intensityFactor > aerobicMaxIF(usesPower) {
		return Efficiency{}, false
	}

	half := total / 2
	first, second := splitAt(samples, half)

	firstEF, ok1 := factorOf(first)
	secondEF, ok2 := factorOf(second)
	wholeEF, ok := factorOf(samples)
	if !ok || !ok1 || !ok2 || firstEF == 0 {
		return Efficiency{}, false
	}

	return Efficiency{
		Factor:        wholeEF,
		DecouplingPct: (firstEF - secondEF) / firstEF * 100,
		FromPower:     usesPower,
	}, true
}

// splitAt divides the ride at a point in elapsed time, not at a sample index —
// sampling intervals vary, so the halves would otherwise be unequal in the
// only dimension that matters.
func splitAt(samples []EffortSample, seconds float64) (first, second []EffortSample) {
	var elapsed float64
	for i, s := range samples {
		elapsed += s.Seconds
		if elapsed >= seconds {
			return samples[:i+1], samples[i+1:]
		}
	}
	return samples, nil
}

// factorOf is output per heartbeat over a stretch, weighted by how long each
// sample covers. Samples missing either half of the ratio are skipped rather
// than counted as zero.
func factorOf(samples []EffortSample) (float64, bool) {
	var outputSum, hrSum, seconds float64
	for _, s := range samples {
		if s.HeartRateBpm == nil || *s.HeartRateBpm <= 0 {
			continue
		}
		output := 0.0
		switch {
		case s.PowerWatts != nil:
			output = *s.PowerWatts
		case s.SpeedMps != nil:
			output = *s.SpeedMps
		default:
			continue
		}
		outputSum += output * s.Seconds
		hrSum += *s.HeartRateBpm * s.Seconds
		seconds += s.Seconds
	}
	if seconds == 0 || hrSum == 0 {
		return 0, false
	}
	avgOutput := outputSum / seconds
	avgHR := hrSum / seconds
	if avgHR <= 0 {
		return 0, false
	}
	return avgOutput / avgHR, true
}

// efficiencyStatement puts the two numbers into words. Decoupling is the
// interesting half: it is the one that says something a rider can act on.
func efficiencyStatement(e Efficiency) Statement {
	source := "aus Tempo und Puls"
	if e.FromPower {
		source = "aus Leistung und Puls"
	}

	var text string
	switch d := e.DecouplingPct; {
	case d <= decouplingThresholdPct:
		text = "Du hast diese Fahrt bis zum Schluss gleichmäßig durchgezogen — dein Puls ist gegen " +
			"Ende nicht davongelaufen. Das ist das Zeichen einer belastbaren Grundlage."
	case d <= 10:
		text = "Gegen Ende der Fahrt hat dein Puls für dasselbe Tempo etwas mehr arbeiten müssen. " +
			"Ein bisschen davon ist normal; deutlich mehr wäre ein Zeichen, dass die Grundlage noch fehlt."
	default:
		text = "Gegen Ende der Fahrt ist dein Puls für dasselbe Tempo klar nach oben gewandert. " +
			"Genau da endet gerade deine Ausdauer — mit längeren, ruhigen Fahrten verschiebt sich diese " +
			"Grenze nach hinten."
	}

	return Statement{
		Text:   text,
		Metric: fmt.Sprintf("%s %% Abfall in der zweiten Hälfte · %s", decimal(math.Abs(e.DecouplingPct), 1), source),
		Kind:   "endurance",
	}
}
