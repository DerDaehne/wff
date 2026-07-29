package analyze

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }

// kinds present in a story, for asserting what did and did not get said.
func kinds(s Story) map[string]string {
	out := map[string]string{}
	for _, st := range s.Statements {
		out[st.Kind] = st.Text
	}
	return out
}

// statementOf finds the one statement of a given kind, for tests that need
// more than its Text (e.g. Metrics).
func statementOf(s Story, kind string) (Statement, bool) {
	for _, st := range s.Statements {
		if st.Kind == kind {
			return st, true
		}
	}
	return Statement{}, false
}

func TestRideStorySessionBands(t *testing.T) {
	for _, tc := range []struct {
		ifactor  float64
		headline string
	}{
		{0.55, "Erholungsfahrt"},
		{0.70, "Grundlagenfahrt"},
		{0.80, "Zügige Tempofahrt"},
		{0.90, "Harte Einheit"},
		{1.05, "Sehr harte Einheit"},
	} {
		story := RideStory(RideFacts{
			IntensityFactor: f64(tc.ifactor),
			DistanceMeters:  f64(42300),
			ElapsedSeconds:  5400,
			FromPower:       true,
		})
		if story.Title != tc.headline {
			t.Errorf("IF %.2f: title %q, want %q", tc.ifactor, story.Title, tc.headline)
		}
		// Distance and duration moved out of the title into the headline stats,
		// so they can be typeset large with a small unit (#607).
		stats := map[string]string{}
		for _, s := range story.Stats {
			stats[s.Label] = s.Value + " " + s.Unit
		}
		if stats["Distanz"] != "42,3 km" {
			t.Errorf("IF %.2f: distance stat %q", tc.ifactor, stats["Distanz"])
		}
		if stats["Dauer"] != "1:30 h" {
			t.Errorf("IF %.2f: duration stat %q", tc.ifactor, stats["Dauer"])
		}
		if story.Gauge == nil {
			t.Fatalf("IF %.2f: no intensity gauge", tc.ifactor)
		}
		// The bar clamps at 100 %, the label keeps the true reading.
		wantPercent := int(tc.ifactor*100 + 0.5)
		if story.Gauge.Percent != min(wantPercent, 100) {
			t.Errorf("IF %.2f: gauge %d %%", tc.ifactor, story.Gauge.Percent)
		}
		if !strings.Contains(story.Gauge.Label, fmt.Sprint(wantPercent)) {
			t.Errorf("IF %.2f: gauge label %q lost the real value", tc.ifactor, story.Gauge.Label)
		}
	}
}

func TestGermanDate(t *testing.T) {
	// 2026-07-26 is a Sunday.
	got := germanDate(time.Date(2026, 7, 26, 8, 5, 0, 0, time.Local))
	if got != "Sonntag, 26. Juli · 08:05" {
		t.Errorf("germanDate = %q", got)
	}
	if germanDate(time.Time{}) != "" {
		t.Error("zero time should produce no subtitle")
	}
}

// Without FTP or LTHR there is no intensity at all — the story must say so
// instead of pretending the ride was easy.
func TestRideStoryWithoutIntensity(t *testing.T) {
	story := RideStory(RideFacts{DistanceMeters: f64(20000), ElapsedSeconds: 3600})

	k := kinds(story)
	if _, ok := k["effort"]; ok {
		t.Error("effort statement made up despite missing intensity factor")
	}
	hint, ok := k["hint_profile"]
	if !ok || !strings.Contains(hint, "FTP") {
		t.Errorf("expected a hint pointing at the missing FTP/LTHR, got %q", hint)
	}
	if _, ok := k["load"]; ok {
		t.Error("load statement made up despite missing TSS")
	}
}

func TestRideStoryHeartRatePathIsLabelledAsEstimate(t *testing.T) {
	fromHR := RideStory(RideFacts{IntensityFactor: f64(0.8), ElapsedSeconds: 3600, FromPower: false})
	fromPower := RideStory(RideFacts{IntensityFactor: f64(0.8), ElapsedSeconds: 3600, FromPower: true})

	if !strings.Contains(fromHR.Statements[0].Metric, "Puls") {
		t.Errorf("HR path not labelled as an estimate: %q", fromHR.Statements[0].Metric)
	}
	if !strings.Contains(fromPower.Statements[0].Metric, "Leistung") {
		t.Errorf("power path mislabelled: %q", fromPower.Statements[0].Metric)
	}
}

