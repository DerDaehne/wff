package analyze

import (
	"fmt"
	"math"
)

// trendWindowDays is how far back the "is my fitness going up or down?"
// comparison looks. Four weeks: long enough that a single hard weekend doesn't
// swing it, short enough to still be about now.
const trendWindowDays = 28

// minDaysForTrend — below this there is no trend, only noise. Two weeks of
// history cannot say whether fitness is building.
const minDaysForTrend = 14

// TrainingStatus answers "how am I doing?" in plain language, reusing the same
// Story shape as a single ride (#601): a title that states the situation,
// headline figures, and statements that explain them.
//
// The technical names stay, but in brackets behind the plain one — a rider who
// looks CTL up elsewhere should still recognise it (#600, guideline 1).
func TrainingStatus(series []DayLoad) Story {
	if len(series) == 0 {
		return Story{}
	}
	latest := series[len(series)-1]

	story := Story{
		Title: formTitle(latest.TSB),
		Gauge: trainingLevel(series),
		Stats: []Stat{
			{Value: fmt.Sprintf("%d", int(latest.CTL+0.5)), Label: "Fitness (CTL)"},
			{Value: fmt.Sprintf("%d", int(latest.ATL+0.5)), Label: "Müdigkeit (ATL)"},
			{Value: signed(latest.TSB), Label: "Frische (TSB)"},
		},
	}

	story.Statements = append(story.Statements, Statement{
		Text:    formExplanation(latest.TSB),
		Metric:  fmt.Sprintf("Frische (TSB) %s", signed(latest.TSB)),
		Metrics: []Stat{{Value: signed(latest.TSB), Label: "Frische (TSB)"}},
		Kind:    "form",
	})

	if s, ok := trendStatement(series); ok {
		story.Statements = append(story.Statements, s)
	}
	return story
}

// trainingLevels translate CTL into something a rider can place themselves in.
// CTL already IS the "training level score" — it just has no name and no scale,
// so 41 says nothing about whether that is a lot.
//
// The boundaries are not invented. They follow the CTL ranges commonly cited
// for weekly training volume: roughly 30–50 for recreational riders at 3–5 h a
// week, 60–90 for committed amateurs at 8–12 h, 80–110 for club racers, 100–140
// for elite amateurs, 140+ for professionals. Rounded to memorable numbers,
// because the exact cut is far less meaningful than the band.
var trainingLevels = []struct {
	upTo    float64
	name    string
	caption string
}{
	{20, "Einstieg", "ein paar Fahrten in den letzten Wochen"},
	{40, "Gelegenheitsfahrer", "etwa 3 bis 5 Stunden pro Woche"},
	{70, "Regelmäßig im Sattel", "etwa 6 bis 8 Stunden pro Woche"},
	{100, "Ambitioniert", "etwa 8 bis 12 Stunden pro Woche"},
	{math.MaxFloat64, "Wettkampfniveau", "so viel wie ambitionierte Rennfahrer"},
}

// trainingLevel turns CTL into a named band plus how far through it you are.
// Nil below minDaysForTrend: with a week of history the number describes the
// last few days, not a training level.
func trainingLevel(series []DayLoad) *Gauge {
	if len(series) < minDaysForTrend {
		return nil
	}
	ctl := series[len(series)-1].CTL

	lower := 0.0
	for _, level := range trainingLevels {
		if ctl < level.upTo {
			// Progress through the current band. The open-ended top band has no
			// "through" to speak of, so it simply reads full.
			percent := 100
			if level.upTo != math.MaxFloat64 {
				percent = int((ctl - lower) / (level.upTo - lower) * 100)
			}
			return &Gauge{
				Percent: min(max(percent, 0), 100),
				Label:   fmt.Sprintf("Trainingsniveau: %s", level.name),
				Caption: level.caption,
			}
		}
		lower = level.upTo
	}
	return nil
}

// formTitle states the situation. Bands follow the usual TSB reading: clearly
// positive means rested, clearly negative means deep in a training block.
func formTitle(tsb float64) string {
	switch {
	case tsb > 15:
		return "Du bist frisch und ausgeruht"
	case tsb > 5:
		return "Du bist gut erholt"
	case tsb > -10:
		return "Du bist im normalen Bereich"
	case tsb > -30:
		return "Du steckst in einer Belastungsphase"
	default:
		return "Du bist deutlich ermüdet"
	}
}

