package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID              int64
	Username        string
	DisplayName     string
	CreatedAt       time.Time
	CredentialCount int
}

// ListUsers returns every user, oldest first, with how many passkey
// credentials each has registered — 0 means an invite was created but never
// redeemed into a real registration (shouldn't normally happen: the users
// row is only created in FinishRegistration, alongside the credential).
func ListUsers(ctx context.Context, pool *pgxpool.Pool) ([]User, error) {
	rows, err := pool.Query(ctx, `
		SELECT u.id, u.username, u.display_name, u.created_at, count(c.id)
		FROM users u
		LEFT JOIN webauthn_credentials c ON c.user_id = u.id
		GROUP BY u.id
		ORDER BY u.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.CredentialCount); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// DeleteUser removes a user by username. Cascades via FK constraints to
// webauthn_credentials, sessions, and activities (which cascades further to
// samples/enrichment) — see arch-wff-datenmodell. Returns pgx.ErrNoRows if
// no such user exists.
func DeleteUser(ctx context.Context, pool *pgxpool.Pool, username string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM users WHERE username = $1`, username)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
