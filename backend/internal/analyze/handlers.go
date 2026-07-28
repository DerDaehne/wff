package analyze

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	mux.Handle("GET /api/training-load", auth.RequireAuth(h.pool)(http.HandlerFunc(h.trainingLoad)))
	mux.Handle("GET /api/progress", auth.RequireAuth(h.pool)(http.HandlerFunc(h.progress)))
	mux.Handle("GET /api/me/year-review", auth.RequireAuth(h.pool)(http.HandlerFunc(h.yearReview)))
}

type trainingLoadResponse struct {
	Series   []DayLoad `json:"series"`
	Insights []Insight `json:"insights"`
	// Status says in plain language how the rider is doing, in the same shape
	// as a single ride's story (#602) — the chart below it is the evidence,
	// this is the answer.
	Status Story `json:"status"`
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
		Status:   TrainingStatus(series),
	})
}

// progress returns the weekly history of the plain figures — distance, time,
// climbing, speed — plus what their direction means (#618). Separate from
// training-load because it answers a different question: that one is "how am I
// right now", this one is "am I getting anywhere".
func (h *Handlers) progress(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	progress, err := WeeklyProgress(r.Context(), h.pool, userID)
	if err != nil {
		http.Error(w, "could not compute progress", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(progress)
}

// yearReview answers "how was my year" (#638) — a plain sum of a calendar
// year's rides plus its two standout rides, not a new metric. ?year=YYYY
// selects the year; missing or unparsable falls back to the current one
// rather than erroring, since a stray query param shouldn't break the page.
func (h *Handlers) yearReview(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())

	year, err := strconv.Atoi(r.URL.Query().Get("year"))
	if err != nil {
		year = time.Now().Year()
	}

	review, err := YearReviewFor(r.Context(), h.pool, userID, year)
	if err != nil {
		http.Error(w, "could not compute year review", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}
