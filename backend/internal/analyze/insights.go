package analyze

import "fmt"

// Insight is one piece of advice derived from a rider's recent training load.
// Rule-based only — no ML, no training-plan generation (see arch-wff-analyze
// on why that's out of scope).
//
// Action and Reason are separate fields on purpose (#603): a tip only helps
// someone without training knowledge if it says what to DO, and it is only
// trustworthy if it says why. Splitting them makes both halves testable and
// lets the frontend give the instruction its own visual weight.
//
// An Insight with an empty Action is not advice but an honest note that the
// data doesn't support any yet. Severity drives icon and colour.
type Insight struct {
	Action   string `json:"action"`
	Reason   string `json:"reason"`
	Severity string `json:"severity"` // "info" | "success" | "warning"
}

// minHistoryDaysForInsights is how far back the CTL-trend rules look. This
// counts CALENDAR DAYS since the rider's first ride, not number of rides —
// several rides uploaded on the same day (a very normal way to try the app
// for the first time) still count as one day of history.
const minHistoryDaysForInsights = 7

// freshTooLongDays — being rested is an advantage before something hard and a
// problem as a permanent state. Only worth saying once it has clearly lasted.
const freshTooLongDays = 10

// Insights derives concrete, explainable advice from a TrainingLoad series.
//
// Deliberately silent rather than inventive: with too little history it
// returns the reason for waiting and no instruction at all. Advice built on
// four days of data would be confident and wrong, which is worse than none —
// especially for this audience, who has no way to sanity-check it (#600).
func Insights(series []DayLoad) []Insight {
	if len(series) < minHistoryDaysForInsights {
		return []Insight{{
			Severity: "info",
			Reason: fmt.Sprintf(
				"Für einen Rat zu deinem Training ist deine Historie noch zu kurz (%d von %d Tagen seit "+
					"der ersten Fahrt). Fahr einfach weiter wie bisher — sobald genug zusammengekommen ist, "+
					"wird es hier konkret.",
				len(series), minHistoryDaysForInsights),
		}}
	}

	latest := series[len(series)-1]
	weekAgo := series[len(series)-minHistoryDaysForInsights]

	var insights []Insight

	if latest.TSB < -30 {
		insights = append(insights, Insight{
			Severity: "warning",
			Action: "Nimm dir die nächsten zwei bis drei Tage bewusst locker: kurze Runden im Tempo, " +
				"in dem du dich noch unterhalten könntest — oder gar nicht fahren.",
			Reason: fmt.Sprintf(
				"Deine Belastung der letzten Tage liegt weit über dem, was du gewohnt bist (Frische %s). "+
					"Fitness entsteht in der Erholung nach dem Reiz, nicht im Reiz selbst — ohne diese Tage "+
					"wird daraus keine Form, sondern Erschöpfung.", signed(latest.TSB)),
		})
	}

	if latest.TSB > 15 && latest.CTL >= weekAgo.CTL {
		insights = append(insights, Insight{
			Severity: "success",
			Action: "Wenn du etwas Anspruchsvolles vorhast — eine lange Tour, deinen Hausberg, einen " +
				"Wettkampf — dann in den nächsten Tagen.",
			Reason: "Du bist erholt und deine Fitness ist zuletzt nicht gefallen. Besser wird die " +
				"Ausgangslage für einen harten Tag nicht.",
		})
	}

	if fresh := freshFor(series); fresh >= freshTooLongDays {
		insights = append(insights, Insight{
			Severity: "info",
			Action:   "Fahr wieder regelmäßiger — lieber dreimal kurz in der Woche als einmal lang.",
			Reason: fmt.Sprintf(
				"Du bist seit %d Tagen durchgehend gut erholt. Das klingt gut, heißt aber auch: du "+
					"trainierst gerade zu wenig, um deine Fitness zu halten.", fresh),
		})
	}

	if weekAgo.CTL > 0 {
		if drop := (weekAgo.CTL - latest.CTL) / weekAgo.CTL; drop > 0.10 {
			insights = append(insights, Insight{
				Severity: "warning",
				Action: "Setz dir für die nächsten zwei Wochen zwei bis drei feste Termine, an denen du " +
					"fährst — auch wenn es nur eine Stunde ist.",
				Reason: fmt.Sprintf(
					"Dein Trainingsumfang ist in einer Woche um %d %% gesunken. Fitness fällt langsamer, "+
						"als sie steigt, aber sie fällt — regelmäßig kurz zu fahren hält mehr davon als eine "+
						"einzelne lange Ausfahrt.", int(drop*100+0.5)),
			})
		}
	}

	if len(insights) == 0 {
		insights = append(insights, Insight{
			Severity: "info",
			Action: "Wenn du aufbauen willst, mach eine Fahrt pro Woche 15 bis 20 Minuten länger — " +
				"nicht härter, nur länger.",
			Reason: "Belastung und Erholung halten sich bei dir gerade die Waage. Das ist ein gesunder " +
				"Zustand, aber ohne einen etwas größeren Reiz bleibt die Fitness, wo sie ist.",
		})
	}
	return insights
}

// freshFor counts how many days in a row the rider has been clearly rested.
// A single rest day says nothing; a fortnight of them is the actual message.
func freshFor(series []DayLoad) int {
	days := 0
	for i := len(series) - 1; i >= 0; i-- {
		if series[i].TSB <= 15 {
			break
		}
		days++
	}
	return days
}
