package analyze

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Story is a single ride told in plain language, for riders without training
// science background (#600/#601): every statement leads with what it MEANS,
// and carries the technical value only as a small secondary label.
//
// This lives in the backend, not the frontend, so that the wording and the
// thresholds behind it have exactly one home — the same place that already
// computes IF/TSS (see arch-wff-analyze) and the training-load insights.
type Story struct {
	// Title is what kind of ride this was ("Zügige Tempofahrt") — the meaning,
	// not the rubric.
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	// Stats are the two or three figures that lead the page. Pre-formatted
	// here so the frontend never re-implements German number formatting, and
	// split into value/unit so it can typeset the number large and the unit
	// small (#607).
	Stats []Stat `json:"stats"`
	// DetailStats is every headline-style figure this ride has data for, in a
	// fixed order — the data-dense ride-detail grid, unlike Stats above,
	// doesn't curate down to 2-3 (nil on the dashboard story, which has no
	// single ride to detail).
	DetailStats []Stat `json:"detail_stats,omitempty"`
	// Gauge is the one bar this view shows: ride intensity on a ride,
	// training level on the dashboard. Nil when there is nothing to show one
	// for.
	Gauge      *Gauge      `json:"gauge,omitempty"`
	Statements []Statement `json:"statements"`
	// Zones is the ride's time per heart-rate band, for the bar that shows how
	// the effort was actually spread (#621). Absent on the dashboard story and
	// on rides without a threshold heart rate.
	Zones *ZoneDistribution `json:"zones,omitempty"`
}

// Stat is one headline figure: "42,3" + "km" + "Distanz".
type Stat struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
	Label string `json:"label"`
}

// Gauge is a percentage meant to be drawn as a bar. Percent is clamped to
// 0..100 for the bar; Label carries the real, unclamped reading, because an
// intensity above the hour-power threshold is exactly what a rider wants to
// see rather than a bar pinned at full.
type Gauge struct {
	Percent int    `json:"percent"`
	Label   string `json:"label"`
	Caption string `json:"caption"`
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
	// Metrics is the number(s) behind Metric, structured so the frontend can
	// typeset them large instead of showing the pre-formatted string (#651).
	// Left empty for statements whose Metric is a genuine multi-part
	// narrative (e.g. a four-way zone split) rather than one or two values —
	// the frontend falls back to Metric text there, not to nothing.
	Metrics []Stat `json:"metrics,omitempty"`
	Kind    string `json:"kind"`
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
	// PriorSpeedsKmh are earlier rides' average speeds — the only baseline
	// available to a rider with neither a power meter nor a heart-rate strap.
	PriorSpeedsKmh []float64
	// Course is what the route itself says (speed, terrain, wind per heading).
	// Nil when the ride has no usable GPS. Everything derived from it works
	// without power or heart rate — see #606.
	Course *CourseStats
	// StartedAt drives the subtitle. Zero time simply means no subtitle.
	StartedAt time.Time
	// WeightKg turns climbing speed into a relative-power estimate (#610).
	// Nil simply means that part goes unsaid.
	WeightKg *float64
	// PrimaryMetric is the figure this rider wants to lead with (#616). Empty
	// keeps the previous order.
	PrimaryMetric string
	// Endurance is how well output per heartbeat held up across the ride
	// (#613). Nil whenever the ride was too short, too hard or too ragged for
	// the number to describe anything.
	Endurance *Efficiency
	// Zones is the time spent in each heart-rate band (#621). Nil without a
	// threshold heart rate, or when too little pulse was recorded.
	Zones *ZoneDistribution
	// AvgHeartRate, BirthYear and Sex are what an energy estimate costs on top
	// of the weight (#625). Any of them missing means no calorie figure.
	AvgHeartRate *float64
	BirthYear    *int
	Sex          *string
	// Milestones are the rider's best-ever distance/climbing figures from
	// before this ride (#636) — nil fields mean nothing to compare against.
	Milestones MilestoneFacts
}

// ageAtRide is the rider's age in the season the ride happened, not today. A
// stored age would go stale; a stored birth year does not.
func (f RideFacts) ageAtRide() (int, bool) {
	return ageAt(f.StartedAt, f.BirthYear)
}

