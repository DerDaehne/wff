package analyze

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Heart-rate zones turn one average number into what a ride actually was
// (#621). An hour at a steady easy pace and an hour with four hard efforts
// share the same average heart rate and are completely different training.
//
// The zone model is the usual one relative to threshold heart rate (Friel):
// the boundaries below are percentages of LTHR. They are percentages rather
// than fixed pulse values because a threshold of 150 and one of 185 describe
// the same effort at the same percentage.
//
// The names are the point, not the numbers. "Zone 3" means nothing to someone
// who has never read a training book; "zügig — der teuerste Kompromiss" does.

// Zone is one band, its share of the ride, and what it means in words.
type Zone struct {
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	Meaning string  `json:"meaning"`
	Seconds int     `json:"seconds"`
	Share   float64 `json:"share"`
}

// ZoneDistribution is the time spent per zone plus what the split says.
type ZoneDistribution struct {
	Zones        []Zone      `json:"zones"`
	TotalSeconds int         `json:"total_seconds"`
	Statements   []Statement `json:"statements"`
	// Assumed marks zones built from an observed maximum heart rate instead
	// of a real threshold — a looser stand-in the rider hasn't confirmed by
	// riding an actual threshold effort (#624).
	Assumed bool `json:"assumed,omitempty"`
}

// zoneDefs are the bands in ascending order. minPctLTHR is the lower edge; the
// upper edge is the next zone's lower edge, and the last zone is open-ended.
var zoneDefs = []struct {
	key, name, meaning string
	minPctLTHR         float64
}{
	{"recovery", "Ganz locker", "Erholung. Hier passiert kein Trainingsreiz — und genau das ist der Zweck: du könntest mühelos nebenher reden.", 0},
	{"endurance", "Grundlage", "Der Bereich, in dem Ausdauer wächst. Du kannst in ganzen Sätzen reden und stundenlang so weiterfahren.", 81},
	{"tempo", "Zügig", "Fühlt sich nach Training an und ist doch der teuerste Kompromiss: zu hart, um sich zu erholen, zu leicht für einen echten Reiz.", 90},
	{"threshold", "Schwelle", "Knapp an der Grenze, die du etwa eine Stunde halten kannst. Reden geht nur noch in Wortfetzen.", 94},
	{"vo2", "Ganz hart", "Über deiner Schwelle. Nur in kurzen Intervallen haltbar, und danach brauchst du echte Erholung.", 100},
}

// zoneForRatio picks the band a pulse-derived intensity falls into. It is the
// same table the zone chart buckets on, which is the point: a ride the chart
// calls "Grundlage" cannot be titled "harte Einheit" above it (#630).
func zoneForRatio(ratio float64) int {
	band := 0
	for i, z := range zoneDefs {
		if ratio >= z.minPctLTHR/100 {
			band = i
		}
	}
	return band
}

// aerobicMaxRatioHR is where a heart-rate intensity stops describing the
// aerobic base: the top of the endurance band, which is the range decoupling
// and EF are meant to be read in. Its power-side counterpart is
// efficiencyMaxIF, and the two numbers differ because the scales do — resting
// pulse already sits near 40 % of threshold, so IF_hr never reaches the low
// values a power IF shows on an easy ride.
var aerobicMaxRatioHR = zoneDefs[2].minPctLTHR / 100

const (
	// greyZoneMaxShare — above this much time in the tempo band the week has
	// drifted into the pattern that costs the most and returns the least. The
	// polarised-training literature puts the useful upper bound at roughly a
	// fifth of total time; a quarter is where it is worth saying something.
	greyZoneMaxShare = 0.25
	// hardMinShare — under this, nothing in the week was hard enough to be a
	// stimulus at all.
	hardMinShare = 0.02
	// easyTargetShare is the "80" in 80/20.
	easyTargetShare = 0.80
	// zoneVerdictMinSeconds — under three hours of recorded pulse in a week,
	// one ride decides the whole distribution.
	zoneVerdictMinSeconds = 3 * 3600
	// zoneRideMinSeconds — the same idea for a single ride: under ten minutes
	// of pulse there is no distribution worth showing.
	zoneRideMinSeconds = 600
	// zoneMixedHardShare — above this much time above threshold, a ride is an
	// interval session whatever its biggest band says.
	zoneMixedHardShare = 0.15
)

// ZoneBounds converts the percentage model into the absolute pulse values that
// the database aggregation buckets on. Returns the lower edges of zones 2..n:
// width_bucket puts everything below the first edge into bucket 0, which is
// exactly zone 1.
func ZoneBounds(lthrBpm int) []float64 {
	bounds := make([]float64, 0, len(zoneDefs)-1)
	for _, z := range zoneDefs[1:] {
		bounds = append(bounds, float64(lthrBpm)*z.minPctLTHR/100)
	}
	return bounds
}

