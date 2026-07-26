package analyze

import "fmt"

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
		Stats: []Stat{
			{Value: fmt.Sprintf("%d", int(latest.CTL+0.5)), Label: "Fitness (CTL)"},
			{Value: fmt.Sprintf("%d", int(latest.ATL+0.5)), Label: "Müdigkeit (ATL)"},
			{Value: signed(latest.TSB), Label: "Frische (TSB)"},
		},
	}

	story.Statements = append(story.Statements, Statement{
		Text:   formExplanation(latest.TSB),
		Metric: fmt.Sprintf("Frische (TSB) %s", signed(latest.TSB)),
		Kind:   "form",
	})

	if s, ok := trendStatement(series); ok {
		story.Statements = append(story.Statements, s)
	}
	return story
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
		Kind:   "trend",
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