// ageAt is the rider's age at a given moment, from their birth year rather
// than a stored age so it never goes stale.
func ageAt(t time.Time, birthYear *int) (int, bool) {
	if birthYear == nil || *birthYear <= 0 || t.IsZero() {
		return 0, false
	}
	age := t.Year() - *birthYear
	// A plausibility window, not a judgement: it catches a typo like 1900 or a
	// year in the future, both of which would produce a confident wrong number.
	if age < 10 || age > 100 {
		return 0, false
	}
	return age, true
}

// minRidesForComparison — with fewer earlier rides than this, "compared to
// your recent rides" is noise, not information.
const minRidesForComparison = 3

// distanceMeters is the ride's length: the device's own figure when it recorded
// one, the GPS track only as a fallback. The device value comes from a wheel
// sensor where present and survives GPS dropouts, so it is the number the rider
// also sees on the head unit.
func (f RideFacts) distanceMeters() float64 {
	if f.DistanceMeters != nil && *f.DistanceMeters > 0 {
		return *f.DistanceMeters
	}
	if f.Course != nil {
		return f.Course.DistanceMeters
	}
	return 0
}

// avgSpeedKmh over moving time, not elapsed — coffee stops are not slow riding.
func (f RideFacts) avgSpeedKmh() float64 {
	seconds := f.MovingSeconds
	if seconds <= 0 {
		seconds = f.ElapsedSeconds
	}
	if seconds <= 0 {
		return 0
	}
	return f.distanceMeters() / float64(seconds) * 3.6
}

// metersPerKm is how much climbing there was per kilometre — the number that
// separates "flat" from "hilly" independently of ride length.
func (f RideFacts) metersPerKm() float64 {
	km := f.distanceMeters() / 1000
	gain := 0.0
	switch {
	case f.ElevationGainMeters != nil:
		gain = *f.ElevationGainMeters
	case f.Course != nil:
		gain = f.Course.ElevationGainMeters
	}
	if km <= 0 || gain <= 0 {
		return 0
	}
	return gain / km
}

// RideStory turns one ride's numbers into plain-language statements.
//
// Hints ("set your FTP", "not enough rides to compare yet") are collected and
// appended last no matter where they arise: a rider opening a ride wants to read
// what IS known first, not an apology for what isn't.
func RideStory(f RideFacts) Story {
	story := Story{
		Title:       rideTitle(f),
		Subtitle:    germanDate(f.StartedAt),
		Stats:       headlineStats(f),
		DetailStats: detailStats(f),
		Gauge:       intensityGauge(f),
	}
	var hints []Statement

	add := func(s Statement, ok bool) {
		switch {
		case !ok:
		case strings.HasPrefix(s.Kind, "hint_"):
			hints = append(hints, s)
		default:
			story.Statements = append(story.Statements, s)
		}
	}

	add(effortStatement(f))
	add(loadStatement(f))
	add(paceStatement(f))
	add(climbStatement(f))
	// Decoupling compares the two halves of a steady ride. On the power side an
	// interval session is caught by NP/avg, but a pulse-only ride has no such
	// figure — so the zone split is what disqualifies it here, or the drift
	// between two halves that were different workouts gets reported as a
	// verdict on endurance (#630).
	if f.Endurance != nil && !f.isMixed() {
		add(efficiencyStatement(*f.Endurance), true)
	}
	if kcal, age, ok := f.calories(); ok {
		add(caloriesStatement(kcal, age), true)
	}
	if f.Zones != nil {
		story.Zones = f.Zones
		for _, s := range f.Zones.Statements {
			add(s, true)
		}
	}
	for _, s := range contextStatements(f) {
		add(s, true)
	}
	add(comparisonStatement(f))
	for _, s := range milestoneStatements(f) {
		add(s, true)
	}

	story.Statements = append(story.Statements, hints...)
	return story
}

// sessionKind names the type of session from intensity alone. Bands follow
// the established IF zones (Coggan): below ~0.6 recovery, 0.6–0.75 endurance,
// 0.75–0.85 tempo, 0.85–0.95 threshold, above that race/VO2max territory.
// effortBands are the five ways a ride gets described, easiest first: what to
// call it, and the sentence that explains it. One list, because a ride's title
// and its effort card must never disagree.
var effortBands = []struct{ session, text string }{
	{"Erholungsfahrt", "Das war locker — ein Tempo, das du sehr lange durchhalten könntest."},
	{"Grundlagenfahrt", "Ein gleichmäßiges, gut haltbares Tempo — die Intensität, in der Ausdauer aufgebaut wird."},
	{"zügige Tempofahrt", "Zügig unterwegs: anstrengend, aber noch weit vom Anschlag entfernt."},
	{"harte Einheit", "Hart: nahe an dem Tempo, das du gerade noch eine Stunde durchhalten kannst."},
	{"sehr harte Einheit", "Sehr hart — Wettkampf- oder Intervalltempo, das sich nicht lange durchhalten lässt."},
}