func formExplanation(tsb float64) string {
	switch {
	case tsb > 15:
		return "Deine Beine sind ausgeruht — ein guter Zeitpunkt für eine harte Einheit, " +
			"einen langen Ausflug oder ein Rennen. Länger in diesem Zustand zu bleiben baut allerdings " +
			"auch Fitness ab."
	case tsb > 5:
		return "Du hast dich von den letzten Einheiten erholt und kannst wieder Gas geben, " +
			"ohne dass es dir schwerfallen sollte."
	case tsb > -10:
		return "Belastung und Erholung halten sich gerade die Waage. Genau in diesem Bereich " +
			"wird Ausdauer über Wochen aufgebaut, ohne dass man sich dauernd platt fühlt."
	case tsb > -30:
		return "Du hast zuletzt mehr trainiert als du bisher gewohnt warst. Das ist genau der Reiz, " +
			"der fitter macht — aber nur, wenn du ihm auch ruhige Tage folgen lässt."
	default:
		return "Die Belastung der letzten Tage liegt deutlich über dem, was du gewohnt bist. " +
			"So etwas hält man nicht lange durch: plan die nächsten Tage ruhig, sonst wird aus dem " +
			"Trainingsreiz Erschöpfung."
	}
}

// trendStatement is the answer to "werde ich besser?" — the question the whole
// app exists for. Silent below minDaysForTrend rather than reading a trend into
// two weeks of data.
func trendStatement(series []DayLoad) (Statement, bool) {
	if len(series) < minDaysForTrend {
		return Statement{
			Text: fmt.Sprintf(
				"Ob deine Fitness steigt oder fällt, lässt sich noch nicht sagen — dafür braucht es "+
					"mindestens %d Tage Historie, du hast bisher %d. Das kommt mit der Zeit von allein, "+
					"nicht durch mehr Fahrten auf einmal.",
				minDaysForTrend, len(series)),
			Kind: "hint_history",
		}, true
	}

	latest := series[len(series)-1]
	back := series[max(0, len(series)-1-trendWindowDays)]
	days := len(series) - 1 - max(0, len(series)-1-trendWindowDays)

	// An absolute floor as well as a relative one: going from CTL 2 to 3 is
	// +50 % and means nothing.
	delta := latest.CTL - back.CTL
	relative := 0.0
	if back.CTL > 0 {
		relative = delta / back.CTL
	}

	var text string
	switch {
	case delta >= 3 && relative >= 0.08:
		text = fmt.Sprintf("Deine Fitness ist in den letzten %d Tagen gestiegen — das Training wirkt.", days)
	case delta <= -3 && relative <= -0.08:
		text = fmt.Sprintf("Deine Fitness ist in den letzten %d Tagen gesunken. Das passiert nach einer "+
			"Pause ganz normal und kommt mit regelmäßigen Fahrten zurück.", days)
	default:
		text = fmt.Sprintf("Deine Fitness ist in den letzten %d Tagen etwa gleich geblieben. "+
			"Zum Aufbauen brauchst du entweder längere oder härtere Einheiten.", days)
	}

	return Statement{
		Text:   text,
		Metric: fmt.Sprintf("Fitness (CTL) %d → %d", int(back.CTL+0.5), int(latest.CTL+0.5)),
		Metrics: []Stat{
			{Value: fmt.Sprintf("%d", int(back.CTL+0.5)), Label: fmt.Sprintf("Vor %d Tagen", days)},
			{Value: fmt.Sprintf("%d", int(latest.CTL+0.5)), Label: "Jetzt"},
		},
		Kind: "trend",
	}, true
}

// signed keeps the plus sign on positive form values — "+12" reads as a
// direction, "12" reads as a quantity.
func signed(v float64) string {
	if v > 0 {
		return fmt.Sprintf("+%d", int(v+0.5))
	}
	return fmt.Sprintf("%d", int(v-0.5))
}
