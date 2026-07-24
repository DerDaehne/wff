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

	authenticator := virtualwebauthn.NewAuthenticator()
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
