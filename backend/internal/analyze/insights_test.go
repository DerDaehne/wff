package analyze

import (
	"strings"
	"testing"
	"time"
)

func daySeries(n int, ctl func(i int) float64, tsb func(i int) float64) []DayLoad {
	series := make([]DayLoad, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		series[i] = DayLoad{Date: base.AddDate(0, 0, i), CTL: ctl(i), TSB: tsb(i)}
	}
	return series
}

// Advice is matched across both halves: which half a phrase lands in is
// wording, not behaviour.
func containsMessage(insights []Insight, substr string) bool {
	for _, ins := range insights {
		if strings.Contains(ins.Action+" "+ins.Reason, substr) {
			return true
		}
	}
	return false
}

// The core of #603: a tip that only describes a state leaves a rider without
// training knowledge exactly where they were. Every piece of advice must say
// what to do AND why.
func TestEveryInsightGivesAnActionWithAReason(t *testing.T) {
	cases := map[string][]DayLoad{
		"deep fatigue": daySeries(14, func(int) float64 { return 50 }, func(i int) float64 {
			if i == 13 {
				return -35
			}
			return 0
		}),
		"rested and fit": daySeries(14, func(i int) float64 { return 50 + float64(i) }, func(i int) float64 {
			if i == 13 {
				return 20
			}
			return 0
		}),
		"load collapsing": func() []DayLoad {
			s := daySeries(14, func(int) float64 { return 100 }, func(int) float64 { return 0 })
			s[13].CTL = 80
			return s
		}(),
		"steady": daySeries(14, func(int) float64 { return 50 }, func(int) float64 { return 0 }),
	}

	for name, series := range cases {
		insights := Insights(series)
		if len(insights) == 0 {
			t.Errorf("%s: no insight at all", name)
			continue
		}
		for _, ins := range insights {
			if ins.Action == "" {
				t.Errorf("%s: insight without an instruction: %+v", name, ins)
			}
			if ins.Reason == "" {
				t.Errorf("%s: instruction without a reason: %+v", name, ins)
			}
			if ins.Severity == "" {
				t.Errorf("%s: insight without severity: %+v", name, ins)
			}
		}
	}
}

// With too little history the app must decline to advise rather than guess.
// This audience cannot sanity-check a confident wrong recommendation.
func TestInsightsInsufficientHistoryGivesNoInstruction(t *testing.T) {
	series := daySeries(3, func(int) float64 { return 10 }, func(int) float64 { return 0 })
	insights := Insights(series)

	if len(insights) != 1 {
		t.Fatalf("len(insights) = %d, want exactly the honest note", len(insights))
	}
	if insights[0].Action != "" {
		t.Errorf("advice invented from 3 days of history: %q", insights[0].Action)
	}
	if !strings.Contains(insights[0].Reason, "3 von 7") {
		t.Errorf("the note should say how far along the rider is, got %q", insights[0].Reason)
	}
}

func TestInsightsHighFatigue(t *testing.T) {
	series := daySeries(7, func(int) float64 { return 50 }, func(i int) float64 {
		if i == 6 {
			return -35 // latest day: deep fatigue
		}
		return 0
	})
	insights := Insights(series)
	if !containsMessage(insights, "locker") {
		t.Fatalf("insights = %+v, want an instruction to ease off (TSB=-35 on the latest day)", insights)
	}
	if !containsMessage(insights, "Erholung") {
		t.Fatalf("insights = %+v, want the reason to mention recovery", insights)
	}
}

func TestInsightsGoodForm(t *testing.T) {
	series := daySeries(7, func(i int) float64 { return 50 + float64(i) }, func(i int) float64 {
		if i == 6 {
			return 20 // fresh
		}
		return 0
	})
	insights := Insights(series)
	if !containsMessage(insights, "Anspruchsvolles") {
		t.Fatalf("insights = %+v, want a go-hard-now tip (TSB=20, CTL rising over the week)", insights)
	}
}

// Being rested is an advantage before a hard day and a problem as a permanent
// state — the difference is how long it has lasted.
func TestInsightsFreshForTooLong(t *testing.T) {
	short := daySeries(14, func(int) float64 { return 30 }, func(i int) float64 {
		if i >= 12 {
			return 20 // rested for two days
		}
		return 0
	})
	if containsMessage(Insights(short), "zu wenig") {
		t.Error("two rested days already treated as undertraining")
	}

	long := daySeries(20, func(int) float64 { return 30 }, func(i int) float64 {
		if i >= 8 {
			return 20 // rested for twelve days straight
		}
		return 0
	})
	if !containsMessage(Insights(long), "zu wenig") {
		t.Errorf("insights = %+v, want the undertraining tip after 12 rested days", Insights(long))
	}
}

func TestInsightsCTLDropping(t *testing.T) {
	series := daySeries(7, func(int) float64 { return 100 }, func(int) float64 { return 0 })
	series[6].CTL = 85 // >10% drop from the week-ago value

	insights := Insights(series)
	if !containsMessage(insights, "feste Termine") {
		t.Fatalf("insights = %+v, want a concrete instruction (CTL dropped 15%% over the week)", insights)
	}
	if !containsMessage(insights, "15 %") {
		t.Fatalf("insights = %+v, want the reason to quote the actual drop", insights)
	}
}
