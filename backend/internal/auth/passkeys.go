package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Passkey self-management: add a second (or third...) passkey to an
// already-registered account, list them, delete one. Distinct from
// beginRegistration/finishRegistration in handlers.go, which create a new
// USER from an invite — these operate on the signed-in user's existing row.
type passkeyResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (h *Handlers) listPasskeys(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserID(r.Context())

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, created_at, last_used_at FROM webauthn_credentials
		 WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		http.Error(w, "could not load passkeys", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	passkeys := []passkeyResponse{}
	for rows.Next() {
		var p passkeyResponse
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &p.LastUsedAt); err != nil {
			http.Error(w, "could not load passkeys", http.StatusInternalServerError)
			return
		}
		passkeys = append(passkeys, p)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not load passkeys", http.StatusInternalServerError)
		return
	}
	writeJSON(w, passkeys)
}

func (h *Handlers) beginAddPasskey(w http.ResponseWriter, r *http.Request) {
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

	var username, displayName string
	var webAuthnID []byte
	if err := h.pool.QueryRow(r.Context(),
		`SELECT username, display_name, webauthn_user_handle FROM users WHERE id = $1`, userID,
	).Scan(&username, &displayName, &webAuthnID); err != nil {
		http.Error(w, "could not load user", http.StatusInternalServerError)
		return
	}

	existing, err := loadCredentials(r.Context(), h.pool, userID)
	if err != nil {
		http.Error(w, "could not load existing passkeys", http.StatusInternalServerError)
		return
	}
	// Without this, an authenticator that's already registered could be
	// re-registered as a second row for the same user — WithExclusions is
	// what makes the platform picker grey it out / refuse it up front.
	exclude := make([]protocol.CredentialDescriptor, len(existing))
	for i, c := range existing {
		exclude[i] = c.Descriptor()
	}

	user := &webauthnUser{id: webAuthnID, username: username, displayName: displayName}
	creation, session, err := h.wa.BeginRegistration(user, webauthn.WithExclusions(exclude))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ceremonyID := h.addPasskeys.put(addPasskeyCeremony{
		session:     *session,
		userID:      userID,
		webAuthnID:  webAuthnID,
		username:    username,
		displayName: displayName,
		name:        name,
	})
	setCeremonyCookie(w, ceremonyID)
	writeJSON(w, creation)
}

func (h *Handlers) finishAddPasskey(w http.ResponseWriter, r *http.Request) {
	ceremonyID, ok := ceremonyCookie(r)
	if !ok {
		http.Error(w, "missing or expired ceremony", http.StatusBadRequest)
		return
	}
	ceremony, ok := h.addPasskeys.take(ceremonyID)
	if !ok {
		http.Error(w, "missing or expired ceremony", http.StatusBadRequest)
		return
	}
	clearCeremonyCookie(w)

	user := &webauthnUser{id: ceremony.webAuthnID, username: ceremony.username, displayName: ceremony.displayName}
	cred, err := h.wa.FinishRegistration(user, ceremony.session, r)
	if err != nil {
		log.Printf("auth: finishAddPasskey: user_id=%d FinishRegistration failed: %v", ceremony.userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	transports := make([]string, len(cred.Transport))
	for i, t := range cred.Transport {
		transports[i] = string(t)
	}

	var created passkeyResponse
	if err := h.pool.QueryRow(r.Context(),
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, backup_eligible, backup_state, name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, name, created_at, last_used_at`,
		ceremony.userID, cred.ID, cred.PublicKey, cred.AttestationType, cred.Authenticator.AAGUID, cred.Authenticator.SignCount, transports,
		cred.Flags.BackupEligible, cred.Flags.BackupState, ceremony.name,
	).Scan(&created.ID, &created.Name, &created.CreatedAt, &created.LastUsedAt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("auth: finishAddPasskey: user_id=%d added passkey %q", ceremony.userID, ceremony.name)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

func (h *Handlers) deletePasskey(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserID(r.Context())

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid passkey id", http.StatusBadRequest)
		return
	}

	// Atomic guard against deleting a rider's last passkey: there is no
	// password or email fallback in this app (#569), so a lockout here is
	// permanent. The count check runs inside the same DELETE statement so
	// two concurrent deletes of "the last two" can't both slip through.
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM webauthn_credentials
		 WHERE id = $1 AND user_id = $2
		   AND (SELECT count(*) FROM webauthn_credentials WHERE user_id = $2) > 1`,
		id, userID)
	if err != nil {
		http.Error(w, "could not delete passkey", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		h.pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM webauthn_credentials WHERE id = $1 AND user_id = $2)`,
			id, userID,
		).Scan(&exists)
		if exists {
			http.Error(w, "cannot delete your only passkey — there is no password or email fallback", http.StatusConflict)
		} else {
			http.Error(w, "passkey not found", http.StatusNotFound)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
