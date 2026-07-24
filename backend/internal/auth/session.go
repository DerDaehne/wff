package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sessionCookieName = "wff_session"
	sessionTTL        = 30 * 24 * time.Hour
)

type contextKey int

const userIDContextKey contextKey = 0

// cookieSecure defaults to true (required for real deployments behind TLS).
// Set COOKIE_SECURE=false only for plain-http local development.
func cookieSecure() bool {
	return os.Getenv("COOKIE_SECURE") != "false"
}

func createSession(ctx context.Context, pool *pgxpool.Pool, w http.ResponseWriter, userID int64) error {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	hash := sha256.Sum256(token)
	expiresAt := time.Now().Add(sessionTTL)
	if _, err := pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)`,
		hash[:], userID, expiresAt,
	); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(token),
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func Logout(ctx context.Context, pool *pgxpool.Pool, r *http.Request, w http.ResponseWriter) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if token, err := base64.RawURLEncoding.DecodeString(cookie.Value); err == nil {
			hash := sha256.Sum256(token)
			pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, hash[:])
		}
	}
	clearSessionCookie(w)
}

// RequireAuth resolves the session cookie against the sessions table and
// attaches user_id to the request context. Every DB query downstream must
// filter by this user_id — this is the sole enforcement point for
// per-user data isolation (no Postgres RLS, see arch-wff-datenmodell).
func RequireAuth(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			hash := sha256.Sum256(token)

			var userID int64
			var expiresAt time.Time
			err = pool.QueryRow(r.Context(),
				`SELECT user_id, expires_at FROM sessions WHERE id = $1`, hash[:],
			).Scan(&userID, &expiresAt)
			if err != nil || time.Now().After(expiresAt) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			newExpiry := time.Now().Add(sessionTTL)
			pool.Exec(r.Context(),
				`UPDATE sessions SET last_seen_at = now(), expires_at = $2 WHERE id = $1`,
				hash[:], newExpiry,
			)

			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDContextKey).(int64)
	return id, ok
}