// distribution turns raw per-zone seconds into zones with names and shares.
func distribution(seconds []int) ZoneDistribution {
	out := ZoneDistribution{Zones: make([]Zone, len(zoneDefs))}
	for i, def := range zoneDefs {
		out.Zones[i] = Zone{Key: def.key, Name: def.name, Meaning: def.meaning}
		if i < len(seconds) {
			out.Zones[i].Seconds = seconds[i]
			out.TotalSeconds += seconds[i]
		}
	}
	if out.TotalSeconds > 0 {
		for i := range out.Zones {
			out.Zones[i].Share = float64(out.Zones[i].Seconds) / float64(out.TotalSeconds)
		}
	}
	return out
}

// shares collapses the five bands into the three that training distribution is
// actually judged on: easy, the grey middle, and hard.
func (d ZoneDistribution) shares() (easy, grey, hard float64) {
	for _, z := range d.Zones {
		switch z.Key {
		case "recovery", "endurance":
			easy += z.Share
		case "tempo":
			grey += z.Share
		default:
			hard += z.Share
		}
	}
	return easy, grey, hard
}

// RideZones is the distribution for a single ride, said in one sentence: which
// band the ride mostly lived in.
func RideZones(seconds []int) (ZoneDistribution, bool) {
	d := distribution(seconds)
	if d.TotalSeconds < zoneRideMinSeconds {
		return ZoneDistribution{}, false
	}

	var top Zone
	for _, z := range d.Zones {
		if z.Seconds > top.Seconds {
			top = z
		}
	}

	// An interval session spends most of its minutes recovering, so its biggest
	// band is "ganz locker" — which would print as the ride's character
	// directly under the card calling that same ride hard, and the pair reads
	// as a bug. Once a real part of the ride was above threshold, the split is
	// the honest summary, not the largest slice of it.
	if easy, grey, hard := d.shares(); hard >= zoneMixedHardShare {
		return ZoneDistribution{
			Zones:        d.Zones,
			TotalSeconds: d.TotalSeconds,
			Statements: []Statement{{
				Text: fmt.Sprintf(
					"Diese Fahrt hatte kein einheitliches Tempo: %d %% locker, %d %% zügig, %d %% hart. "+
						"So sieht es aus, wenn du zwischendurch richtig Gas gegeben hast — der "+
						"Durchschnittspuls verrät davon nichts, die Aufteilung schon.",
					int(easy*100+0.5), int(grey*100+0.5), int(hard*100+0.5)),
				Metric: fmt.Sprintf("%s Puls aufgezeichnet", duration(d.TotalSeconds)),
				Kind:   "zones",
			}},
		}, true
	}

	d.Statements = []Statement{{
		Text: fmt.Sprintf("Den größten Teil der Fahrt lag dein Puls im Bereich „%s\". %s",
			top.Name, top.Meaning),
		Metric: fmt.Sprintf("%s davon · %s Puls aufgezeichnet",
			duration(top.Seconds), duration(d.TotalSeconds)),
		Kind: "zones",
	}}
	return d, true
}

// WeeklyZones judges the distribution over a block of weeks — the level at
// which it means anything. A single ride in the grey zone is a ride; a month of
// them is a training pattern.
func WeeklyZones(seconds []int) ZoneDistribution {
	d := distribution(seconds)
	if d.TotalSeconds < zoneVerdictMinSeconds {
		if d.TotalSeconds > 0 {
			d.Statements = []Statement{{
				Kind: "hint_history",
				Text: fmt.Sprintf(
					"Für eine Aussage über deine Verteilung ist noch zu wenig Puls aufgezeichnet — "+
						"bisher %s in den letzten Wochen, sinnvoll wird es ab etwa drei Stunden.",
					duration(d.TotalSeconds)),
			}}
		}
		return d
	}

	easy, grey, hard := d.shares()
	var text string
	switch {
	case grey > greyZoneMaxShare:
		text = fmt.Sprintf(
			"%d %% deiner Zeit lagen im zügigen Bereich. Das ist der teuerste Kompromiss: zu hart, "+
				"um sich davon zu erholen, zu leicht für einen echten Reiz. Fahr die lockeren Fahrten "+
				"deutlich lockerer — und wenn es hart sein soll, dann richtig.",
			int(grey*100+0.5))
	case hard < hardMinShare:
		text = fmt.Sprintf(
			"Du fährst fast alles ruhig (%d %% locker). Das ist eine gute Grundlage, aber ohne eine "+
				"harte Einheit fehlt der Reiz nach oben. Eine Fahrt pro Woche mit ein paar kurzen, "+
				"kräftigen Anstiegen reicht schon.",
			int(easy*100+0.5))
	case easy >= easyTargetShare:
		text = fmt.Sprintf(
			"Deine Verteilung passt: %d %% locker, %d %% hart, wenig dazwischen. Genau so ist "+
				"Training am wirksamsten — der Großteil ruhig, ein kleiner Teil richtig hart.",
			int(easy*100+0.5), int(hard*100+0.5))
	default:
		text = fmt.Sprintf(
			"Du fährst %d %% locker und %d %% hart. Etwas mehr Ruhe in den lockeren Fahrten würde "+
				"die harten besser wirken lassen — angepeilt sind rund 80 %% locker.",
			int(easy*100+0.5), int(hard*100+0.5))
	}

	d.Statements = []Statement{{
		Text: text,
		Metric: fmt.Sprintf("%d %% locker · %d %% zügig · %d %% hart · %s Puls aufgezeichnet",
			int(easy*100+0.5), int(grey*100+0.5), int(hard*100+0.5), duration(d.TotalSeconds)),
		Kind: "zones",
	}}
	return d
}

