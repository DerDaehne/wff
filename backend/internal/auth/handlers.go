package auth

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ceremonyCookieName = "wff_ceremony"

type Handlers struct {
	pool          *pgxpool.Pool
	wa            *webauthn.WebAuthn
	registrations *ceremonyStore[registrationCeremony]
	logins        *ceremonyStore[loginCeremony]
}

func NewHandlers(pool *pgxpool.Pool, wa *webauthn.WebAuthn) *Handlers {
	return &Handlers{
		pool:          pool,
		wa:            wa,
		registrations: newCeremonyStore[registrationCeremony](),
		logins:        newCeremonyStore[loginCeremony](),
	}
}

// Register wires the auth endpoints and a protected /api/me probe onto mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/invite/{token}", h.beginRegistration)
	mux.HandleFunc("POST /auth/invite/{token}", h.finishRegistration)
	mux.HandleFunc("POST /auth/login/begin", h.beginLogin)
	mux.HandleFunc("POST /auth/login/finish", h.finishLogin)
	mux.HandleFunc("POST /auth/logout", h.handleLogout)
	mux.Handle("GET /api/me", RequireAuth(h.pool)(http.HandlerFunc(h.whoAmI)))
	// Session only, on purpose: a device token must not be able to mint or
	// revoke device tokens (#617).
	mux.Handle("GET /api/device-tokens", RequireAuth(h.pool)(http.HandlerFunc(h.listDeviceTokens)))
	mux.Handle("POST /api/device-tokens", RequireAuth(h.pool)(http.HandlerFunc(h.createDeviceToken)))
	mux.Handle("DELETE /api/device-tokens/{id}", RequireAuth(h.pool)(http.HandlerFunc(h.revokeDeviceToken)))
}