// powerEffortEdges are the band edges for an intensity read off power.
var powerEffortEdges = []float64{0.60, 0.75, 0.85, 0.95}

// effortBandFor picks the band, and the scale decides how (#630). An easy hour
// is IF 0.65 on power but 0.88 on pulse — a resting heart already beats at
// roughly 40 % of threshold, so the pulse scale is compressed into its top
// half. Reading pulse off the power edges called every base ride hard. The
// pulse side uses the zone table so the ride's headline and its zone chart
// cannot contradict each other.
func effortBandFor(intensityFactor float64, fromPower bool) int {
	if !fromPower {
		return zoneForRatio(intensityFactor)
	}
	band := 0
	for _, edge := range powerEffortEdges {
		if intensityFactor >= edge {
			band++
		}
	}
	return band
}

// isMixed reports a ride that spent a real part of itself above threshold. Its
// average falls in a band that describes neither the hard part nor the easy
// one, so naming that band would be precise about the wrong thing.
func (f RideFacts) isMixed() bool {
	if f.Zones == nil {
		return false
	}
	_, _, hard := f.Zones.shares()
	return hard >= zoneMixedHardShare
}

func sessionKind(f RideFacts) string {
	if f.IntensityFactor == nil {
		return "Ausfahrt"
	}
	if f.isMixed() {
		return "Intervallfahrt"
	}
	return effortBands[effortBandFor(*f.IntensityFactor, f.FromPower)].session
}

func rideTitle(f RideFacts) string {
	kind := sessionKind(f)
	// Without an intensity factor the generic "Ausfahrt" is all the metrics
	// allow — but the profile still says something worth putting in the title.
	if f.IntensityFactor == nil && f.metersPerKm() >= 10 {
		kind = "hügelige Ausfahrt"
	}
	return strings.ToUpper(kind[:1]) + kind[1:]
}

var germanWeekdays = [...]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}

var germanMonths = [...]string{
	"Januar", "Februar", "März", "April", "Mai", "Juni",
	"Juli", "August", "September", "Oktober", "November", "Dezember",
}

// germanDate spells the ride's date out. Go's time package has no locales, and
// pulling in a localisation library to render one line would be absurd.
func germanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	local := t.Local()
	return fmt.Sprintf("%s, %d. %s · %02d:%02d",
		germanWeekdays[int(local.Weekday())], local.Day(), germanMonths[int(local.Month())-1],
		local.Hour(), local.Minute())
}

// PrimaryMetric names the figure a rider wants to see first (#616). The
// values are the API contract; an unknown or empty one simply falls back to
// the default order.
const (
	MetricDistance  = "distance"
	MetricSpeed     = "speed"
	MetricDuration  = "duration"
	MetricElevation = "elevation"
	MetricLoad      = "load"
	// The four below are never a PrimaryMetric choice (defaultMetricOrder is
	// unchanged) — they only key detailStatsMap for the ride-detail grid.
	MetricHeartRate = "heart_rate"
	MetricIntensity = "intensity"
	MetricDrift     = "drift"
	MetricHeadwind  = "headwind"
)

// defaultMetricOrder is what the page showed before anyone could choose:
// distance first because it is the figure every rider quotes.
var defaultMetricOrder = []string{
	MetricDistance, MetricDuration, MetricElevation, MetricSpeed, MetricLoad,
}

// headlineStatsLimit — three figures fit the hero on a phone without the block
// eating the screen; a fourth would wrap to its own row.
const headlineStatsLimit = 3

// headlineStats are the figures that lead the page, the rider's preferred one
// first.
//
// A metric that this particular ride has no value for (no climbing recorded,
// no training load without FTP) is skipped rather than shown empty, so the
// next one moves up — a preference must not turn into a gap.
func headlineStats(f RideFacts) []Stat {
	available := detailStatsMap(f)

	var stats []Stat
	seen := map[string]bool{}
	for _, metric := range append([]string{f.PrimaryMetric}, defaultMetricOrder...) {
		if metric == "" || seen[metric] {
			continue
		}
		seen[metric] = true
		if stat, ok := available[metric]; ok {
			stats = append(stats, stat)
		}
		if len(stats) == headlineStatsLimit {
			break
		}
	}
	return stats
}

