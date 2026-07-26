package analyze

import (
	"fmt"
	"sort"
	"strings"
)

// Story is a single ride told in plain language, for riders without training
// science background (#600/#601): every statement leads with what it MEANS,
// and carries the technical value only as a small secondary label.
//
// This lives in the backend, not the frontend, so that the wording and the
// thresholds behind it have exactly one home — the same place that already
// computes IF/TSS (see arch-wff-analyze) and the training-load insights.
type Story struct {
	Headline   string      `json:"headline"`
	Statements []Statement `json:"statements"`
}

// Statement is one plain-language sentence plus the number it came from.
// Kind lets the frontend label and illustrate statements without re-deriving
// meaning from the text: what kind of session (effort), what it did to you
// (load), why it felt the way it did (context), how it compares
// (comparison). The hint_* kinds say what the app CAN'T tell yet and why —
// split by cause so the frontend can offer the right way out (a link to the
// profile vs. "keep riding") without sniffing the German text.
type Statement struct {
	Text   string `json:"text"`
	Metric string `json:"metric,omitempty"`
	Kind   string `json:"kind"`
}

// RideFacts is everything RideStory reasons about. Pointers are genuinely
// optional data (no GPS, no power meter, not enriched yet) — a nil field must
// never turn into a made-up statement.
type RideFacts struct {
	DistanceMeters      *float64
	ElapsedSeconds      int
	MovingSeconds       int
	ElevationGainMeters *float64
	IntensityFactor     *float64
	TSS                 *float64
	// FromPower distinguishes the power path from the heart-rate estimate
	// (#563 vs. #564) — an HR-derived number is honest about being an
	// estimate rather than silently presented as measured intensity.
	FromPower          bool
	HeadwindMps        *float64
	TemperatureCelsius *float64
	// PriorTSS are the load values of this rider's earlier rides. Comparison
	// statements stay silent below minRidesForComparison instead of calling a
	// ride "harder than usual" when there is no usual yet.
	PriorTSS []float64
}

// minRidesForComparison — with fewer earlier rides than this, "compared to
// your recent rides" is noise, not information.
const minRidesForComparison = 3

// RideStory turns one ride's numbers into plain-language statements.
func RideStory(f RideFacts) Story {
	story := Story{Headline: headline(f)}

	if s, ok := effortStatement(f); ok {
		story.Statements = append(story.Statements, s)
	}
	if s, ok := loadStatement(f); ok {
		story.Statements = append(story.Statements, s)
	}
	story.Statements = append(story.Statements, contextStatements(f)...)
	if s, ok := comparisonStatement(f); ok {
		story.Statements = append(story.Statements, s)
	}
	return story
}

// sessionKind names the type of session from intensity alone. Bands follow
// the established IF zones (Coggan): below ~0.6 recovery, 0.6–0.75 endurance,
// 0.75–0.85 tempo, 0.85–0.95 threshold, above that race/VO2max territory.
func sessionKind(intensityFactor *float64) string {
	if intensityFactor == nil {
		return "Ausfahrt"
	}
	switch f := *intensityFactor; {
	case f < 0.60:
		return "Erholungsfahrt"
	case f < 0.75:
		return "Grundlagenfahrt"
	case f < 0.85:
		return "zügige Tempofahrt"
	case f < 0.95:
		return "harte Einheit"
	default:
		return "sehr harte Einheit"
	}
}

func headline(f RideFacts) string {
	kind := sessionKind(f.IntensityFactor)
	kind = strings.ToUpper(kind[:1]) + kind[1:]
	if f.DistanceMeters != nil && *f.DistanceMeters > 0 {
		return fmt.Sprintf("%s über %s km, %s", kind, decimal(*f.DistanceMeters/1000, 1), duration(f.ElapsedSeconds))
	}
	return fmt.Sprintf("%s, %s", kind, duration(f.ElapsedSeconds))
}

func effortStatement(f RideFacts) (Statement, bool) {
	if f.IntensityFactor == nil {
		return Statement{
			Text: "Wie anstrengend diese Fahrt für dich war, kann die App noch nicht einordnen — " +
				"dafür fehlt dein FTP-Wert (Leistung) oder deine Schwellen-Herzfrequenz im Profil.",
			Kind: "hint_profile",
		}, true
	}

	var text string
	switch f := *f.IntensityFactor; {
	case f < 0.60:
		text = "Das war locker — ein Tempo, das du sehr lange durchhalten könntest."
	case f < 0.75:
		text = "Ein gleichmäßiges, gut haltbares Tempo — die Intensität, in der Ausdauer aufgebaut wird."
	case f < 0.85:
		text = "Zügig unterwegs: anstrengend, aber noch weit vom Anschlag entfernt."
	case f < 0.95:
		text = "Hart: nahe an dem Tempo, das du gerade noch eine Stunde durchhalten kannst."
	default:
		text = "Sehr hart — Wettkampf- oder Intervalltempo, das sich nicht lange durchhalten lässt."
	}

	source := "aus deiner Leistung"
	if !f.FromPower {
		source = "aus deinem Puls geschätzt"
	}
	return Statement{
		Text:   text,
		Metric: fmt.Sprintf("Intensität (IF) %s · %s", decimal(*f.IntensityFactor, 2), source),
		Kind:   "effort",
	}, true
}