func (h *Handlers) beginRegistration(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	invite, err := lookupInvite(r.Context(), h.pool, token)
	if err != nil {
		log.Printf("auth: beginRegistration: invite lookup failed: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	webAuthnID := randomBytes(16)
	user := &webauthnUser{id: webAuthnID, username: invite.username, displayName: invite.displayName}

	creation, session, err := h.wa.BeginRegistration(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ceremonyID := h.registrations.put(registrationCeremony{
		session:     *session,
		inviteID:    invite.id,
		webAuthnID:  webAuthnID,
		username:    invite.username,
		displayName: invite.displayName,
	})
	setCeremonyCookie(w, ceremonyID)
	writeJSON(w, creation)
}

func (h *Handlers) finishRegistration(w http.ResponseWriter, r *http.Request) {
	ceremonyID, ok := ceremonyCookie(r)
	if !ok {
		http.Error(w, "missing or expired ceremony", http.StatusBadRequest)
		return
	}
	ceremony, ok := h.registrations.take(ceremonyID)
	if !ok {
		http.Error(w, "missing or expired ceremony", http.StatusBadRequest)
		return
	}
	clearCeremonyCookie(w)

	user := &webauthnUser{id: ceremony.webAuthnID, username: ceremony.username, displayName: ceremony.displayName}
	cred, err := h.wa.FinishRegistration(user, ceremony.session, r)
	if err != nil {
		log.Printf("auth: finishRegistration: user=%s FinishRegistration failed: %v", ceremony.username, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	// Race guard: two concurrent redemptions of the same invite must not both
	// succeed. The UPDATE only affects a row that is still unused.
	tag, err := tx.Exec(ctx,
		`UPDATE invites SET used_at = now() WHERE id = $1 AND used_at IS NULL`, ceremony.inviteID)
	if err != nil || tag.RowsAffected() != 1 {
		http.Error(w, "invite already used", http.StatusConflict)
		return
	}

	var userID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (username, display_name, webauthn_user_handle) VALUES ($1, $2, $3) RETURNING id`,
		ceremony.username, ceremony.displayName, ceremony.webAuthnID,
	).Scan(&userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	transports := make([]string, len(cred.Transport))
	for i, t := range cred.Transport {
		transports[i] = string(t)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, backup_eligible, backup_state)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		userID, cred.ID, cred.PublicKey, cred.AttestationType, cred.Authenticator.AAGUID, cred.Authenticator.SignCount, transports,
		cred.Flags.BackupEligible, cred.Flags.BackupState,
	); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := createSession(ctx, h.pool, w, userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("auth: finishRegistration: user=%s user_id=%d registered", ceremony.username, userID)
	w.WriteHeader(http.StatusCreated)
}

type loginBeginRequest struct {
	Username string `json:"username"`
}

func (h *Handlers) beginLogin(w http.ResponseWriter, r *http.Request) {
	var req loginBeginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var userID int64
	var displayName string
	var webAuthnID []byte
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, display_name, webauthn_user_handle FROM users WHERE username = $1`, req.Username,
	).Scan(&userID, &displayName, &webAuthnID)
	if err != nil {
		log.Printf("auth: beginLogin: user=%s not found: %v", req.Username, err)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	credentials, err := loadCredentials(r.Context(), h.pool, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(credentials) == 0 {
		log.Printf("auth: beginLogin: user=%s user_id=%d has no credentials", req.Username, userID)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	user := &webauthnUser{id: webAuthnID, username: req.Username, displayName: displayName, credentials: credentials}

	assertion, session, err := h.wa.BeginLogin(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ceremonyID := h.logins.put(loginCeremony{
		session:     *session,
		userID:      userID,
		webAuthnID:  webAuthnID,
		username:    req.Username,
		displayName: displayName,
		credentials: credentials,
	})
	setCeremonyCookie(w, ceremonyID)
	writeJSON(w, assertion)
}

func (h *Handlers) finishLogin(w http.ResponseWriter, r *http.Request) {
	ceremonyID, ok := ceremonyCookie(r)
	if !ok {
		http.Error(w, "missing or expired ceremony", http.StatusBadRequest)
		return
	}
	ceremony, ok := h.logins.take(ceremonyID)
	if !ok {
		http.Error(w, "missing or expired ceremony", http.StatusBadRequest)
		return
	}
	clearCeremonyCookie(w)

	user := &webauthnUser{
		id: ceremony.webAuthnID, username: ceremony.username,
		displayName: ceremony.displayName, credentials: ceremony.credentials,
	}
	cred, err := h.wa.FinishLogin(user, ceremony.session, r)
	if err != nil {
		log.Printf("auth: finishLogin: user=%s user_id=%d FinishLogin failed: %v", ceremony.username, ceremony.userID, err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// BackupState (unlike BackupEligible) is allowed to change over a
	// credential's lifetime — e.g. a user enables sync on their password
	// manager after registering — so it's written back on every login,
	// same as sign_count.
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE webauthn_credentials SET sign_count = $2, backup_state = $3, last_used_at = now() WHERE credential_id = $1`,
		cred.ID, cred.Authenticator.SignCount, cred.Flags.BackupState,
	); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := createSession(r.Context(), h.pool, w, ceremony.userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("auth: finishLogin: user=%s user_id=%d logged in", ceremony.username, ceremony.userID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	Logout(r.Context(), h.pool, r, w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) whoAmI(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserID(r.Context())
	writeJSON(w, map[string]any{"user_id": userID})
}

func loadCredentials(ctx context.Context, pool *pgxpool.Pool, userID int64) ([]webauthn.Credential, error) {
	rows, err := pool.Query(ctx,
		`SELECT credential_id, public_key, attestation_type, aaguid, sign_count, transports, backup_eligible, backup_state
		 FROM webauthn_credentials WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []webauthn.Credential
	for rows.Next() {
		var (
			credID, pubKey, aaguid      []byte
			attType                     string
			signCount                   int64
			transportStrs               []string
			backupEligible, backupState bool
		)
		if err := rows.Scan(&credID, &pubKey, &attType, &aaguid, &signCount, &transportStrs, &backupEligible, &backupState); err != nil {
			return nil, err
		}
		transports := make([]protocol.AuthenticatorTransport, len(transportStrs))
		for i, t := range transportStrs {
			transports[i] = protocol.AuthenticatorTransport(t)
		}
		creds = append(creds, webauthn.Credential{
			ID:              credID,
			PublicKey:       pubKey,
			AttestationType: attType,
			Transport:       transports,
			Flags: webauthn.CredentialFlags{
				BackupEligible: backupEligible,
				BackupState:    backupState,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    aaguid,
				SignCount: uint32(signCount),
			},
		})
	}
	return creds, rows.Err()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func setCeremonyCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     ceremonyCookieName,
		Value:    id,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   cookieSecure(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(ceremonyTTL.Seconds()),
	})
}

func ceremonyCookie(r *http.Request) (string, bool) {
	c, err := r.Cookie(ceremonyCookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

func clearCeremonyCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     ceremonyCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   cookieSecure(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