// detailStatsMap computes every headline-style figure this ride has data
// for, keyed by metric name. headlineStats above curates a PrimaryMetric-led
// 2-3 of these for the narrative hero; detailStats below returns all of them,
// in a fixed order, for the data-dense ride-detail grid (the Nocturne reskin,
// 2026-08-30) — nil/unavailable figures are simply absent, never a
// placeholder, same rule as everywhere else in this file.
func detailStatsMap(f RideFacts) map[string]Stat {
	available := map[string]Stat{}

	if km := f.distanceMeters() / 1000; km > 0 {
		available[MetricDistance] = Stat{Value: decimal(km, 1), Unit: "km", Label: "Distanz"}
	}
	if seconds := f.ElapsedSeconds; seconds > 0 {
		value, unit := durationParts(seconds)
		available[MetricDuration] = Stat{Value: value, Unit: unit, Label: "Dauer"}
	}
	gain := 0.0
	switch {
	case f.ElevationGainMeters != nil:
		gain = *f.ElevationGainMeters
	case f.Course != nil:
		gain = f.Course.ElevationGainMeters
	}
	if gain >= 50 {
		available[MetricElevation] = Stat{
			Value: fmt.Sprintf("%d", int(gain+0.5)), Unit: "hm", Label: "Anstieg",
		}
	}
	if speed := f.avgSpeedKmh(); speed > 0 {
		available[MetricSpeed] = Stat{Value: decimal(speed, 1), Unit: "km/h", Label: "⌀ Tempo"}
	}
	if f.TSS != nil {
		available[MetricLoad] = Stat{
			Value: fmt.Sprintf("%d", int(*f.TSS+0.5)), Label: "Belastung",
		}
	}
	if f.AvgHeartRate != nil && *f.AvgHeartRate > 0 {
		available[MetricHeartRate] = Stat{
			Value: fmt.Sprintf("%d", int(*f.AvgHeartRate+0.5)), Unit: "bpm", Label: "⌀ Puls",
		}
	}
	if f.IntensityFactor != nil {
		available[MetricIntensity] = Stat{Value: decimal(*f.IntensityFactor, 2), Label: "Intensität (IF)"}
	}
	// Same guard efficiencyStatement's call site uses: decoupling only means
	// something on a steady ride, not an interval session (#630).
	if f.Endurance != nil && !f.isMixed() {
		available[MetricDrift] = Stat{
			Value: decimal(absf(f.Endurance.DecouplingPct), 1), Unit: "%", Label: "Abfall 2. Hälfte",
		}
	}
	if f.Course != nil && f.Course.DistanceMeters > 0 {
		available[MetricHeadwind] = Stat{
			Value: fmt.Sprintf("%d", int(f.Course.HeadwindShare*100+0.5)), Unit: "%", Label: "Gegenwind",
		}
	}
	return available
}

// detailStats is detailStatsMap flattened into the fixed order the
// data-dense ride-detail grid displays it in.
func detailStats(f RideFacts) []Stat {
	available := detailStatsMap(f)
	order := []string{
		MetricDistance, MetricSpeed, MetricHeartRate, MetricLoad,
		MetricElevation, MetricDrift, MetricIntensity, MetricHeadwind,
	}
	stats := make([]Stat, 0, len(order))
	for _, metric := range order {
		if stat, ok := available[metric]; ok {
			stats = append(stats, stat)
		}
	}
	return stats
}

// intensityGauge expresses the intensity factor as a percentage bar. 100 % is
// the effort a rider can just about hold for an hour, which makes it the
// natural full mark — harder rides go above it, so the bar clamps while the
// label keeps the true number.
func intensityGauge(f RideFacts) *Gauge {
	if f.IntensityFactor == nil {
		return nil
	}
	percent := int(*f.IntensityFactor*100 + 0.5)
	caption := "aus deiner Leistung"
	label := fmt.Sprintf("%d %% Intensität", percent)
	if !f.FromPower {
		caption = "aus deinem Puls geschätzt"
		// Naming it plain "Intensität" invited the power reading, where 88 %
		// means nearly flat out. On the pulse scale it means 88 % of threshold
		// pulse, and a resting heart is already at roughly 40 % — so the label
		// says which scale the number is on (#630).
		label = fmt.Sprintf("%d %% deines Schwellenpulses", percent)
	}
	return &Gauge{
		Percent: min(percent, 100),
		Label:   label,
		Caption: caption,
	}
}