// loadStatement translates TSS into recovery language. Bands are the widely
// used single-ride TSS guidance: up to ~50 low, to ~100 medium (recovered by
// the next day), to ~200 high (residual fatigue the day after), above that
// very high (two days or more).
func loadStatement(f RideFacts) (Statement, bool) {
	if f.TSS == nil {
		return Statement{}, false
	}

	var text string
	switch t := *f.TSS; {
	case t < 50:
		text = "Als Trainingsreiz war das klein — morgen bist du wieder frisch."
	case t < 100:
		text = "Ein solider Trainingsreiz, den du über Nacht wegsteckst."
	case t < 200:
		text = "Ordentliche Belastung — morgen wirst du sie in den Beinen spüren."
	case t < 300:
		text = "Große Belastung: plane ein bis zwei ruhige Tage danach ein."
	default:
		text = "Sehr große Belastung — so etwas braucht mehrere Tage Erholung."
	}
	return Statement{
		Text:   text,
		Metric: fmt.Sprintf("Belastung (TSS) %d", int(*f.TSS+0.5)),
		Kind:   "load",
	}, true
}

// contextStatements explain why a ride felt harder or easier than the bare
// numbers suggest. Only clearly noticeable conditions get a sentence —
// mentioning 0.2 m/s of wind would be noise.
func contextStatements(f RideFacts) []Statement {
	var out []Statement

	if f.HeadwindMps != nil && absf(*f.HeadwindMps) >= 1.0 {
		wind := *f.HeadwindMps
		text := "Im Schnitt hattest du Gegenwind — der kostet Tempo bei gleicher Anstrengung, " +
			"deine Geschwindigkeit sagt an so einem Tag weniger über deine Form aus."
		if wind < 0 {
			text = "Im Schnitt hattest du Rückenwind — dadurch fielen die Zeiten leichter als die Anstrengung vermuten lässt."
		}
		out = append(out, Statement{
			Text:   text,
			Metric: fmt.Sprintf("⌀ %s m/s %s", decimal(absf(wind), 1), windLabel(wind)),
			Kind:   "context",
		})
	}

	if gain := f.ElevationGainMeters; gain != nil && *gain >= 300 {
		out = append(out, Statement{
			Text: "Die Strecke war hügelig. Bergauf steigt die Anstrengung stark an, " +
				"auch wenn die Durchschnittsgeschwindigkeit dadurch niedriger aussieht.",
			Metric: fmt.Sprintf("%d Höhenmeter", int(*gain+0.5)),
			Kind:   "context",
		})
	}

	if t := f.TemperatureCelsius; t != nil && (*t >= 27 || *t <= 5) {
		text := "Es war heiß. Bei Hitze arbeitet der Kreislauf zusätzlich gegen die Wärme — " +
			"der Puls liegt höher als sonst bei gleichem Tempo."
		if *t <= 5 {
			text = "Es war kalt. In der Kälte kommt der Kreislauf langsamer in Gang, " +
				"der Puls bleibt am Anfang oft niedriger als die Anstrengung."
		}
		out = append(out, Statement{
			Text:   text,
			Metric: fmt.Sprintf("⌀ %d °C", int(*t+0.5)),
			Kind:   "context",
		})
	}

	return out
}

func comparisonStatement(f RideFacts) (Statement, bool) {
	if f.TSS == nil {
		return Statement{}, false
	}
	if len(f.PriorTSS) < minRidesForComparison {
		return Statement{
			Text: fmt.Sprintf(
				"Für einen Vergleich mit deinen sonstigen Fahrten sind es noch zu wenige (%d bisher). "+
					"Ab ein paar Fahrten mehr kann die App einordnen, ob eine Einheit für dich hart oder normal war.",
				len(f.PriorTSS)),
			Kind: "hint_history",
		}, true
	}

	median := medianOf(f.PriorTSS)
	if median <= 0 {
		return Statement{}, false
	}

	ratio := *f.TSS / median
	var text string
	switch {
	case ratio >= 1.3:
		text = "Deutlich anstrengender als deine üblichen Fahrten."
	case ratio <= 0.7:
		text = "Ruhiger als deine üblichen Fahrten."
	default:
		text = "Das liegt im Rahmen deiner üblichen Fahrten."
	}
	return Statement{
		Text:   text,
		Metric: fmt.Sprintf("Belastung sonst im Mittel %d", int(median+0.5)),
		Kind:   "comparison",
	}, true
}

func windLabel(headwindMps float64) string {
	if headwindMps < 0 {
		return "Rückenwind"
	}
	return "Gegenwind"
}

func medianOf(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// decimal formats with a German decimal comma — these strings go straight
// into the UI, so formatting them at the source avoids every caller
// re-implementing it.
func decimal(v float64, digits int) string {
	return strings.Replace(fmt.Sprintf("%.*f", digits, v), ".", ",", 1)
}

func duration(seconds int) string {
	h, m := seconds/3600, (seconds%3600)/60
	if h > 0 {
		return fmt.Sprintf("%d:%02d h", h, m)
	}
	return fmt.Sprintf("%d min", m)
}
