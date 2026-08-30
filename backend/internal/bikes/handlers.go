package bikes

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DerDaehne/wff/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool}
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/bikes", auth.RequireAuth(h.pool)(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/bikes", auth.RequireAuth(h.pool)(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /api/bikes/{id}", auth.RequireAuth(h.pool)(http.HandlerFunc(h.update)))
	mux.Handle("POST /api/bikes/{id}/activate", auth.RequireAuth(h.pool)(http.HandlerFunc(h.activate)))
	mux.Handle("POST /api/bikes/{id}/chain-replaced", auth.RequireAuth(h.pool)(http.HandlerFunc(h.chainReplaced)))
}

// list is also the response every mutation ends with (create/update/activate/
// chain-replaced all finish by calling it) — the same pattern profile's
// update() uses: the client always gets the fresh, authoritative state back
// rather than trusting its own optimistic guess of what changed.
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	var activeBikeID *int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT active_bike_id FROM users WHERE id = $1`, userID,
	).Scan(&activeBikeID); err != nil {
		http.Error(w, "could not load bikes", http.StatusInternalServerError)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT b.id, b.name, b.retired_at, b.chain_interval_km, b.chain_replaced_at_km,
		       coalesce(sum(a.distance_meters), 0) / 1000.0,
		       count(a.id),
		       coalesce(sum(a.moving_seconds), 0),
		       coalesce(sum(a.elevation_gain_meters), 0)
		FROM bikes b
		LEFT JOIN activities a ON a.bike_id = b.id
		WHERE b.user_id = $1
		GROUP BY b.id
		ORDER BY b.retired_at NULLS FIRST, b.created_at`,
		userID,
	)
	if err != nil {
		http.Error(w, "could not load bikes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []Bike{} // never null in the JSON response
	for rows.Next() {
		var id int64
		var name string
		var retiredAt *time.Time
		var intervalKm, replacedAtKm, distanceKm, elevationGainMeters float64
		var rideCount int
		var movingSeconds int64
		if err := rows.Scan(
			&id, &name, &retiredAt, &intervalKm, &replacedAtKm, &distanceKm,
			&rideCount, &movingSeconds, &elevationGainMeters,
		); err != nil {
			http.Error(w, "could not load bikes", http.StatusInternalServerError)
			return
		}
		out = append(out, Bike{
			ID:                  id,
			Name:                name,
			Active:              activeBikeID != nil && *activeBikeID == id,
			RetiredAt:           retiredAt,
			DistanceKm:          distanceKm,
			ChainIntervalKm:     intervalKm,
			ChainDueKm:          chainDueKm(distanceKm, intervalKm, replacedAtKm),
			RideCount:           rideCount,
			MovingSeconds:       movingSeconds,
			ElevationGainMeters: elevationGainMeters,
			AvgSpeedKmh:         avgSpeedKmh(distanceKm, movingSeconds),
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load bikes", http.StatusInternalServerError)
		return
	}

	writeJSON(w, out)
}

func (h *Handlers) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	var bikeID int64
	if err := h.pool.QueryRow(r.Context(),
		`INSERT INTO bikes (user_id, name) VALUES ($1, $2) RETURNING id`, userID, body.Name,
	).Scan(&bikeID); err != nil {
		http.Error(w, "could not create bike", http.StatusInternalServerError)
		return
	}

	// A rider's first bike becomes active automatically — most riders have
	// exactly one, and forcing an extra "make this active" click for the
	// common case would be needless friction. Only fires when nothing is
	// active yet, so adding a second bike never silently switches the rider
	// off the one they're currently riding.
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE users SET active_bike_id = $1 WHERE id = $2 AND active_bike_id IS NULL`,
		bikeID, userID,
	); err != nil {
		http.Error(w, "could not create bike", http.StatusInternalServerError)
		return
	}

	h.list(w, r)
}

func (h *Handlers) update(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	bikeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !h.ownsBike(r.Context(), userID, bikeID) {
		http.Error(w, "bike not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name            *string  `json:"name"`
		ChainIntervalKm *float64 `json:"chain_interval_km"`
		Retired         *bool    `json:"retired"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.ChainIntervalKm != nil && *body.ChainIntervalKm <= 0 {
		http.Error(w, "chain_interval_km must be positive", http.StatusBadRequest)
		return
	}

	if body.Retired != nil {
		if *body.Retired {
			if _, err := h.pool.Exec(r.Context(),
				`UPDATE bikes SET retired_at = now() WHERE id = $1`, bikeID,
			); err != nil {
				http.Error(w, "could not update bike", http.StatusInternalServerError)
				return
			}
			// A retired bike can't stay the active one — new uploads would
			// silently attach to a bike no longer in the rider's rotation.
			if _, err := h.pool.Exec(r.Context(),
				`UPDATE users SET active_bike_id = NULL WHERE id = $1 AND active_bike_id = $2`,
				userID, bikeID,
			); err != nil {
				http.Error(w, "could not update bike", http.StatusInternalServerError)
				return
			}
		} else if _, err := h.pool.Exec(r.Context(),
			`UPDATE bikes SET retired_at = NULL WHERE id = $1`, bikeID,
		); err != nil {
			http.Error(w, "could not update bike", http.StatusInternalServerError)
			return
		}
	}

	if _, err := h.pool.Exec(r.Context(), `
		UPDATE bikes SET name = COALESCE($2, name), chain_interval_km = COALESCE($3, chain_interval_km)
		WHERE id = $1`,
		bikeID, body.Name, body.ChainIntervalKm,
	); err != nil {
		http.Error(w, "could not update bike", http.StatusInternalServerError)
		return
	}

	h.list(w, r)
}

func (h *Handlers) activate(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	bikeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !h.ownsBike(r.Context(), userID, bikeID) {
		http.Error(w, "bike not found", http.StatusNotFound)
		return
	}

	var retiredAt *time.Time
	if err := h.pool.QueryRow(r.Context(),
		`SELECT retired_at FROM bikes WHERE id = $1`, bikeID,
	).Scan(&retiredAt); err != nil {
		http.Error(w, "could not activate bike", http.StatusInternalServerError)
		return
	}
	if retiredAt != nil {
		http.Error(w, "cannot activate a retired bike", http.StatusBadRequest)
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE users SET active_bike_id = $1 WHERE id = $2`, bikeID, userID,
	); err != nil {
		http.Error(w, "could not activate bike", http.StatusInternalServerError)
		return
	}

	h.list(w, r)
}

// chainReplaced resets the wear counter to the bike's current odometer
// reading — measured the same way the reminder is, a live SUM over its
// rides, not a value the caller supplies (which could be typed wrong or
// drift from reality).
func (h *Handlers) chainReplaced(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	bikeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !h.ownsBike(r.Context(), userID, bikeID) {
		http.Error(w, "bike not found", http.StatusNotFound)
		return
	}

	if _, err := h.pool.Exec(r.Context(), `
		UPDATE bikes SET chain_replaced_at_km = coalesce(
			(SELECT sum(distance_meters) FROM activities WHERE bike_id = $1), 0
		) / 1000.0
		WHERE id = $1`,
		bikeID,
	); err != nil {
		http.Error(w, "could not update bike", http.StatusInternalServerError)
		return
	}

	h.list(w, r)
}

func (h *Handlers) ownsBike(ctx context.Context, userID, bikeID int64) bool {
	return OwnsBike(ctx, h.pool, userID, bikeID)
}

// OwnsBike reports whether bikeID belongs to userID — exported so the
// activities package can validate a bike assignment (#729/#730) without
// duplicating this query.
func OwnsBike(ctx context.Context, pool *pgxpool.Pool, userID, bikeID int64) bool {
	var owner int64
	err := pool.QueryRow(ctx, `SELECT user_id FROM bikes WHERE id = $1`, bikeID).Scan(&owner)
	return err == nil && owner == userID
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
