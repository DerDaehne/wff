package analyze

import (
	"strings"
	"testing"
	"time"
)

func daySeries(n int, ctl func(i int) float64, tsb func(i int) float64) []DayLoad {
	series := make([]DayLoad, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		series[i] = DayLoad{Date: base.AddDate(0, 0, i), CTL: ctl(i), TSB: tsb(i)}
	}
	return series
}

func containsMessage(insights []Insight, substr string) bool {
	for _, ins := range insights {
		if strings.Contains(ins.Message, substr) {
			return true
		}
	}
	return false
}

func TestInsightsInsufficientHistory(t *testing.T) {
	series := daySeries(3, func(int) float64 { return 10 }, func(int) float64 { return 0 })
	insights := Insights(series)
	if len(insights) != 1 {
		t.Fatalf("len(insights) = %d, want 1 for insufficient history", len(insights))
	}
	if insights[0].Message == "" {
		t.Fatal("generic insight has empty message")
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
	if !containsMessage(insights, "Ermüdung") {
		t.Fatalf("insights = %+v, want a fatigue warning (TSB=-35 on the latest day)", insights)
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
	if !containsMessage(insights, "Gute Form") {
		t.Fatalf("insights = %+v, want a good-form tip (TSB=20, CTL rising over the week)", insights)
	}
}

func TestInsightsCTLDropping(t *testing.T) {
	series := daySeries(7, func(i int) float64 {
		if i == 0 {
			return 100
		}
		return 100 // week-ago value stays 100
	}, func(int) float64 { return 0 })
	series[6].CTL = 85 // >10% drop from the week-ago value (series[0].CTL=100)

	insights := Insights(series)
	if !containsMessage(insights, "sinkt") {
		t.Fatalf("insights = %+v, want a declining-load tip (CTL dropped 15%% over the week)", insights)
	}
}

func TestInsightsNeutralStillReturnsAtLeastOne(t *testing.T) {
	series := daySeries(7, func(int) float64 { return 50 }, func(int) float64 { return 0 })
	insights := Insights(series)
	if len(insights) == 0 {
		t.Fatal("Insights returned no tips for a neutral series, want at least the generic fallback")
	}
	if !containsMessage(insights, "normalen Bereich") {
		t.Fatalf("insights = %+v, want the neutral fallback message", insights)
	}
}
