package analyze

import (
	"strings"
	"testing"
	"time"
)

// weeks builds n weekly blocks; speedKmh and km may vary per index so a trend
// can be shaped.
func weeks(n int, km func(i int) float64, speedKmh func(i int) float64) []Week {
	out := make([]Week, n)
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	for i := range out {
		distance := km(i) * 1000
		seconds := int(distance / (speedKmh(i) / 3.6))
		out[i] = Week{
			Start:          start.AddDate(0, 0, 7*i),
			Rides:          3,
			DistanceMeters: distance,
			MovingSeconds:  seconds,
			AvgSpeedKmh:    speedKmh(i),
		}
	}
	return out
}

func statementText(statements []Statement) string {
	var b strings.Builder
	for _, s := range statements {
		b.WriteString(s.Text + " " + s.Metric + " ")
	}
	return b.String()
}

// The whole point of the view: seeing that you got faster.
func TestProgressReportsGettingFaster(t *testing.T) {
	// Four weeks at 26 km/h, then four at 28.
	got := progressStatements(weeks(8, func(int) float64 { return 100 }, func(i int) float64 {
		if i < 4 {
			return 26
		}
		return 28
	}))

	text := statementText(got)
	if !strings.Contains(text, "schneller") {
		t.Errorf("2 km/h faster over two months not reported: %q", text)
	}
	// Honesty caveat is part of the statement, not a footnote elsewhere.
	if !strings.Contains(text, "Wind") {
		t.Errorf("speed claim made without naming its limits: %q", text)
	}
}

func TestProgressReportsSlowingDown(t *testing.T) {
	got := progressStatements(weeks(8, func(int) float64 { return 100 }, func(i int) float64 {
		if i < 4 {
			return 29
		}
		return 26
	}))
	if !strings.Contains(statementText(got), "unter dem von vor zwei Monaten") {
		t.Errorf("slowing down not reported: %q", statementText(got))
	}
}

// Half a km/h across two months is noise, not progress.
func TestProgressIgnoresTinySpeedChanges(t *testing.T) {
	got := progressStatements(weeks(8, func(int) float64 { return 100 }, func(i int) float64 {
		if i < 4 {
			return 26
		}
		return 26.2
	}))
	if !strings.Contains(statementText(got), "gleich geblieben") {
		t.Errorf("0.2 km/h sold as a change: %q", statementText(got))
	}
}

func TestProgressReportsVolume(t *testing.T) {
	more := progressStatements(weeks(8, func(i int) float64 {
		if i < 4 {
			return 80
		}
		return 120
	}, func(int) float64 { return 26 }))
	if !strings.Contains(statementText(more), "mehr Kilometer") {
		t.Errorf("rising volume not reported: %q", statementText(more))
	}

	less := progressStatements(weeks(8, func(i int) float64 {
		if i < 4 {
			return 120
		}
		return 70
	}, func(int) float64 { return 26 }))
	if !strings.Contains(statementText(less), "weniger unterwegs") {
		t.Errorf("falling volume not reported: %q", statementText(less))
	}
}

// Without two comparable blocks there is nothing to compare, and saying so
// beats inventing a direction.
func TestProgressNeedsTwoBlocks(t *testing.T) {
	got := progressStatements(weeks(5, func(int) float64 { return 100 }, func(int) float64 { return 26 }))
	if len(got) != 1 || got[0].Kind != "hint_history" {
		t.Fatalf("trend claimed from 5 weeks: %+v", got)
	}
	if !strings.Contains(got[0].Text, "8 Wochen") {
		t.Errorf("the note should say how many weeks are needed, got %q", got[0].Text)
	}
}

// Weekly speed is distance over time for the whole week — otherwise a short
// spin counts as much as a long tour.
func TestBlockSpeedIsDistanceOverTime(t *testing.T) {
	block := []Week{
		{DistanceMeters: 10_000, MovingSeconds: 1800},   // 20 km/h
		{DistanceMeters: 100_000, MovingSeconds: 12000}, // 30 km/h
	}
	got := blockSpeed(block)
	// 110 km over 13800 s = 28.7 km/h. The naive mean of the two weeks' own
	// averages would be 25 — the half-hour spin would count as much as the
	// three-hour ride.
	if got < 28.5 || got > 28.9 {
		t.Errorf("blockSpeed = %.2f km/h, want ~28.7 (total distance over total time)", got)
	}
}

// currentStreakWeeks (#655): consecutive weeks with a ride, counted back from
// the most recent one present — a gap of more than 7 days between
// neighbouring entries ends the streak.
func TestCurrentStreakWeeks(t *testing.T) {
	t.Run("no weeks at all", func(t *testing.T) {
		if got := currentStreakWeeks(nil); got != 0 {
			t.Errorf("currentStreakWeeks(nil) = %d, want 0", got)
		}
	})

	t.Run("unbroken run counts every week", func(t *testing.T) {
		w := weeks(5, func(int) float64 { return 50 }, func(int) float64 { return 25 })
		if got := currentStreakWeeks(w); got != 5 {
			t.Errorf("currentStreakWeeks = %d, want 5", got)
		}
	})

	t.Run("a gap ends the streak before it", func(t *testing.T) {
		start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		w := []Week{
			{Start: start},                    // isolated week, 3 weeks before the gap
			{Start: start.AddDate(0, 0, 7*4)}, // week 4: streak starts here
			{Start: start.AddDate(0, 0, 7*5)}, // week 5
			{Start: start.AddDate(0, 0, 7*6)}, // week 6: most recent
		}
		if got := currentStreakWeeks(w); got != 3 {
			t.Errorf("currentStreakWeeks = %d, want 3 (only the last unbroken run)", got)
		}
	})
}