func effortStatement(f RideFacts) (Statement, bool) {
	if f.IntensityFactor == nil {
		return Statement{
			Text: "Wie anstrengend diese Fahrt für dich war, kann die App noch nicht einordnen — " +
				"dafür fehlt dein FTP-Wert (Leistung) oder deine Schwellen-Herzfrequenz im Profil.",
			Kind: "hint_profile",
		}, true
	}

	text := effortBands[effortBandFor(*f.IntensityFactor, f.FromPower)].text
	if f.isMixed() {
		// "Ein gleichmäßiges Tempo" is the one thing an interval session was
		// not. The zone card right below carries the actual split.
		text = "Wechselhaft: ein Teil der Fahrt lag klar über deiner Schwelle, der Rest deutlich " +
			"darunter. Der Schnitt liegt dazwischen und beschreibt keins von beidem."
	}

	source := "aus deiner Leistung"
	if !f.FromPower {
		source = "aus deinem Puls geschätzt"
	}
	return Statement{
		Text:    text,
		Metric:  fmt.Sprintf("Intensität (IF) %s · %s", decimal(*f.IntensityFactor, 2), source),
		Metrics: []Stat{{Value: decimal(*f.IntensityFactor, 2), Label: "Intensität (IF)"}},
		Kind:    "effort",
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
		Text:    text,
		Metric:  fmt.Sprintf("Belastung (TSS) %d", int(*f.TSS+0.5)),
		Metrics: []Stat{{Value: fmt.Sprintf("%d", int(*f.TSS+0.5)), Label: "Belastung (TSS)"}},
		Kind:    "load",
	}, true
}

// paceStatement is the one effort reading available to a bike with no power
// meter and no heart-rate strap: how fast, put in relation to how hilly.
//
// It must not be dressed up as a fitness verdict. Speed depends on wind,
// gradient, surface and the bike itself, so when it's the ONLY signal there is
// (no FTP/LTHR, hence no intensity factor) the sentence says so itself rather
// than letting the rider over-read it.
func paceStatement(f RideFacts) (Statement, bool) {
	speed := f.avgSpeedKmh()
	if speed <= 0 {
		return Statement{}, false
	}
	metersPerKm := f.metersPerKm()

	text := fmt.Sprintf("Du bist im Schnitt %s km/h gefahren.", decimal(speed, 1))
	if metersPerKm >= 8 {
		text = fmt.Sprintf(
			"Du bist im Schnitt %s km/h gefahren — und das auf einer Strecke mit %d Höhenmetern pro Kilometer, "+
				"wo dasselbe Tempo mehr Arbeit ist als in der Ebene.",
			decimal(speed, 1), int(metersPerKm+0.5))
	}

	if len(f.PriorSpeedsKmh) >= minRidesForComparison {
		if median := medianOf(f.PriorSpeedsKmh); median > 0 {
			switch ratio := speed / median; {
			case ratio >= 1.08:
				text += " Das ist schneller als auf deinen letzten Fahrten."
			case ratio <= 0.92:
				text += " Das ist langsamer als auf deinen letzten Fahrten."
			}
		}
	}

	if f.IntensityFactor == nil {
		text += " Wie anstrengend das für dich war, sagt das Tempo allerdings nicht: " +
			"Wind, Steigung, Untergrund und Rad zählen genauso mit."
	}

	metric := fmt.Sprintf("⌀ %s km/h · %s km", decimal(speed, 1), decimal(f.distanceMeters()/1000, 1))
	metrics := []Stat{
		{Value: decimal(speed, 1), Unit: "km/h", Label: "⌀ Tempo"},
		{Value: decimal(f.distanceMeters()/1000, 1), Unit: "km", Label: "Distanz"},
	}
	return Statement{Text: text, Metric: metric, Metrics: metrics, Kind: "pace"}, true
}

