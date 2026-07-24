package main

import (
	"os"
	"testing"
	"time"
)

func TestEnrichmentPollInterval(t *testing.T) {
	t.Cleanup(func() { os.Unsetenv("ENRICHMENT_POLL_INTERVAL") })

	os.Unsetenv("ENRICHMENT_POLL_INTERVAL")
	if got := enrichmentPollInterval(); got != time.Hour {
		t.Errorf("unset env: got %v, want default 1h", got)
	}

	os.Setenv("ENRICHMENT_POLL_INTERVAL", "15m")
	if got := enrichmentPollInterval(); got != 15*time.Minute {
		t.Errorf("ENRICHMENT_POLL_INTERVAL=15m: got %v, want 15m", got)
	}

	os.Setenv("ENRICHMENT_POLL_INTERVAL", "not-a-duration")
	if got := enrichmentPollInterval(); got != time.Hour {
		t.Errorf("invalid env value: got %v, want fallback default 1h", got)
	}
}
