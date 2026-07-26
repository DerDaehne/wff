package analyze

import (
	"strings"
	"testing"
)

func TestTrainingLevelBands(t *testing.T) {
	for _, tc := range []struct {
		ctl  float64
		name string
	}{
		{8, "Einstieg"},
		{30, "Gelegenheitsfahrer"},
		{55, "Regelmäßig im Sattel"},
		{85, "Ambitioniert"},
		{130, "Wettkampfniveau"},
	} {
		gauge := TrainingStatus(days(30, tc.ctl, tc.ctl, 0)).Gauge
		if gauge == nil {
			t.Fatalf("CTL %.0f: no level", tc.ctl)
		}
		if !strings.Contains(gauge.Label, tc.name) {
			t.Errorf("CTL %.0f: %q, want it to say %q", tc.ctl, gauge.Label, tc.name)
		}
		if gauge.Percent < 0 || gauge.Percent > 100 {
			t.Errorf("CTL %.0f: bar at %d %%", tc.ctl, gauge.Percent)
		}
	}
}

// The bar shows progress THROUGH the band, so it has to rise within a band and
// reset when the next one starts — otherwise it says nothing the number didn't.
func TestTrainingLevelProgressWithinBand(t *testing.T) {
	low := TrainingStatus(days(30, 45, 45, 0)).Gauge  // just into 40–70
	high := TrainingStatus(days(30, 65, 65, 0)).Gauge // near its top
	next := TrainingStatus(days(30, 72, 72, 0)).Gauge // just into 70–100

	if low.Percent >= high.Percent {
		t.Errorf("progress did not rise within the band: %d %% then %d %%", low.Percent, high.Percent)
	}
	if next.Percent >= high.Percent {
		t.Errorf("crossing into the next band should reset progress: %d %% after %d %%", next.Percent, high.Percent)
	}
	if !strings.Contains(next.Label, "Ambitioniert") {
		t.Errorf("CTL 72 should be the next band up, got %q", next.Label)
	}
}

// A week of riding describes the last few days, not a training level.
func TestTrainingLevelNeedsHistory(t *testing.T) {
	if g := TrainingStatus(days(7, 40, 40, 0)).Gauge; g != nil {
		t.Errorf("level claimed from 7 days of history: %+v", *g)
	}
}