// zoneSeconds aggregates time per zone in the database rather than pulling
// every sample across the wire: the answer is five numbers, and for a week's
// worth of rides the samples behind them are hundreds of thousands of rows.
//
// The zone edges travel as a parameter, so the model stays defined in Go
// (zoneDefs) and this query knows nothing about training theory.
func zoneSeconds(ctx context.Context, pool *pgxpool.Pool, activityIDs []int64, lthrBpm int) ([]int, error) {
	seconds := make([]int, len(zoneDefs))
	if len(activityIDs) == 0 || lthrBpm <= 0 {
		return seconds, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT bucket, sum(gap)::bigint FROM (
			SELECT width_bucket(heart_rate::float, $2::float[]) AS bucket,
			       extract(epoch FROM time - lag(time) OVER (PARTITION BY activity_id ORDER BY time)) AS gap
			FROM samples
			WHERE activity_id = ANY($1) AND heart_rate IS NOT NULL
		) t
		-- A stop belongs to no zone.
		WHERE gap > 0 AND gap <= $3
		GROUP BY bucket`,
		activityIDs, ZoneBounds(lthrBpm), MaxSampleGapSeconds,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var bucket, total int
		if err := rows.Scan(&bucket, &total); err != nil {
			return nil, err
		}
		if bucket >= 0 && bucket < len(seconds) {
			seconds[bucket] = total
		}
	}
	return seconds, rows.Err()
}

// ActivityZones is the per-ride distribution, or nothing if the rider has no
// threshold heart rate — real or assumed (#624) — configured. assumed marks
// the distribution as built off an observed maximum rather than a real
// threshold, for callers that resolved lthrBpm via EffectiveLTHR.
func ActivityZones(ctx context.Context, pool *pgxpool.Pool, activityID int64, lthrBpm *int, assumed bool) (*ZoneDistribution, error) {
	if lthrBpm == nil {
		return nil, nil
	}
	seconds, err := zoneSeconds(ctx, pool, []int64{activityID}, *lthrBpm)
	if err != nil {
		return nil, err
	}
	d, ok := RideZones(seconds)
	if !ok {
		return nil, nil
	}
	d.Assumed = assumed
	return &d, nil
}

// RecentZones is the same over the rider's recent weeks — the level at which
// the distribution is a training pattern rather than one afternoon. assumed
// marks the distribution as built off an observed maximum rather than a real
// threshold, for callers that resolved lthrBpm via EffectiveLTHR.
func RecentZones(ctx context.Context, pool *pgxpool.Pool, userID int64, lthrBpm *int, assumed bool) (ZoneDistribution, error) {
	if lthrBpm == nil {
		// Without a threshold there is nothing to say about zones, but plenty to
		// say about that: the profile already estimates one from the rider's own
		// rides (#609).
		return ZoneDistribution{Statements: []Statement{{
			Kind: "hint_profile",
			Text: "Für Pulsbereiche fehlt dein Schwellenpuls. Ohne ihn lässt sich nicht sagen, was für " +
				"dich locker und was hart ist. Im Profil steht meist schon eine Schätzung aus deinen " +
				"eigenen Fahrten bereit.",
		}}}, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT id FROM activities
		WHERE user_id = $1 AND started_at > now() - make_interval(weeks => $2)`,
		userID, zoneWeeks,
	)
	if err != nil {
		return ZoneDistribution{}, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return ZoneDistribution{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return ZoneDistribution{}, err
	}

	seconds, err := zoneSeconds(ctx, pool, ids, *lthrBpm)
	if err != nil {
		return ZoneDistribution{}, err
	}
	d := WeeklyZones(seconds)
	d.Assumed = assumed
	return d, nil
}

// zoneWeeks is the window the distribution is judged over: long enough to be a
// pattern, short enough to still describe how the rider trains now.
const zoneWeeks = 4
