package analyze

import (
	"testing"
	"time"
)

func TestCaloriesHandCalculated(t *testing.T) {
	// Keytel, male: -55.0969 + 0.6309*141 + 0.1988*78 + 0.2017*40
	//             = -55.0969 + 88.9569 + 15.5064 + 8.068 = 57.4344 kJ/min
	// / 4.184 = 13.7273 kcal/min, over 60 min = 824 kcal.
	got, ok := Calories(141, 3600, 78, 40, SexMale)
	if !ok {
		t.Fatal("refused a normal hour at 141 bpm")
	}
	if got < 823 || got > 825 {
		t.Errorf("kcal = %d, want ~824", got)
	}

	// The female coefficients are a different set, not a scaled version — the
	// same inputs must not come out the same.
	female, ok := Calories(141, 3600, 78, 40, SexFemale)
	if !ok {
		t.Fatal("refused the same hour for the female coefficients")
	}
	if female == got {
		t.Errorf("both coefficient sets returned %d — one of them is not being used", got)
	}
}

func TestCaloriesScalesWithTime(t *testing.T) {
	hour, _ := Calories(141, 3600, 78, 40, SexMale)
	half, _ := Calories(141, 1800, 78, 40, SexMale)
	if d := hour - 2*half; d < -1 || d > 1 {
		t.Errorf("an hour is %d and half an hour %d — should be double", hour, half)
	}
}

// The regression was fitted on people who were exercising. Extrapolating below
// that range produces negative numbers, and a made-up figure is worse than none.
func TestCaloriesRefusesOutsideItsRange(t *testing.T) {
	cases := map[string]struct {
		hr      float64
		seconds int
		weight  float64
		age     int
		sex     string
	}{
		"pulse below the fitted range": {70, 3600, 78, 40, SexMale},
		"no time":                      {141, 0, 78, 40, SexMale},
		"no weight":                    {141, 3600, 0, 40, SexMale},
		"no age":                       {141, 3600, 78, 0, SexMale},
		"no sex given":                 {141, 3600, 78, 40, ""},
		"unknown sex value":            {141, 3600, 78, 40, "unspecified"},
	}
	for name, c := range cases {
		if _, ok := Calories(c.hr, c.seconds, c.weight, c.age, c.sex); ok {
			t.Errorf("%s: returned a figure, want a refusal", name)
		}
	}
}

// Age has to come from the season the ride happened in, or a three-year-old
// ride gets recomputed with today's age every time the app is opened.
func TestAgeComesFromTheRidesOwnYear(t *testing.T) {
	year := 1985
	f := RideFacts{BirthYear: &year, StartedAt: time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)}
	if age, ok := f.ageAtRide(); !ok || age != 35 {
		t.Errorf("age = %d (ok=%v), want 35 — the age in 2020, not now", age, ok)
	}

	typo := 1900
	f.BirthYear = &typo
	if _, ok := f.ageAtRide(); ok {
		t.Error("accepted a birth year of 1900 — a typo would produce a confident wrong number")
	}

	future := 2030
	f.BirthYear = &future
	if _, ok := f.ageAtRide(); ok {
		t.Error("accepted a birth year after the ride")
	}
}
