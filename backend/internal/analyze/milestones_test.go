package analyze

import "testing"

func TestDistanceMilestoneOnlyOnANewRecord(t *testing.T) {
	prior := 100_000.0 // 100 km
	f := RideFacts{DistanceMeters: new(120_000.0), Milestones: MilestoneFacts{LongestPriorMeters: &prior}}
	s, ok := distanceMilestone(f)
	if !ok {
		t.Fatal("120 km after a 100 km best did not count as a new record")
	}
	if s.Kind != "milestone" {
		t.Errorf("Kind = %q, want milestone", s.Kind)
	}

	// Same distance as the record, not a new one.
	f.DistanceMeters = new(100_000.0)
	if _, ok := distanceMilestone(f); ok {
		t.Error("matching the existing record was reported as beating it")
	}

	// Shorter than the record.
	f.DistanceMeters = new(80_000.0)
	if _, ok := distanceMilestone(f); ok {
		t.Error("a shorter ride was reported as a new distance record")
	}

	// No prior ride to compare against — nothing to have beaten yet.
	f.DistanceMeters = new(120_000.0)
	f.Milestones = MilestoneFacts{}
	if _, ok := distanceMilestone(f); ok {
		t.Error("a first-ever ride was reported as a record")
	}
}

func TestClimbMilestoneOnlyOnANewRecord(t *testing.T) {
	prior := 800.0
	f := RideFacts{ElevationGainMeters: new(1200.0), Milestones: MilestoneFacts{MostClimbingPriorMeters: &prior}}
	s, ok := climbMilestone(f)
	if !ok {
		t.Fatal("1200 hm after an 800 hm best did not count as a new record")
	}
	if s.Kind != "milestone" {
		t.Errorf("Kind = %q, want milestone", s.Kind)
	}

	f.ElevationGainMeters = new(500.0)
	if _, ok := climbMilestone(f); ok {
		t.Error("less climbing than the record was reported as beating it")
	}

	f.ElevationGainMeters = new(1200.0)
	f.Milestones = MilestoneFacts{}
	if _, ok := climbMilestone(f); ok {
		t.Error("a first-ever ride was reported as a climbing record")
	}
}
