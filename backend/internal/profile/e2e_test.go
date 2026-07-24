package profile_test

// End-to-end test against a real Postgres instance and a real HTTP server,
// reusing the passkey-login pattern from #551.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/profile"
	"github.com/descope/virtualwebauthn"
)

func TestSettingsGetAndUpdate(t *testing.T) {
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
	profile.NewHandlers(pool).Register(mux)
	server.Config.Handler = mux
	server.Start()
	defer server.Close()

	rp := virtualwebauthn.RelyingParty{Name: "WFF", ID: "localhost", Origin: server.URL}
	stamp := time.Now().UnixNano()
	username := fmt.Sprintf("settings-test-%d", stamp)

	token, err := auth.CreateInvite(ctx, pool, username, "Settings Test")
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	attestationBody := getBody(t, client, server.URL+"/auth/invite/"+token, http.StatusOK)
	attestationOptions, err := virtualwebauthn.ParseAttestationOptions(attestationBody)
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	authenticator := virtualwebauthn.NewAuthenticator()
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, authenticator, credential, *attestationOptions)
	postJSON(t, client, server.URL+"/auth/invite/"+token, attestationResponse, http.StatusCreated)

	// Before any settings are configured, both fields must be null - callers
	// downstream (analyze pipeline) rely on this to skip calculation cleanly.
	initial := getJSON(t, client, server.URL+"/api/me/settings")
	if initial["ftp_watts"] != nil || initial["lthr_bpm"] != nil {
		t.Fatalf("initial settings = %v, want both fields null", initial)
	}

	// Unauthenticated access must be rejected.
	anon := &http.Client{}
	resp, err := anon.Get(server.URL + "/api/me/settings")
	if err != nil {
		t.Fatalf("GET as anon: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET as anon: status = %d, want 401", resp.StatusCode)
	}

	// Set FTP only; LTHR must remain untouched (partial PATCH).
	updated := patchJSON(t, client, server.URL+"/api/me/settings", `{"ftp_watts":250}`)
	if v, ok := updated["ftp_watts"].(float64); !ok || v != 250 {
		t.Fatalf("after setting ftp_watts: got %v, want 250", updated["ftp_watts"])
	}
	if updated["lthr_bpm"] != nil {
		t.Fatalf("after setting only ftp_watts: lthr_bpm = %v, want still null", updated["lthr_bpm"])
	}

	// Now set LTHR too; FTP must be preserved.
	updated = patchJSON(t, client, server.URL+"/api/me/settings", `{"lthr_bpm":165}`)
	if v, ok := updated["ftp_watts"].(float64); !ok || v != 250 {
		t.Fatalf("ftp_watts not preserved across partial PATCH: got %v, want 250", updated["ftp_watts"])
	}
	if v, ok := updated["lthr_bpm"].(float64); !ok || v != 165 {
		t.Fatalf("lthr_bpm = %v, want 165", updated["lthr_bpm"])
	}

	// Invalid values are rejected.
	resp2, err := client.Do(mustPatchRequest(t, server.URL+"/api/me/settings", `{"ftp_watts":-5}`))
	if err != nil {
		t.Fatalf("PATCH invalid ftp_watts: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH negative ftp_watts: status = %d, want 400", resp2.StatusCode)
	}
}

func getBody(t *testing.T, client *http.Client, url string, wantStatus int) string {
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

func getJSON(t *testing.T, client *http.Client, url string) map[string]any {
	t.Helper()
	body := getBody(t, client, url, http.StatusOK)
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode JSON from %s: %v (body: %s)", url, err, body)
	}
	return v
}

func postJSON(t *testing.T, client *http.Client, url, body string, wantStatus int) {
	t.Helper()
	resp, err := client.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s = %d, want %d, body: %s", url, resp.StatusCode, wantStatus, respBody)
	}
}

func mustPatchRequest(t *testing.T, url, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("build PATCH request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func patchJSON(t *testing.T, client *http.Client, url, body string) map[string]any {
	t.Helper()
	resp, err := client.Do(mustPatchRequest(t, url, body))
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH %s = %d, want 200, body: %s", url, resp.StatusCode, respBody)
	}
	var v map[string]any
	if err := json.Unmarshal(respBody, &v); err != nil {
		t.Fatalf("decode JSON from PATCH %s: %v (body: %s)", url, err, respBody)
	}
	return v
}
