package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A device token is the second way into this app, next to the browser session
// (#617). It exists because an iPhone cannot share a file into a web app —
// Safari has never implemented the Web Share Target API (WebKit bug 194593) —
// so the iOS route is a Shortcut that posts the .fit itself, and a Shortcut
// cannot perform a WebAuthn ceremony.
//
// The whole design follows from where the token lives: in cleartext inside a
// Shortcut, on a phone, synced through iCloud. So:
//
//   - it can upload rides and nothing else (RequireUploadAuth is the only
//     middleware that accepts it — in particular it cannot mint further tokens,
//     because token management sits behind RequireAuth, session only),
//   - only its SHA-256 hash is stored, so a database dump does not hand out
//     upload rights,
//   - it is revocable per device, which is why each one carries a name,
//   - it does not expire on its own; last_used_at is shown instead so a token
//     no longer in use is visible and can be removed deliberately.
const deviceTokenPrefix = "wff_"

// insertDeviceToken returns the cleartext token exactly once — only its hash is
// stored, so it cannot be shown again later.
func insertDeviceToken(ctx context.Context, pool *pgxpool.Pool, userID int64, name string) (string, deviceTokenResponse, error) {
	// The prefix makes the string recognisable as a WFF credential wherever it
	// surfaces (Shortcut, clipboard, secret scanners), which pure base64 isn't.
	token := deviceTokenPrefix + randomToken(32)
	hash := sha256.Sum256([]byte(token))

	var created deviceTokenResponse
	err := pool.QueryRow(ctx,
		`INSERT INTO device_tokens (user_id, token_hash, name) VALUES ($1, $2, $3)
		 RETURNING id, name, created_at, last_used_at`,
		userID, hash[:], name,
	).Scan(&created.ID, &created.Name, &created.CreatedAt, &created.LastUsedAt)
	if err != nil {
		return "", deviceTokenResponse{}, err
	}
	return token, created, nil
}

// deviceTokenUserID resolves an Authorization: Bearer header. It returns false
// for anything it doesn't recognise; the caller decides what that means.
func deviceTokenUserID(r *http.Request, pool *pgxpool.Pool) (int64, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || !strings.HasPrefix(token, deviceTokenPrefix) {
		return 0, false
	}
	hash := sha256.Sum256([]byte(token))

	var userID int64
	if err := pool.QueryRow(r.Context(),
		`UPDATE device_tokens SET last_used_at = now() WHERE token_hash = $1 RETURNING user_id`,
		hash[:],
	).Scan(&userID); err != nil {
		return 0, false
	}
	return userID, true
}

// RequireUploadAuth accepts either a browser session or a device token. It is
// deliberately the only place device tokens are honoured — see the note above.
func RequireUploadAuth(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := sessionUserID(r, pool)
			if !ok {
				userID, ok = deviceTokenUserID(r, pool)
			}
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDContextKey, userID)))
		})
	}
}

type deviceTokenResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	// Set only in the response to creation — the one moment it exists in the
	// clear.
	Token string `json:"token,omitempty"`
}

func (h *Handlers) listDeviceTokens(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserID(r.Context())

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, created_at, last_used_at FROM device_tokens
		 WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		http.Error(w, "could not load device tokens", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tokens := []deviceTokenResponse{}
	for rows.Next() {
		var t deviceTokenResponse
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt); err != nil {
			http.Error(w, "could not load device tokens", http.StatusInternalServerError)
			return
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load device tokens", http.StatusInternalServerError)
		return
	}
	writeJSON(w, tokens)
}

func (h *Handlers) createDeviceToken(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserID(r.Context())

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len([]rune(name)) > 60 {
		http.Error(w, "name must be 1..60 characters", http.StatusBadRequest)
		return
	}

	token, created, err := insertDeviceToken(r.Context(), h.pool, userID, name)
	if err != nil {
		http.Error(w, "could not create device token", http.StatusInternalServerError)
		return
	}
	created.Token = token

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

func (h *Handlers) revokeDeviceToken(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserID(r.Context())

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid token id", http.StatusBadRequest)
		return
	}

	// The user_id filter is what stops one account revoking another's token.
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM device_tokens WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		http.Error(w, "could not revoke device token", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
