package analyze

import (
	"encoding/json"
	"net/http"

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
	mux.Handle("GET /api/training-load", auth.RequireAuth(h.pool)(http.HandlerFunc(h.trainingLoad)))
}

type trainingLoadResponse struct {
	Series   []DayLoad `json:"series"`
	Insights []Insight `json:"insights"`
}

func (h *Handlers) trainingLoad(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	series, err := TrainingLoad(r.Context(), h.pool, userID)
	if err != nil {
		http.Error(w, "could not compute training load", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trainingLoadResponse{
		Series:   series,
		Insights: Insights(series),
	})
}
