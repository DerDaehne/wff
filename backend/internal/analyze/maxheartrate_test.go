package analyze

import (
	"testing"
	"time"
)

func TestPlausibleAcceptsARealEffortAndRejectsALazyDay(t *testing.T) {
	year := 1990 // 36 at the ride below
	ride := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Tanaka max at 36 is 208 - 0.7*36 = 182.8; 80 % of that is 146.24.
	hard := ObservedMaxHR{Bpm: 175, RiddenAt: ride}
	if !hard.plausible(&year) {
		t.Error("175 bpm at 36 was rejected — well within a believable maximal effort")
	}

	lazy := ObservedMaxHR{Bpm: 130, RiddenAt: ride}
	if lazy.plausible(&year) {
		t.Error("130 bpm at 36 was accepted — that is only 71 % of the age-predicted maximum")
	}

	// Without a birth year there is nothing to check against.
	if !lazy.plausible(nil) {
		t.Error("a missing birth year must not turn into a rejection")
	}
}

func TestAssumedLTHRRefusesAnImplausibleMaximum(t *testing.T) {
	year := 1990
	ride := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if _, ok := AssumedLTHR(nil, &year); ok {
		t.Error("derived a threshold with no observed maximum at all")
	}

	lazy := &ObservedMaxHR{Bpm: 130, RiddenAt: ride}
	if _, ok := AssumedLTHR(lazy, &year); ok {
		t.Error("derived a threshold from a maximum that was never a real effort")
	}

	hard := &ObservedMaxHR{Bpm: 180, RiddenAt: ride}
	bpm, ok := AssumedLTHR(hard, &year)
	if !ok {
		t.Fatal("refused a plausible maximum")
	}
	if want := int(float64(hard.Bpm)*assumedLTHRShareOfMax + 0.5); bpm != want {
		t.Errorf("assumed LTHR = %d, want %d (92%% of 180)", bpm, want)
	}
}

func TestEffectiveLTHRPrefersTheRealValue(t *testing.T) {
	year := 1990
	real := 165
	max := &ObservedMaxHR{Bpm: 180, RiddenAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}

	bpm, assumed := EffectiveLTHR(&real, max, &year)
	if assumed || bpm == nil || *bpm != real {
		t.Errorf("got bpm=%v assumed=%v, want the rider's own %d unassumed", bpm, assumed, real)
	}

	bpm, assumed = EffectiveLTHR(nil, max, &year)
	if want := int(float64(max.Bpm)*assumedLTHRShareOfMax + 0.5); !assumed || bpm == nil || *bpm != want {
		t.Errorf("got bpm=%v assumed=%v, want the assumed stand-in %d", bpm, assumed, want)
	}

	bpm, assumed = EffectiveLTHR(nil, nil, &year)
	if assumed || bpm != nil {
		t.Errorf("got bpm=%v assumed=%v, want nothing with no real value and no observed max", bpm, assumed)
	}
}