// climbStatement reports the ride's best ascent in climbing speed (VAM) —
// vertical metres per hour. It needs nothing but elevation and time, which
// makes it the one hard performance number available to a rider with no power
// meter and no heart-rate strap, and it is comparable across rides and riders.
//
// With a body weight on file it also gives relative power via Ferrari's
// approximation. That is explicitly a rough conversion, and the text says so —
// presenting it as measured watts would be a lie dressed as precision.
func climbStatement(f RideFacts) (Statement, bool) {
	if f.Course == nil || f.Course.BestClimb == nil {
		return Statement{}, false
	}
	c := *f.Course.BestClimb

	text := fmt.Sprintf(
		"Dein längster Anstieg ging über %s km bei %s %% Steigung. Du bist ihn mit %d Höhenmetern "+
			"pro Stunde hochgefahren — diese Kletterrate kannst du direkt mit anderen Fahrten "+
			"vergleichen, ganz ohne Messgeräte am Rad.",
		decimal(c.DistanceMeters/1000, 1), decimal(c.GradePct, 1), int(c.VAM+0.5))

	metric := fmt.Sprintf("%d hm/h · %d Höhenmeter · %s", int(c.VAM+0.5), int(c.GainMeters+0.5), duration(int(c.Seconds)))
	metrics := []Stat{
		{Value: fmt.Sprintf("%d", int(c.VAM+0.5)), Unit: "hm/h", Label: "Kletterrate"},
		{Value: fmt.Sprintf("%d", int(c.GainMeters+0.5)), Unit: "Höhenmeter"},
	}

	if wkg, ok := c.RelativePowerWkg(); ok && f.WeightKg != nil && *f.WeightKg > 0 {
		text += fmt.Sprintf(
			" Bei deinem Gewicht entspricht das grob %s Watt pro Kilogramm, also rund %d Watt — "+
				"überschlagen aus Steigung und Tempo, nicht gemessen.",
			decimal(wkg, 1), int(wkg**f.WeightKg+0.5))
		metric += fmt.Sprintf(" · ≈ %s W/kg", decimal(wkg, 1))
		metrics = append(metrics, Stat{Value: decimal(wkg, 1), Unit: "W/kg"})
	}

	return Statement{Text: text, Metric: metric, Metrics: metrics, Kind: "climb"}, true
}