func TestRideStoryComparisonNeedsHistory(t *testing.T) {
	base := RideFacts{IntensityFactor: f64(0.7), TSS: f64(80), ElapsedSeconds: 3600, FromPower: true}

	// Too few earlier rides: an honest "can't compare yet", never a verdict.
	base.PriorTSS = []float64{70, 75}
	if k := kinds(RideStory(base)); k["comparison"] != "" {
		t.Errorf("compared against %d prior rides: %q", len(base.PriorTSS), k["comparison"])
	}

	base.PriorTSS = []float64{50, 55, 60} // median 55, ratio 80/55 ≈ 1.45
	if k := kinds(RideStory(base)); !strings.Contains(k["comparison"], "Deutlich anstrengender") {
		t.Errorf("hard ride not called out: %q", k["comparison"])
	}

	base.PriorTSS = []float64{140, 150, 160} // median 150, ratio 80/150 ≈ 0.53
	if k := kinds(RideStory(base)); !strings.Contains(k["comparison"], "Ruhiger") {
		t.Errorf("easy ride not called out: %q", k["comparison"])
	}

	base.PriorTSS = []float64{75, 80, 85} // median 80, ratio 1.0
	if k := kinds(RideStory(base)); !strings.Contains(k["comparison"], "im Rahmen") {
		t.Errorf("typical ride not called out: %q", k["comparison"])
	}
}

func TestRideStoryContextOnlyWhenNoticeable(t *testing.T) {
	quiet := RideStory(RideFacts{
		IntensityFactor:     f64(0.7),
		ElapsedSeconds:      3600,
		HeadwindMps:         f64(0.3), // barely a breeze
		ElevationGainMeters: f64(120), // flat-ish
		TemperatureCelsius:  f64(18),  // pleasant
	})
	for _, s := range quiet.Statements {
		if s.Kind == "context" {
			t.Errorf("unremarkable conditions produced context noise: %q / %q", s.Text, s.Metric)
		}
	}

	windy := RideStory(RideFacts{
		IntensityFactor:     f64(0.7),
		ElapsedSeconds:      3600,
		HeadwindMps:         f64(2.5),
		ElevationGainMeters: f64(800),
		TemperatureCelsius:  f64(31),
	})
	var context []Statement
	for _, s := range windy.Statements {
		if s.Kind == "context" {
			context = append(context, s)
		}
	}
	if len(context) != 3 {
		t.Fatalf("want wind + hills + heat context, got %d: %+v", len(context), context)
	}
	if !strings.Contains(context[0].Metric, "Gegenwind") {
		t.Errorf("headwind mislabelled: %q", context[0].Metric)
	}

	tailwind := RideStory(RideFacts{IntensityFactor: f64(0.7), ElapsedSeconds: 3600, HeadwindMps: f64(-2.5)})
	if !strings.Contains(tailwind.Statements[1].Metric, "Rückenwind") {
		t.Errorf("negative headwind is tailwind, got %q", tailwind.Statements[1].Metric)
	}
}

func TestRideStoryLoadBands(t *testing.T) {
	for _, tc := range []struct {
		tss  float64
		want string
	}{
		{30, "morgen bist du wieder frisch"},
		{80, "über Nacht"},
		{150, "in den Beinen spüren"},
		{250, "ein bis zwei ruhige Tage"},
		{400, "mehrere Tage Erholung"},
	} {
		k := kinds(RideStory(RideFacts{IntensityFactor: f64(0.7), TSS: f64(tc.tss), ElapsedSeconds: 3600}))
		if !strings.Contains(k["load"], tc.want) {
			t.Errorf("TSS %.0f: got %q, want it to mention %q", tc.tss, k["load"], tc.want)
		}
	}
}

// Statement.Metrics (#651) is what the frontend's compact row typesets as a
// big number instead of showing the pre-formatted Metric string — every
// statement kind that carries a single clear value must populate it.
func TestRideStoryMetricsAreStructured(t *testing.T) {
	story := RideStory(RideFacts{
		IntensityFactor: f64(0.89),
		TSS:             f64(79),
		FromPower:       false,
		ElapsedSeconds:  3600,
	})

	effort, ok := statementOf(story, "effort")
	if !ok {
		t.Fatal("no effort statement")
	}
	if len(effort.Metrics) != 1 || effort.Metrics[0].Value != "0,89" || effort.Metrics[0].Label != "Intensität (IF)" {
		t.Errorf("effort.Metrics = %+v, want one Stat{Value: \"0,89\", Label: \"Intensität (IF)\"}", effort.Metrics)
	}

	load, ok := statementOf(story, "load")
	if !ok {
		t.Fatal("no load statement")
	}
	if len(load.Metrics) != 1 || load.Metrics[0].Value != "79" || load.Metrics[0].Label != "Belastung (TSS)" {
		t.Errorf("load.Metrics = %+v, want one Stat{Value: \"79\", Label: \"Belastung (TSS)\"}", load.Metrics)
	}
}
