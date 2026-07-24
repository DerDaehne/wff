// Package fitparse decodes .fit activity files (via muktihari/fit) into
// internal structs that map 1:1 onto the activities/samples schema
// (see arch-wff-datenmodell). No DB or HTTP concerns here.
package fitparse

import (
	"bytes"
	"errors"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/filedef"
)

var (
	ErrNotFITFile = errors.New("not a FIT file")
	ErrCorrupted  = errors.New("corrupted FIT file (checksum mismatch)")
	ErrNoActivity = errors.New("FIT file does not contain a complete activity")
)

// FileID identifies the recording device+session, used by the persistence
// layer to derive a dedup key (see ticket "Persistenz: Activity+Samples-Insert").
type FileID struct {
	SerialNumber uint32
	TimeCreated  time.Time
}

type Activity struct {
	FileID FileID

	Sport          string
	StartedAt      time.Time
	ElapsedSeconds int
	MovingSeconds  int

	DistanceMeters      *float64
	ElevationGainMeters *float64

	AvgPowerWatts        *float64
	MaxPowerWatts        *float64
	NormalizedPowerWatts *float64
	AvgHeartRate         *float64
	MaxHeartRate         *float64
	AvgCadence           *float64
	MaxCadence           *float64
	IntensityFactor      *float64
	TrainingStressScore  *float64

	Samples []Sample
}

type Sample struct {
	Time time.Time

	Lat, Lon           *float64
	AltitudeMeters     *float64
	PowerWatts         *int
	HeartRate          *int
	Cadence            *int
	SpeedMps           *float64
	TemperatureCelsius *float64
}

// Parse decodes raw .fit bytes into an Activity. Corrupt, empty, or non-FIT
// input never panics — it always comes back as one of the sentinel errors
// above (checkable via errors.Is).
func Parse(data []byte) (act *Activity, err error) {
	defer func() {
		if r := recover(); r != nil {
			act, err = nil, ErrNotFITFile
		}
	}()

	lis := filedef.NewListener()
	defer lis.Close()

	dec := decoder.New(bytes.NewReader(data), decoder.WithMesgListener(lis))
	if _, decErr := dec.Decode(); decErr != nil {
		switch {
		case errors.Is(decErr, decoder.ErrCRCChecksumMismatch):
			return nil, ErrCorrupted
		default:
			return nil, ErrNotFITFile
		}
	}

	f := lis.File()
	fileActivity, ok := f.(*filedef.Activity)
	if !ok || len(fileActivity.Sessions) == 0 {
		return nil, ErrNoActivity
	}
	session := fileActivity.Sessions[0]
	if session.TotalElapsedTime == basetype.Uint32Invalid {
		return nil, ErrNoActivity
	}

	a := &Activity{
		FileID: FileID{
			SerialNumber: fileActivity.FileId.SerialNumber,
			TimeCreated:  fileActivity.FileId.TimeCreated,
		},
		Sport:          session.Sport.String(),
		StartedAt:      session.StartTime,
		ElapsedSeconds: int(session.TotalElapsedTimeScaled()),
		MovingSeconds:  int(session.TotalTimerTimeScaled()),

		DistanceMeters:       scaledUint32OrNil(session.TotalDistance, session.TotalDistanceScaled()),
		ElevationGainMeters:  uint16OrNil(session.TotalAscent),
		AvgPowerWatts:        uint16OrNil(session.AvgPower),
		MaxPowerWatts:        uint16OrNil(session.MaxPower),
		NormalizedPowerWatts: uint16OrNil(session.NormalizedPower),
		AvgHeartRate:         uint8OrNil(session.AvgHeartRate),
		MaxHeartRate:         uint8OrNil(session.MaxHeartRate),
		AvgCadence:           uint8OrNil(session.AvgCadence),
		MaxCadence:           uint8OrNil(session.MaxCadence),
		IntensityFactor:      scaledUint16OrNil(session.IntensityFactor, session.IntensityFactorScaled()),
		TrainingStressScore:  scaledUint16OrNil(session.TrainingStressScore, session.TrainingStressScoreScaled()),
	}

	a.Samples = make([]Sample, 0, len(fileActivity.Records))
	for _, rec := range fileActivity.Records {
		s := Sample{Time: rec.Timestamp}
		if rec.PositionLat != basetype.Sint32Invalid && rec.PositionLong != basetype.Sint32Invalid {
			lat, lon := rec.PositionLatDegrees(), rec.PositionLongDegrees()
			s.Lat, s.Lon = &lat, &lon
		}
		if rec.Altitude != basetype.Uint16Invalid {
			v := rec.AltitudeScaled()
			s.AltitudeMeters = &v
		}
		if rec.Power != basetype.Uint16Invalid {
			v := int(rec.Power)
			s.PowerWatts = &v
		}
		if rec.HeartRate != basetype.Uint8Invalid {
			v := int(rec.HeartRate)
			s.HeartRate = &v
		}
		if rec.Cadence != basetype.Uint8Invalid {
			v := int(rec.Cadence)
			s.Cadence = &v
		}
		if rec.Speed != basetype.Uint16Invalid {
			v := rec.SpeedScaled()
			s.SpeedMps = &v
		}
		if rec.Temperature != basetype.Sint8Invalid {
			v := float64(rec.Temperature)
			s.TemperatureCelsius = &v
		}
		a.Samples = append(a.Samples, s)
	}

	return a, nil
}

func uint8OrNil(v uint8) *float64 {
	if v == basetype.Uint8Invalid {
		return nil
	}
	f := float64(v)
	return &f
}

func uint16OrNil(v uint16) *float64 {
	if v == basetype.Uint16Invalid {
		return nil
	}
	f := float64(v)
	return &f
}

func scaledUint16OrNil(raw uint16, scaled float64) *float64 {
	if raw == basetype.Uint16Invalid {
		return nil
	}
	return &scaled
}

func scaledUint32OrNil(raw uint32, scaled float64) *float64 {
	if raw == basetype.Uint32Invalid {
		return nil
	}
	return &scaled
}
