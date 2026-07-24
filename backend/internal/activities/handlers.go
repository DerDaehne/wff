// Package activities implements the .fit upload HTTP endpoint, wiring
// fitparse (decode) and ingest (persist) together behind the auth middleware.
package activities

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/enrich"
	"github.com/DerDaehne/wff/internal/fitparse"
	"github.com/DerDaehne/wff/internal/ingest"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxUploadBytes is generous for .fit: even multi-hour rides at 1 Hz are
// typically low single-digit MB.
const maxUploadBytes = 50 << 20

type Handlers struct {
	pool      *pgxpool.Pool
	uploadDir string
	weather   *openmeteo.Client
}

func NewHandlers(pool *pgxpool.Pool, uploadDir string, weather *openmeteo.Client) *Handlers {
	return &Handlers{pool: pool, uploadDir: uploadDir, weather: weather}
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/activities", auth.RequireAuth(h.pool)(http.HandlerFunc(h.upload)))
}

func (h *Handlers) upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "request too large or malformed", http.StatusRequestEntityTooLarge)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing \"file\" field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read upload", http.StatusBadRequest)
		return
	}

	act, err := fitparse.Parse(raw)
	if err != nil {
		http.Error(w, "invalid .fit file: "+err.Error(), http.StatusBadRequest)
		return
	}

	externalUID := ingest.ExternalUID(act.FileID, raw)

	rawPath, err := h.saveRawFile(userID, externalUID, raw)
	if err != nil {
		http.Error(w, "could not store upload", http.StatusInternalServerError)
		return
	}

	activityID, err := ingest.Store(r.Context(), h.pool, userID, act, externalUID)
	if err != nil {
		if errors.Is(err, ingest.ErrDuplicateActivity) {
			// rawPath is deterministic from externalUID, so on a duplicate it's
			// either the pre-existing file untouched or overwritten with
			// identical bytes — either way it still correctly belongs to the
			// original activity row. Do not remove it.
			http.Error(w, "activity already exists", http.StatusConflict)
			return
		}
		os.Remove(rawPath) // genuine failure: no activity row references this file
		http.Error(w, "could not store activity", http.StatusInternalServerError)
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE activities SET raw_file_path = $1 WHERE id = $2`, rawPath, activityID,
	); err != nil {
		http.Error(w, "could not store activity", http.StatusInternalServerError)
		return
	}

	// Best-effort immediate attempt: usually a no-op wait (ERA5 has ~5 day
	// lag) or a no-op for GPS-less rides, but occasionally data is already
	// there. Uses its own background context — the HTTP response below has
	// already gone out by the time this runs. The retry poller (started in
	// main) is the durable path; this is purely a latency optimization.
	go func() {
		if _, err := enrich.Activity(context.Background(), h.pool, h.weather, activityID); err != nil {
			log.Printf("enrich: immediate attempt for activity %d: %v", activityID, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"activity_id":` + strconv.FormatInt(activityID, 10) + `}`))
}

func (h *Handlers) saveRawFile(userID int64, externalUID string, raw []byte) (string, error) {
	dir := filepath.Join(h.uploadDir, strconv.FormatInt(userID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, externalUID+".fit")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
