package auth_test

// Full end-to-end verification of the passkey registration/login flow against
// a real Postgres instance and real WebAuthn signature verification (via a
// virtual authenticator that signs real challenges — not mocked out). Needs
// DATABASE_URL pointing at a migrated (000001..000004) scratch database;
// skipped otherwise since this is an integration test, not a unit test.
//
//	DATABASE_URL=postgres://wff@/wff?host=/tmp/wffpgXXXX go test ./internal/auth/... -run TestPasskeyFlow -v

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/descope/virtualwebauthn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPasskeyFlow(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping live-Postgres integration test")
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := httptest.NewUnstartedServer(nil)
	server.Listener.Close()
	server.Listener = listener

	os.Setenv("WEBAUTHN_RPID", "localhost")
	os.Setenv("WEBAUTHN_ORIGIN", "http://"+listener.Addr().String())
	os.Setenv("COOKIE_SECURE", "false") // httptest server is plain HTTP

	wa, err := auth.NewWebAuthn()
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}
	mux := http.NewServeMux()
	auth.NewHandlers(pool, wa).Register(mux)
	server.Config.Handler = mux
	server.Start()
	defer server.Close()

	rp := virtualwebauthn.RelyingParty{Name: "WFF", ID: "localhost", Origin: server.URL}

	stamp := time.Now().UnixNano()
	userA := registerUser(t, ctx, pool, server.URL, rp, fmt.Sprintf("alice-%d", stamp), "Alice")
	userB := registerUser(t, ctx, pool, server.URL, rp, fmt.Sprintf("bob-%d", stamp), "Bob")

	if userA.id == userB.id {
		t.Fatalf("two distinct registrations produced the same user_id %d", userA.id)
	}

	// Logout, then log back in via the passkey login ceremony (not just the
	// session created during registration) for user A.
	mustPost(t, userA.client, server.URL+"/auth/logout", "", http.StatusNoContent)
	mustGetStatus(t, userA.client, server.URL+"/api/me", http.StatusUnauthorized)

	loginBody := mustPostBody(t, userA.client, server.URL+"/auth/login/begin",
		fmt.Sprintf(`{"username":%q}`, userA.username), http.StatusOK)
	assertionOptions, err := virtualwebauthn.ParseAssertionOptions(loginBody)
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	assertionResponse := virtualwebauthn.CreateAssertionResponse(rp, userA.authenticator, userA.credential, *assertionOptions)
	mustPost(t, userA.client, server.URL+"/auth/login/finish", assertionResponse, http.StatusOK)

	meA := mustGetJSON(t, userA.client, server.URL+"/api/me")
	if meA["user_id"] != float64(userA.id) {
		t.Fatalf("after passkey login: /api/me = %v, want user_id %d", meA, userA.id)
	}

	// Isolation: user A's re-established session must still only see user A,
	// user B's untouched session must still only see user B — no bleed-through.
	meB := mustGetJSON(t, userB.client, server.URL+"/api/me")
	if meB["user_id"] != float64(userB.id) {
		t.Fatalf("user B session leaked: /api/me = %v, want user_id %d", meB, userB.id)
	}
}

