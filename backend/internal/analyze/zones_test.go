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
