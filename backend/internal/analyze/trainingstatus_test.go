package analyze

import (
	"strings"
	"testing"
	"time"
)

// days builds a CTL series ending today: ctlFrom at the start, ctlTo at the
// end, linear in between. TSB is set on the last day only, which is all the
// status reads.
func days(n int, ctlFrom, ctlTo, finalTSB float64) []DayLoad {
	out := make([]DayLoad, n)
	start := time.Now().AddDate(0, 0, -(n - 1))
	for i := range out {
		ctl := ctlFrom
		if n > 1 {
			ctl = ctlFrom + (ctlTo-ctlFrom)*float64(i)/float64(n-1)
		}
		out[i] = DayLoad{Date: start.AddDate(0, 0, i), CTL: ctl, ATL: ctl}
	}
	out[n-1].TSB = finalTSB
	return out
}

func TestTrainingStatusFormBands(t *testing.T) {
	for _, tc := range []struct {
		tsb   float64
		title string
	}{
		{20, "frisch und ausgeruht"},
		{8, "gut erholt"},
		{0, "normalen Bereich"},
		{-20, "Belastungsphase"},
		{-40, "deutlich ermüdet"},
	} {
		status := TrainingStatus(days(30, 40, 40, tc.tsb))
		if !strings.Contains(status.Title, tc.title) {
			t.Errorf("TSB %.0f: title %q, want it to mention %q", tc.tsb, status.Title, tc.title)
		}
		form := kinds(status)["form"]
		if form == "" {
			t.Errorf("TSB %.0f: no form explanation", tc.tsb)
		}
	}
}

// The whole point of the dashboard: "werde ich besser?" must be answered, and
// only when the data can answer it.
func TestTrainingStatusTrend(t *testing.T) {
	rising := kinds(TrainingStatus(days(30, 30, 45, 5)))["trend"]
	if !strings.Contains(rising, "gestiegen") {
		t.Errorf("rising fitness not reported: %q", rising)
	}

	falling := kinds(TrainingStatus(days(30, 45, 30, 5)))["trend"]
	if !strings.Contains(falling, "gesunken") {
		t.Errorf("falling fitness not reported: %q", falling)
	}

	flat := kinds(TrainingStatus(days(30, 40, 41, 5)))["trend"]
	if !strings.Contains(flat, "gleich geblieben") {
		t.Errorf("flat fitness not reported: %q", flat)
	}

	// A tiny absolute change is not a trend even when the percentage is huge:
	// CTL 2 → 3 is +50 %.
	noise := kinds(TrainingStatus(days(30, 2, 3, 5)))["trend"]
	if !strings.Contains(noise, "gleich geblieben") {
		t.Errorf("CTL 2->3 sold as a trend: %q", noise)
	}

	// Too little history: an honest "can't say yet", not a verdict.
	short := TrainingStatus(days(10, 20, 40, 5))
	k := kinds(short)
	if k["trend"] != "" {
		t.Errorf("trend claimed from 10 days: %q", k["trend"])
	}
	if !strings.Contains(k["hint_history"], "14 Tage") {
		t.Errorf("expected an honest history hint, got %q", k["hint_history"])
	}
}

func TestTrainingStatusStatsKeepTheTechnicalName(t *testing.T) {
	status := TrainingStatus(days(30, 40, 40, 12))

	labels := map[string]string{}
	for _, s := range status.Stats {
		labels[s.Label] = s.Value
	}
	// Plain word first, jargon in brackets — a rider who reads about CTL
	// elsewhere still has to be able to connect it (#600, guideline 1).
	for _, want := range []string{"Fitness (CTL)", "Müdigkeit (ATL)", "Frische (TSB)"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("missing stat %q, got %v", want, labels)
		}
	}
	if labels["Frische (TSB)"] != "+12" {
		t.Errorf("positive form should keep its sign, got %q", labels["Frische (TSB)"])
	}
	if got := TrainingStatus(days(30, 40, 40, -12)); got.Stats[2].Value != "-12" {
		t.Errorf("negative form rendered as %q", got.Stats[2].Value)
	}
}

func TestTrainingStatusEmptySeries(t *testing.T) {
	if s := TrainingStatus(nil); s.Title != "" || len(s.Statements) != 0 {
		t.Errorf("status invented without any data: %+v", s)
	}
}