// Any registered person can invite another — there is no admin role (#702).
func TestCreateInviteEndpoint(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping live-Postgres integration test")
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := httptest.NewUnstartedServer(nil)
	server.Listener.Close()
	server.Listener = listener

	os.Setenv("WEBAUTHN_RPID", "localhost")
	os.Setenv("WEBAUTHN_ORIGIN", "http://"+listener.Addr().String())
	os.Setenv("COOKIE_SECURE", "false")

	wa, err := auth.NewWebAuthn()
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}
	mux := http.NewServeMux()
	auth.NewHandlers(pool, wa).Register(mux)
	server.Config.Handler = mux
	server.Start()
	defer server.Close()

	rp := virtualwebauthn.RelyingParty{Name: "WFF", ID: "localhost", Origin: server.URL}
	stamp := time.Now().UnixNano()
	inviter := registerUser(t, ctx, pool, server.URL, rp, fmt.Sprintf("inviter-%d", stamp), "Inviter")

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		anon := &http.Client{}
		mustGetStatus(t, anon, server.URL+"/api/me", http.StatusUnauthorized) // sanity: anon really isn't logged in
		resp, err := anon.Post(server.URL+"/api/invites", "application/json",
			strings.NewReader(fmt.Sprintf(`{"username":"anon-%d","display_name":"Anon"}`, stamp)))
		if err != nil {
			t.Fatalf("POST /api/invites: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated POST /api/invites: status = %d, want 401", resp.StatusCode)
		}
	})

	newUsername := fmt.Sprintf("invitee-%d", stamp)
	var inviteToken string

	t.Run("a signed-in rider can invite someone new", func(t *testing.T) {
		body := mustPostBody(t, inviter.client, server.URL+"/api/invites",
			fmt.Sprintf(`{"username":%q,"display_name":"Invitee"}`, newUsername), http.StatusCreated)
		var decoded struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
		}
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			t.Fatalf("decode invite response: %v (body: %s)", err, body)
		}
		if decoded.Token == "" {
			t.Fatalf("invite response has no token: %s", body)
		}
		if decoded.ExpiresAt == "" {
			t.Fatalf("invite response has no expires_at: %s", body)
		}
		inviteToken = decoded.Token
	})

	t.Run("the minted token actually redeems into a working account", func(t *testing.T) {
		invitee := redeemInvite(t, server.URL, rp, inviteToken, newUsername)
		if invitee.id == inviter.id {
			t.Fatalf("invitee got the inviter's own user_id")
		}
	})

	t.Run("a second invite for the now-taken username is rejected up front", func(t *testing.T) {
		resp, err := inviter.client.Post(server.URL+"/api/invites", "application/json",
			strings.NewReader(fmt.Sprintf(`{"username":%q,"display_name":"Invitee Two"}`, newUsername)))
		if err != nil {
			t.Fatalf("POST /api/invites: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("invite for taken username: status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("missing fields are rejected", func(t *testing.T) {
		resp, err := inviter.client.Post(server.URL+"/api/invites", "application/json", strings.NewReader(`{"username":"","display_name":""}`))
		if err != nil {
			t.Fatalf("POST /api/invites: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invite with empty fields: status = %d, want 400", resp.StatusCode)
		}
	})
}

type registeredUser struct {
	id            int64
	username      string
	client        *http.Client
	authenticator virtualwebauthn.Authenticator
	credential    virtualwebauthn.Credential
}

func registerUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, baseURL string, rp virtualwebauthn.RelyingParty, username, displayName string) *registeredUser {
	t.Helper()

	token, err := auth.CreateInvite(ctx, pool, username, displayName)
	if err != nil {
		t.Fatalf("CreateInvite(%s): %v", username, err)
	}
	return redeemInvite(t, baseURL, rp, token, username)
}

// redeemInvite runs the WebAuthn registration ceremony against an
// already-minted token — split out of registerUser so a test can verify a
// token that came from somewhere else (the HTTP invite-creation endpoint,
// not a direct CreateInvite call) actually works end to end.
func redeemInvite(t *testing.T, baseURL string, rp virtualwebauthn.RelyingParty, token, username string) *registeredUser {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	attestationBody := mustGetBody(t, client, baseURL+"/auth/invite/"+token, http.StatusOK)
	attestationOptions, err := virtualwebauthn.ParseAttestationOptions(attestationBody)
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}

	// Real passkeys are discoverable/resident credentials: the authenticator
	// echoes back the registration-time user handle as userHandle on every
	// later assertion, and the RP must verify it still matches. Without this,
	// the virtual authenticator never sends a userHandle at all, so a server
	// bug that recomputes a different WebAuthnID at login (rather than
	// reusing the one from registration) would go completely unnoticed here.
	//
	// BackupEligible: true matches the vast majority of real passkeys today
	// (iCloud Keychain, Google Password Manager, most password managers all
	// create sync/backup-eligible credentials by default). go-webauthn
	// requires this flag to stay identical between registration and every
	// login; the library's zero-value default (false) would mask a server
	// bug that fails to persist and reload it correctly.
	authenticator := virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{
		UserHandle:     []byte(attestationOptions.UserID),
		BackupEligible: true,
	})
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, authenticator, credential, *attestationOptions)

	mustPost(t, client, baseURL+"/auth/invite/"+token, attestationResponse, http.StatusCreated)
	authenticator.AddCredential(credential)

	me := mustGetJSON(t, client, baseURL+"/api/me")
	id, ok := me["user_id"].(float64)
	if !ok {
		t.Fatalf("/api/me returned unexpected body: %v", me)
	}

	return &registeredUser{
		id: int64(id), username: username, client: client,
		authenticator: authenticator, credential: credential,
	}
}

func mustGetBody(t *testing.T, client *http.Client, url string, wantStatus int) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s = %d, want %d, body: %s", url, resp.StatusCode, wantStatus, body)
	}
	return string(body)
}

func mustGetStatus(t *testing.T, client *http.Client, url string, wantStatus int) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s = %d, want %d, body: %s", url, resp.StatusCode, wantStatus, body)
	}
}

func mustGetJSON(t *testing.T, client *http.Client, url string) map[string]any {
	t.Helper()
	body := mustGetBody(t, client, url, http.StatusOK)
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode JSON from %s: %v (body: %s)", url, err, body)
	}
	return v
}

func mustPost(t *testing.T, client *http.Client, url, body string, wantStatus int) {
	t.Helper()
	mustPostBody(t, client, url, body, wantStatus)
}

func mustPostBody(t *testing.T, client *http.Client, url, body string, wantStatus int) string {
	t.Helper()
	resp, err := client.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s = %d, want %d, body: %s", url, resp.StatusCode, wantStatus, respBody)
	}
	return string(respBody)
}