// contextStatements explain why a ride felt harder or easier than the bare
// numbers suggest. Only clearly noticeable conditions get a sentence —
// mentioning 0.2 m/s of wind would be noise.
func contextStatements(f RideFacts) []Statement {
	var out []Statement

	// Per-heading wind beats the stored hourly average whenever GPS allows it:
	// the average cancels itself out on an out-and-back (see Course).
	if c := f.Course; c != nil && c.HasWind {
		out = append(out, windShareStatement(*c))
	} else if f.HeadwindMps != nil && absf(*f.HeadwindMps) >= 1.0 {
		wind := *f.HeadwindMps
		text := "Im Schnitt hattest du Gegenwind — der kostet Tempo bei gleicher Anstrengung, " +
			"deine Geschwindigkeit sagt an so einem Tag weniger über deine Form aus."
		if wind < 0 {
			text = "Im Schnitt hattest du Rückenwind — dadurch fielen die Zeiten leichter als die Anstrengung vermuten lässt."
		}
		out = append(out, Statement{
			Text:    text,
			Metric:  fmt.Sprintf("⌀ %s m/s %s", decimal(absf(wind), 1), windLabel(wind)),
			Metrics: []Stat{{Value: decimal(absf(wind), 1), Unit: "m/s", Label: windLabel(wind)}},
			Kind:    "context",
		})
	}

	if c := f.Course; c != nil && c.HasTerrain && (c.ClimbDistanceShare >= 0.15 || f.metersPerKm() >= 8) {
		out = append(out, terrainStatement(*c, f.metersPerKm()))
	} else if gain := f.ElevationGainMeters; gain != nil && *gain >= 300 {
		out = append(out, Statement{
			Text: "Die Strecke war hügelig. Bergauf steigt die Anstrengung stark an, " +
				"auch wenn die Durchschnittsgeschwindigkeit dadurch niedriger aussieht.",
			Metric:  fmt.Sprintf("%d Höhenmeter", int(*gain+0.5)),
			Metrics: []Stat{{Value: fmt.Sprintf("%d", int(*gain+0.5)), Unit: "Höhenmeter"}},
			Kind:    "context",
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
			Text:    text,
			Metric:  fmt.Sprintf("⌀ %d °C", int(*t+0.5)),
			Metrics: []Stat{{Value: fmt.Sprintf("%d", int(*t+0.5)), Unit: "°C", Label: "Temperatur"}},
			Kind:    "context",
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
		Text:    text,
		Metric:  fmt.Sprintf("Belastung sonst im Mittel %d", int(median+0.5)),
		Metrics: []Stat{{Value: fmt.Sprintf("%d", int(median+0.5)), Label: "Sonst im Mittel"}},
		Kind:    "comparison",
	}, true
}

// windShareStatement reports how much of the DISTANCE was ridden into the
// wind. That is the number a rider recognises from the ride; an hourly average
// headwind of "0.1 m/s" for an out-and-back is technically true and useless.
func windShareStatement(c CourseStats) Statement {
	head, tail := int(c.HeadwindShare*100+0.5), int(c.TailwindShare*100+0.5)

	var text string
	switch {
	case head >= 40 && c.HeadwindOnClimbShare >= 0.5 && c.ClimbDistanceShare > 0:
		text = fmt.Sprintf(
			"Auf %d %% der Strecke ging es gegen den Wind — und der Gegenwind lag überwiegend in den Anstiegen. "+
				"Diese Kombination ist die härteste, die eine Runde zu bieten hat.", head)
	case head >= 40:
		text = fmt.Sprintf(
			"Auf %d %% der Strecke ging es gegen den Wind, auf %d %% mit ihm. "+
				"Gegenwind kostet Tempo bei gleicher Anstrengung — deine Geschwindigkeit sagt an so einem Tag "+
				"weniger über deine Form aus.", head, tail)
	case tail >= 40:
		text = fmt.Sprintf(
			"Der Wind war überwiegend auf deiner Seite: %d %% der Strecke mit Rückenwind, nur %d %% dagegen. "+
				"Die Zeiten fielen dadurch leichter als die Anstrengung vermuten lässt.", tail, head)
	default:
		text = "Der Wind kam meist von der Seite — der bremst kaum, macht aber die Lenkung unruhig."
	}

	return Statement{
		Text: text,
		Metric: fmt.Sprintf("⌀ %s m/s Wind aus %s · %d %% gegen, %d %% mit",
			decimal(c.MeanWindSpeedMps, 1), compassName(c.WindFromDeg), head, tail),
		Kind: "context",
	}
}

// terrainStatement describes the profile in terms a rider feels: how much of
// the way went up, and how steep the hardest sustained stretch was.
func terrainStatement(c CourseStats, metersPerKm float64) Statement {
	share := int(c.ClimbDistanceShare*100 + 0.5)

	text := fmt.Sprintf(
		"Auf etwa %d %% der Strecke ging es aufwärts. Bergauf steigt die Anstrengung stark an, "+
			"auch wenn die Durchschnittsgeschwindigkeit dadurch niedriger aussieht.", share)
	if c.SteepestGradePct >= 6 {
		text += fmt.Sprintf(" Die steilste längere Rampe hatte rund %s %% Steigung.",
			decimal(c.SteepestGradePct, 1))
	}

	return Statement{
		Text: text,
		Metric: fmt.Sprintf("%d Höhenmeter · %d hm pro km",
			int(c.ElevationGainMeters+0.5), int(metersPerKm+0.5)),
		Metrics: []Stat{
			{Value: fmt.Sprintf("%d", int(c.ElevationGainMeters+0.5)), Unit: "Höhenmeter"},
			{Value: fmt.Sprintf("%d", int(metersPerKm+0.5)), Unit: "hm/km"},
		},
		Kind: "context",
	}
}

func windLabel(headwindMps float64) string {
	if headwindMps < 0 {
		return "Rückenwind"
	}
	return "Gegenwind"
}

// medianOf returns 0 for an empty input rather than panicking — callers guard
// on their own minimum sample counts, and a "no data" median of 0 is what
// every one of them treats as "don't make a claim".
func medianOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
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
	value, unit := durationParts(seconds)
	return value + " " + unit
}

// durationParts splits the duration so the number can be typeset large and the
// unit small, like every other headline figure.
func durationParts(seconds int) (value, unit string) {
	h, m := seconds/3600, (seconds%3600)/60
	if h > 0 {
		return fmt.Sprintf("%d:%02d", h, m), "h"
	}
	return fmt.Sprintf("%d", m), "min"
}
