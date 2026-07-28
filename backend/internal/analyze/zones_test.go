package analyze

import (
	"strings"
	"testing"
)

func TestZoneBoundsScaleWithThreshold(t *testing.T) {
	// Zone edges are percentages of LTHR, so a threshold of 160 puts the
	// grey-zone floor (90 %) at 144 bpm.
	got := ZoneBounds(160)
	want := []float64{129.6, 144, 150.4, 160}
	if len(got) != len(want) {
		t.Fatalf("got %d bounds, want %d — one per zone above the first", len(got), len(want))
	}
	for i := range want {
		if got[i] < want[i]-0.01 || got[i] > want[i]+0.01 {
			t.Errorf("bound %d = %.2f, want %.2f", i, got[i], want[i])
		}
	}
}

func TestRideZonesNamesTheBandTheRideLivedIn(t *testing.T) {
	// 10 min easy, 45 min base, 5 min tempo.
	d, ok := RideZones([]int{600, 2700, 300, 0, 0})
	if !ok {
		t.Fatal("refused a ride with an hour of pulse")
	}
	if len(d.Statements) != 1 || d.Statements[0].Kind != "zones" {
		t.Fatalf("statements = %+v, want one zone statement", d.Statements)
	}
	if !strings.Contains(d.Statements[0].Text, "Grundlage") {
		t.Errorf("text = %q, want it to name the band with the most time", d.Statements[0].Text)
	}
	if d.TotalSeconds != 3600 {
		t.Errorf("total = %d, want 3600", d.TotalSeconds)
	}
	if s := d.Zones[1].Share; s < 0.749 || s > 0.751 {
		t.Errorf("base share = %.3f, want 0.75", s)
	}
}

// An interval session spends most of its time recovering. Calling it "mostly
// ganz locker" would sit right under the card calling the same ride hard.
func TestRideZonesDoesNotCallAnIntervalRideEasy(t *testing.T) {
	// 40 min in the recovery band between efforts, 20 min above threshold.
	d, ok := RideZones([]int{2400, 0, 0, 0, 1200})
	if !ok {
		t.Fatal("refused a ride with an hour of pulse")
	}
	text := d.Statements[0].Text
	if strings.Contains(text, "Ganz locker") {
		t.Errorf("text = %q, want it not to sum up an interval ride as the easy band", text)
	}
	if !strings.Contains(text, "33 % hart") {
		t.Errorf("text = %q, want the split spelled out", text)
	}
}

// Power IF and pulse IF are different scales, and one ceiling for both made
// every normal base ride look non-aerobic — which silently emptied the
// endurance analysis for anyone riding on a heart-rate strap alone (#630).
func TestAerobicCeilingDiffersByScale(t *testing.T) {
	// A calm base hour: around 0.65 of threshold power, around 0.88 of
	// threshold pulse. Both readings describe the same ride, and both have to
	// pass as aerobic.
	if 0.65 > aerobicMaxIF(true) {
		t.Errorf("power ceiling %.2f rejects a calm base ride at IF 0.65", aerobicMaxIF(true))
	}
	if 0.88 > aerobicMaxIF(false) {
		t.Errorf("pulse ceiling %.2f rejects a calm base ride at IF_hr 0.88", aerobicMaxIF(false))
	}
	// A tempo ride must still be rejected on both.
	if 0.92 <= aerobicMaxIF(false) {
		t.Errorf("pulse ceiling %.2f accepts a tempo ride at IF_hr 0.92", aerobicMaxIF(false))
	}
	if 0.92 <= aerobicMaxIF(true) {
		t.Errorf("power ceiling %.2f accepts a threshold effort at IF 0.92", aerobicMaxIF(true))
	}
}

// The band a ride's title and effort card come from has to be the band its zone
// chart would put that same intensity in.
func TestEffortBandMatchesTheZoneChart(t *testing.T) {
	for ratio, want := range map[float64]string{
		0.70: "recovery",
		0.88: "endurance",
		0.92: "tempo",
		0.97: "threshold",
		1.05: "vo2",
	} {
		if got := zoneDefs[zoneForRatio(ratio)].key; got != want {
			t.Errorf("ratio %.2f fell into %q, want %q", ratio, got, want)
		}
	}

	// The same intensity of 0.88 has to mean two different things depending on
	// where it came from — otherwise one of the two paths is being mislabelled.
	if got := effortBands[effortBandFor(0.88, false)].session; got != "Grundlagenfahrt" {
		t.Errorf("IF_hr 0.88 titled %q, want Grundlagenfahrt", got)
	}
	if got := effortBands[effortBandFor(0.88, true)].session; got != "harte Einheit" {
		t.Errorf("power IF 0.88 titled %q, want harte Einheit", got)
	}
}

func TestRideZonesRefuseWhenBarelyAnyPulseWasRecorded(t *testing.T) {
	// Five minutes: the strap was picked up late, or fell off early. A
	// distribution over that describes the strap, not the ride.
	if _, ok := RideZones([]int{60, 240, 0, 0, 0}); ok {
		t.Error("a five-minute distribution was reported as if it described the ride")
	}
}

func TestWeeklyVerdictNamesTheGreyZoneProblem(t *testing.T) {
	// Four hours, more than a third of it in the tempo band: the classic
	// recreational pattern.
	d := WeeklyZones([]int{1800, 6600, 5400, 600, 0})
	if len(d.Statements) != 1 {
		t.Fatalf("statements = %+v, want one verdict", d.Statements)
	}
	s := d.Statements[0]
	if s.Kind != "zones" {
		t.Errorf("kind = %q, want zones", s.Kind)
	}
	if !strings.Contains(s.Text, "zügigen Bereich") {
		t.Errorf("text = %q, want the grey zone called out", s.Text)
	}
	if !strings.Contains(s.Text, "lockerer") {
		t.Error("the verdict doesn't say what to do about it — that's the useful half")
	}
}

func TestWeeklyVerdictNoticesNothingHardAtAll(t *testing.T) {
	// Five hours, all of it easy: a fine base, but nothing that pushes.
	d := WeeklyZones([]int{7200, 10800, 0, 0, 0})
	if !strings.Contains(d.Statements[0].Text, "fehlt der Reiz") {
		t.Errorf("text = %q, want it to point out the missing stimulus", d.Statements[0].Text)
	}
}

func TestWeeklyVerdictApprovesAPolarisedWeek(t *testing.T) {
	// 85 % easy, 5 % grey, 10 % hard — roughly the 80/20 shape.
	d := WeeklyZones([]int{7200, 10200, 1000, 1400, 600})
	if !strings.Contains(d.Statements[0].Text, "passt") {
		t.Errorf("text = %q, want the distribution confirmed rather than corrected", d.Statements[0].Text)
	}
}

func TestWeeklyVerdictWithheldUnderThreeHours(t *testing.T) {
	d := WeeklyZones([]int{1800, 3600, 600, 0, 0}) // 1 h 40
	if len(d.Statements) != 1 || d.Statements[0].Kind != "hint_history" {
		t.Fatalf("statements = %+v, want an honest refusal instead of a verdict", d.Statements)
	}
	if len(d.Zones) != len(zoneDefs) || d.TotalSeconds == 0 {
		t.Error("the bars were dropped along with the verdict — only the verdict should be withheld")
	}
}

func TestNoPulseAtAllSaysNothing(t *testing.T) {
	d := WeeklyZones([]int{0, 0, 0, 0, 0})
	if len(d.Statements) != 0 {
		t.Errorf("statements = %+v, want silence when there is no pulse data at all", d.Statements)
	}
}
