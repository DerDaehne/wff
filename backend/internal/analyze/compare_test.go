package analyze

import (
	"testing"
	"time"
)

func dayLoadAgo(daysAgo int, ctl float64) DayLoad {
	return DayLoad{Date: time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -daysAgo), CTL: ctl}
}

func TestCtlDeltaOverFourWeeks(t *testing.T) {
	series := []DayLoad{
		dayLoadAgo(35, 40),
		dayLoadAgo(28, 45),
		dayLoadAgo(14, 50),
		dayLoadAgo(0, 60),
	}
	got := ctlDelta(series)
	if got == nil {
		t.Fatal("expected a delta, got nil")
	}
	if want := 60.0 - 45.0; *got != want {
		t.Errorf("delta = %v, want %v (today's CTL minus the entry at the 28-day mark)", *got, want)
	}
}

func TestCtlDeltaNilWithoutAFullWindow(t *testing.T) {
	series := []DayLoad{dayLoadAgo(10, 20), dayLoadAgo(0, 30)}
	if got := ctlDelta(series); got != nil {
		t.Errorf("delta = %v, want nil — only 10 days of history, not the full 4-week window", *got)
	}
}

func TestCtlDeltaNilForNoHistoryAtAll(t *testing.T) {
	if got := ctlDelta(nil); got != nil {
		t.Errorf("delta = %v, want nil for a rider with no TSS-based rides at all", *got)
	}
}
