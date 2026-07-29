// Package fitfixture builds small, synthetically generated .fit byte streams
// for tests. We deliberately do not check in real device/SDK sample files:
// github.com/muktihari/fit's own testdata/from_official_sdk/* is licensed
// under Garmin's separate FIT SDK License (not BSD-3), so it isn't ours to
// redistribute. Generating our own bytes via the library's (BSD-3) encoder
// carries no such restriction — analogous to using a virtual WebAuthn
// authenticator instead of a real security key in the #551 auth tests.
package fitfixture

import (
	"bytes"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// ValidActivity builds a minimal but protocol-valid FIT activity file:
// FileId + numRecords Records (1 Hz) + Session + Activity summary.
// Every third record omits GPS/altitude/temperature to exercise the
// invalid-value handling path in fitparse.Parse.
func ValidActivity(serialNumber uint32, timeCreated time.Time, numRecords int) []byte {
	fileID := mesgdef.NewFileId(nil)
	fileID.Type = typedef.FileActivity
	fileID.Manufacturer = typedef.ManufacturerDevelopment
	fileID.Product = 1
	fileID.SerialNumber = serialNumber
	fileID.TimeCreated = timeCreated

	messages := []proto.Message{fileID.ToMesg(nil)}

	for i := 0; i < numRecords; i++ {
		rec := mesgdef.NewRecord(nil)
		rec.Timestamp = timeCreated.Add(time.Duration(i) * time.Second)
		rec.Power = uint16(150 + i%50)
		rec.HeartRate = uint8(120 + i%30)
		rec.Cadence = uint8(80 + i%10)
		rec.Speed = uint16(5000 + uint16(i%1000)) // scaled: m/s * 1000
		rec.Calories = uint16(i)
		rec.GpsAccuracy = 3
		rec.Resistance = 50
		rec.LeftTorqueEffectiveness = 160  // scaled /2: 80%
		rec.RightTorqueEffectiveness = 164 // scaled /2: 82%
		rec.LeftPedalSmoothness = 40       // scaled /2: 20%
		rec.RightPedalSmoothness = 44      // scaled /2: 22%
		rec.CombinedPedalSmoothness = 60   // scaled /2: 30%
		rec.LeftRightBalance = typedef.LeftRightBalanceRight | 48 // right leg, 48%
		rec.Grade = 250                                           // scaled /100: 2.5%

		if i%3 != 0 {
			rec.PositionLat = int32(500_000_000 + i)
			rec.PositionLong = int32(100_000_000 + i)
			rec.Altitude = uint16(6000 + i) // scaled: (raw/5)-500 meters
			rec.Temperature = 18
		}
		messages = append(messages, rec.ToMesg(nil))
	}

	endTime := timeCreated.Add(time.Duration(numRecords) * time.Second)

	session := mesgdef.NewSession(nil)
	session.Timestamp = endTime
	session.StartTime = timeCreated
	session.Sport = typedef.SportCycling
	session.TotalElapsedTime = uint32(numRecords * 1000)
	session.TotalTimerTime = uint32(numRecords * 1000)
	session.TotalDistance = uint32(numRecords * 500)
	session.TotalAscent = 42
	session.AvgPower = 180
	session.MaxPower = 300
	session.NormalizedPower = 190
	session.AvgHeartRate = 140
	session.MaxHeartRate = 175
	session.AvgCadence = 85
	session.MaxCadence = 95
	messages = append(messages, session.ToMesg(nil))

	activityMesg := mesgdef.NewActivity(nil)
	activityMesg.Timestamp = endTime
	activityMesg.TotalTimerTime = session.TotalTimerTime
	activityMesg.NumSessions = 1
	activityMesg.Type = typedef.ActivityManual
	messages = append(messages, activityMesg.ToMesg(nil))

	var buf bytes.Buffer
	enc := encoder.New(&buf)
	if err := enc.Encode(&proto.FIT{Messages: messages}); err != nil {
		panic("fitfixture: encode failed: " + err.Error())
	}
	return buf.Bytes()
}

// Truncate corrupts a valid FIT byte stream by cutting off its CRC trailer,
// producing a file that fails checksum validation on decode.
func Truncate(valid []byte) []byte {
	return valid[:len(valid)-4]
}
