package analyze

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Gap is something the app cannot tell the rider yet, together with the ride
// that would fix it (#612).
//
// The point is the instruction. "Dir fehlt ein FTP-Wert" is a dead end for
// someone who does not know what FTP is; "ride 20 minutes as hard as you can
// hold, then upload as usual" is something a person can act on this weekend.
// Nothing here asks them to start a test mode or press anything during the
// ride — the normal upload is the whole protocol.
type Gap struct {
	// Key lets the frontend order and style gaps without parsing German.
	Key string `json:"key"`
	// Unlocks names what the rider gets out of closing it, in their words.
	Unlocks     string `json:"unlocks"`
	Instruction string `json:"instruction"`
}

const (
	GapThreshold = "threshold"
	GapWeight    = "weight"
	GapPerson    = "person"
	GapClimb     = "climb"
	GapSensors   = "sensors"
)

// DataGaps reports what is missing, cheapest to close first: entering a number
// beats riding for it, and riding for it beats buying a sensor.
//
// Silent about anything already covered — a list that keeps nagging about
// things you have done is a list people stop reading.
func DataGaps(ctx context.Context, pool *pgxpool.Pool, userID int64, estimates Estimates) ([]Gap, error) {
	var (
		ftp, lthr, birthYear *int
		weight               *float64
		sex                  *string
		hasPower             bool
		hasHR                bool
		longestGain          *float64
	)

	if err := pool.QueryRow(ctx,
		`SELECT ftp_watts, lthr_bpm, weight_kg, birth_year, sex FROM users WHERE id = $1`, userID,
	).Scan(&ftp, &lthr, &weight, &birthYear, &sex); err != nil {
		return nil, err
	}

	// One pass over the recent rides answers all three data questions.
	// bool_or over no rows is NULL, not false — without the coalesce a rider
	// who has not uploaded anything yet gets an error instead of their list of
	// gaps, which is exactly the moment the list matters most.
	if err := pool.QueryRow(ctx, `
		SELECT
			coalesce(bool_or(normalized_power_watts IS NOT NULL), false),
			coalesce(bool_or(avg_heart_rate IS NOT NULL), false),
			max(elevation_gain_meters)
		FROM activities
		WHERE user_id = $1 AND started_at > now() - make_interval(days => $2)`,
		userID, estimateHistoryDays,
	).Scan(&hasPower, &hasHR, &longestGain); err != nil {
		return nil, err
	}

	var gaps []Gap

	// Cheapest first: a number typed into a form, no riding required.
	if weight == nil {
		gaps = append(gaps, Gap{
			Key:     GapWeight,
			Unlocks: "Wie viel Kraft du am Berg trittst — in Watt, ohne Leistungsmesser.",
			Instruction: "Trag dein Körpergewicht oben ein. Mehr braucht es nicht: aus deinem Tempo " +
				"am Anstieg und deinem Gewicht lässt sich überschlagen, wie viel du getreten hast.",
		})
	}

	// Also a form field, and only worth mentioning to someone who records a
	// pulse — without one the estimate could not be computed anyway.
	if hasHR && (birthYear == nil || sex == nil) {
		gaps = append(gaps, Gap{
			Key:     GapPerson,
			Unlocks: "Wie viele Kalorien eine Fahrt gekostet hat.",
			Instruction: "Trag oben dein Geburtsjahr ein und wähle, mit welcher der beiden Varianten " +
				"der Formel gerechnet werden soll. Der Energieverbrauch aus dem Puls hängt außer am " +
				"Puls selbst am Körpergewicht, am Alter und am Geschlecht — ohne diese Angaben bleibt " +
				"nur Raten, und dann sagt die App lieber nichts.",
		})
	}

	// Only nag about the threshold when neither a stored value nor an estimate
	// from the rides exists — otherwise the profile already offers it.
	if ftp == nil && lthr == nil && estimates.FTPWatts == nil && estimates.LTHRBpm == nil {
		gaps = append(gaps, Gap{
			Key: GapThreshold,
			Unlocks: "Wie hart eine Fahrt für dich war, und damit deine Trainingsbelastung, " +
				"Fitness und Form.",
			Instruction: "Such dir eine Strecke, auf der du 20 Minuten am Stück fahren kannst, ohne " +
				"anhalten zu müssen — leicht ansteigend ist ideal, weil dein Tempo dann von allein " +
				"gleichmäßig bleibt. Fahr so schnell, wie du es gerade noch 20 Minuten durchhältst: " +
				"am Ende soll es richtig wehtun, aber du sollst nicht schon vorher einbrechen. Danach " +
				"lädst du die Fahrt hoch wie immer.",
		})
	}

	// A climb worth the name is what VAM needs; the thresholds are the ones
	// bestClimb uses, so the instruction and the analysis agree.
	if longestGain == nil || *longestGain < minClimbGainMeters {
		gaps = append(gaps, Gap{
			Key:     GapClimb,
			Unlocks: "Deine Kletterrate — die einzige harte Leistungszahl, die ganz ohne Messgerät geht.",
			Instruction: "Nimm einmal eine Strecke mit einem längeren Anstieg mit: mindestens einen " +
				"halben Kilometer am Stück bergauf und rund 30 Höhenmeter. Wie schnell du hochkommst, " +
				"ist über Monate hinweg vergleichbar — anders als dein Schnitt in der Ebene.",
		})
	}

	// Last, because it is the only one that costs money — and it is phrased as
	// information, not as a purchase recommendation.
	if !hasPower && !hasHR {
		gaps = append(gaps, Gap{
			Key:     GapSensors,
			Unlocks: "Eine Einordnung, wie anstrengend eine Fahrt war, statt nur wie schnell.",
			Instruction: "Deine Fahrten enthalten bisher weder Puls noch Leistung. Mit einem Pulsgurt " +
				"oder einer Uhr, die den Puls aufzeichnet, kann die App Anstrengung und Erholung " +
				"beurteilen. Ohne geht vieles trotzdem — Tempo, Höhenprofil und Wind wertet sie so " +
				"oder so aus.",
		})
	}

	return gaps, nil
}
