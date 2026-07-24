package analyze

// Insight is a single, human-readable tip derived from a rider's recent
// training load. Rule-based only — no ML, no training-plan generation (see
// arch-wff-analyze on why that's out of scope).
type Insight struct {
	Message string `json:"message"`
}

// minHistoryDaysForInsights is how far back the CTL-trend rules look.
const minHistoryDaysForInsights = 7

// Insights derives simple, explainable tips from a TrainingLoad series.
// Always returns at least one insight: a generic status message when there
// isn't enough history yet for the threshold rules below, or when none of
// them fire (the acceptance criterion is "at least one tip", not "only
// when something's wrong").
func Insights(series []DayLoad) []Insight {
	if len(series) < minHistoryDaysForInsights {
		return []Insight{{Message: "Noch nicht genug Trainingshistorie für eine Einschätzung — nach ein paar weiteren Rides gibt's hier mehr."}}
	}

	latest := series[len(series)-1]
	weekAgo := series[len(series)-minHistoryDaysForInsights]

	var insights []Insight

	if latest.TSB < -30 {
		insights = append(insights, Insight{Message: "Hohe Ermüdung — Erholung einplanen."})
	}
	if latest.TSB > 15 && latest.CTL >= weekAgo.CTL {
		insights = append(insights, Insight{Message: "Gute Form — guter Zeitpunkt für einen harten Effort oder ein Rennen."})
	}
	if weekAgo.CTL > 0 && (latest.CTL-weekAgo.CTL)/weekAgo.CTL < -0.10 {
		insights = append(insights, Insight{Message: "Trainingsumfang sinkt spürbar — Fitness könnte in den nächsten Wochen abgebaut werden."})
	}

	if len(insights) == 0 {
		insights = append(insights, Insight{Message: "Form im normalen Bereich — weiter so."})
	}
	return insights
}
