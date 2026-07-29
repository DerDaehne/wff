package fitparse_test

import (
	"errors"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
)

func TestParseValidActivity(t *testing.T) {
	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	data := fitfixture.ValidActivity(123456, created, 30)

	act, err := fitparse.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if act.FileID.SerialNumber != 123456 {
		t.Errorf("SerialNumber = %d, want 123456", act.FileID.SerialNumber)
	}
	if !act.FileID.TimeCreated.Equal(created) {
		t.Errorf("TimeCreated = %v, want %v", act.FileID.TimeCreated, created)
	}
	if act.Sport != "cycling" {
		t.Errorf("Sport = %q, want %q", act.Sport, "cycling")
	}
	if act.ElapsedSeconds != 30 || act.MovingSeconds != 30 {
		t.Errorf("Elapsed/MovingSeconds = %d/%d, want 30/30", act.ElapsedSeconds, act.MovingSeconds)
	}
	if act.AvgPowerWatts == nil || *act.AvgPowerWatts != 180 {
		t.Errorf("AvgPowerWatts = %v, want 180", act.AvgPowerWatts)
	}
	if act.NormalizedPowerWatts == nil || *act.NormalizedPowerWatts != 190 {
		t.Errorf("NormalizedPowerWatts = %v, want 190", act.NormalizedPowerWatts)
	}
	if len(act.Samples) != 30 {
		t.Fatalf("len(Samples) = %d, want 30", len(act.Samples))
	}
}

func TestParseGPSConversion(t *testing.T) {
	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	data := fitfixture.ValidActivity(1, created, 3)

	act, err := fitparse.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Record index 1 (i%3 != 0) has GPS set: semicircles 500_000_001 / 100_000_001.
	sample := act.Samples[1]
	if sample.Lat == nil || sample.Lon == nil {
		t.Fatalf("sample 1: Lat/Lon = %v/%v, want non-nil", sample.Lat, sample.Lon)
	}
	const semicircleToDegrees = 180.0 / (1 << 31)
	wantLat := float64(500_000_001) * semicircleToDegrees
	wantLon := float64(100_000_001) * semicircleToDegrees
	if diff := *sample.Lat - wantLat; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("Lat = %v, want %v", *sample.Lat, wantLat)
	}
	if diff := *sample.Lon - wantLon; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("Lon = %v, want %v", *sample.Lon, wantLon)
	}
}

func TestParseMissingFieldsAreNil(t *testing.T) {
	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	data := fitfixture.ValidActivity(1, created, 3)

	act, err := fitparse.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Record index 0 (i%3 == 0) has no GPS/altitude/temperature set by the fixture.
	sample := act.Samples[0]
	if sample.Lat != nil || sample.Lon != nil {
		t.Errorf("sample 0: Lat/Lon = %v/%v, want nil (indoor-style record with no GPS)", sample.Lat, sample.Lon)
	}
	if sample.AltitudeMeters != nil {
		t.Errorf("sample 0: AltitudeMeters = %v, want nil", *sample.AltitudeMeters)
	}
	if sample.TemperatureCelsius != nil {
		t.Errorf("sample 0: TemperatureCelsius = %v, want nil", *sample.TemperatureCelsius)
	}
	// Power/HeartRate/Cadence/Speed are always set by the fixture, even on
	// "indoor" records, so they must not be nil.
	if sample.PowerWatts == nil {
		t.Errorf("sample 0: PowerWatts = nil, want set")
	}
}

func TestParseExtendedSampleFields(t *testing.T) {
	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	data := fitfixture.ValidActivity(1, created, 3)

	act, err := fitparse.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	s := act.Samples[1]
	if s.GradePercent == nil || *s.GradePercent != 2.5 {
		t.Errorf("GradePercent = %v, want 2.5", s.GradePercent)
	}
	if s.CaloriesKcal == nil || *s.CaloriesKcal != 1 {
		t.Errorf("CaloriesKcal = %v, want 1", s.CaloriesKcal)
	}
	if s.LeftRightBalancePercent == nil || *s.LeftRightBalancePercent != 48 {
		t.Errorf("LeftRightBalancePercent = %v, want 48", s.LeftRightBalancePercent)
	}
	if s.LeftRightBalanceRightLeg == nil || !*s.LeftRightBalanceRightLeg {
		t.Errorf("LeftRightBalanceRightLeg = %v, want true", s.LeftRightBalanceRightLeg)
	}
	if s.LeftTorqueEffectivenessPercent == nil || *s.LeftTorqueEffectivenessPercent != 80 {
		t.Errorf("LeftTorqueEffectivenessPercent = %v, want 80", s.LeftTorqueEffectivenessPercent)
	}
	if s.RightTorqueEffectivenessPercent == nil || *s.RightTorqueEffectivenessPercent != 82 {
		t.Errorf("RightTorqueEffectivenessPercent = %v, want 82", s.RightTorqueEffectivenessPercent)
	}
	if s.LeftPedalSmoothnessPercent == nil || *s.LeftPedalSmoothnessPercent != 20 {
		t.Errorf("LeftPedalSmoothnessPercent = %v, want 20", s.LeftPedalSmoothnessPercent)
	}
	if s.RightPedalSmoothnessPercent == nil || *s.RightPedalSmoothnessPercent != 22 {
		t.Errorf("RightPedalSmoothnessPercent = %v, want 22", s.RightPedalSmoothnessPercent)
	}
	if s.CombinedPedalSmoothnessPercent == nil || *s.CombinedPedalSmoothnessPercent != 30 {
		t.Errorf("CombinedPedalSmoothnessPercent = %v, want 30", s.CombinedPedalSmoothnessPercent)
	}
	if s.GpsAccuracyMeters == nil || *s.GpsAccuracyMeters != 3 {
		t.Errorf("GpsAccuracyMeters = %v, want 3", s.GpsAccuracyMeters)
	}
	if s.Resistance == nil || *s.Resistance != 50 {
		t.Errorf("Resistance = %v, want 50", s.Resistance)
	}
}

func TestParseCorruptedFile(t *testing.T) {
	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	valid := fitfixture.ValidActivity(1, created, 5)
	corrupted := fitfixture.Truncate(valid)

	_, err := fitparse.Parse(corrupted)
	if !errors.Is(err, fitparse.ErrCorrupted) && !errors.Is(err, fitparse.ErrNotFITFile) {
		t.Fatalf("Parse(corrupted) = %v, want ErrCorrupted or ErrNotFITFile", err)
	}
}

func TestParseEmptyFile(t *testing.T) {
	_, err := fitparse.Parse(nil)
	if !errors.Is(err, fitparse.ErrNotFITFile) {
		t.Fatalf("Parse(empty) = %v, want ErrNotFITFile", err)
	}
}

func TestParseGarbageNeverPanics(t *testing.T) {
	garbage := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 'A', 'B', 'C'}
	_, err := fitparse.Parse(garbage)
	if err == nil {
		t.Fatalf("Parse(garbage) = nil error, want an error")
	}
}
