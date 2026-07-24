package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInviteInvalid = errors.New("invite invalid, expired, or already used")

const InviteTTL = 72 * time.Hour

// CreateInvite generates a single-use invite token. Only the SHA-256 hash is
// stored — the raw token (needed to redeem the invite) is returned once and
// must be handed to the invitee out-of-band (it is not recoverable from the DB).
func CreateInvite(ctx context.Context, pool *pgxpool.Pool, username, displayName string) (token string, err error) {
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		return "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256(raw)
	_, err = pool.Exec(ctx,
		`INSERT INTO invites (token_hash, username, display_name, expires_at) VALUES ($1, $2, $3, $4)`,
		hash[:], username, displayName, time.Now().Add(InviteTTL),
	)
	return token, err
}

type inviteRow struct {
	id          int64
	username    string
	displayName string
}

func lookupInvite(ctx context.Context, pool *pgxpool.Pool, token string) (*inviteRow, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInviteInvalid
	}
	hash := sha256.Sum256(raw)
	row := &inviteRow{}
	err = pool.QueryRow(ctx,
		`SELECT id, username, display_name FROM invites
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`,
		hash[:],
	).Scan(&row.id, &row.username, &row.displayName)
	if err != nil {
		return nil, ErrInviteInvalid
	}
	return row, nil
}
