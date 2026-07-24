package enrich

import (
	"context"
	"log"
	"time"

	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FindIncomplete returns IDs of activities that have GPS samples but fewer
// enrichment rows than expected hour-buckets — candidates for a retry. An
// activity with no GPS at all never appears here (Activity() is a no-op for
// it, so "incomplete" doesn't apply).
func FindIncomplete(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	rows, err := pool.Query(ctx, `
		SELECT a.id
		FROM activities a
		WHERE EXISTS (
			SELECT 1 FROM samples s WHERE s.activity_id = a.id AND s.lat IS NOT NULL
		)
		AND (
			SELECT count(DISTINCT date_trunc('hour', s.time))
			FROM samples s WHERE s.activity_id = a.id AND s.lat IS NOT NULL
		) > (
			SELECT count(*) FROM enrichment e WHERE e.activity_id = a.id
		)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RunPoller retries incompletely-enriched activities on a fixed interval
// until ctx is cancelled. A failed or partial attempt just leaves the
// activity as a candidate for the next tick — see arch-wff-enrichment for
// why ERA5's ~5 day ingest lag makes "not yet available" the normal steady
// state for recent rides, not an error condition worth alerting on.
func RunPoller(ctx context.Context, pool *pgxpool.Pool, client *openmeteo.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOnce(ctx, pool, client)
		}
	}
}

func pollOnce(ctx context.Context, pool *pgxpool.Pool, client *openmeteo.Client) {
	ids, err := FindIncomplete(ctx, pool)
	if err != nil {
		log.Printf("enrich: poll: find incomplete activities: %v", err)
		return
	}
	for _, id := range ids {
		if _, err := Activity(ctx, pool, client, id); err != nil {
			log.Printf("enrich: poll: activity %d: %v", id, err)
		}
	}
}
