package activities_test

// End-to-end test against a real Postgres instance and a real HTTP server.
// Skipped if DATABASE_URL is unset — see backend/README.md for the
// scratch-cluster invocation.

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
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/activities"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/descope/virtualwebauthn"
)

type shareStatusDTO struct {
	Active    bool       `json:"active"`
	Token     *string    `json:"token"`
	CreatedAt *time.Time `json:"created_at"`
}

func TestRideShareLifecycle(t *testing.T) {
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

	weatherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hourly":{"time":[],"temperature_2m":[],"wind_speed_10m":[],"wind_direction_10m":[],"precipitation":[]}}`))
	}))
	defer weatherServer.Close()

	mux := http.NewServeMux()
	auth.NewHandlers(pool, wa).Register(mux)
	activities.NewHandlers(pool, t.TempDir(), openmeteo.New(weatherServer.URL)).Register(mux)
	server.Config.Handler = mux
	server.Start()
	defer server.Close()

	rp := virtualwebauthn.RelyingParty{Name: "WFF", ID: "localhost", Origin: server.URL}
	stamp := time.Now().UnixNano()

	register := func(username string) *http.Client {
		token, err := auth.CreateInvite(ctx, pool, username, username)
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
		postMultipart(t, client, server.URL+"/auth/invite/"+token, "application/json", []byte(attestationResponse), http.StatusCreated)
		return client
	}

	owner := register(fmt.Sprintf("share-owner-%d", stamp))
	other := register(fmt.Sprintf("share-other-%d", stamp))

	created := time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(626262, created, 10)
	body, status := doUploadMultipart(t, owner, server.URL, validFIT)
	if status != http.StatusCreated {
		t.Fatalf("upload: status = %d, body: %s", status, body)
	}
	var uploaded struct {
		ActivityID int64 `json:"activity_id"`
	}
	if err := json.Unmarshal(body, &uploaded); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	shareURL := fmt.Sprintf("%s/api/activities/%d/share", server.URL, uploaded.ActivityID)

	t.Run("no share exists yet", func(t *testing.T) {
		var s shareStatusDTO
		json.Unmarshal([]byte(getBody(t, owner, shareURL, http.StatusOK)), &s)
		if s.Active {
			t.Fatal("share reported active before one was ever created")
		}
	})

	var firstToken string
	t.Run("creating a share returns an active token", func(t *testing.T) {
		var s shareStatusDTO
		json.Unmarshal([]byte(postAndGetBody(t, owner, shareURL, http.StatusOK)), &s)
		if !s.Active || s.Token == nil || *s.Token == "" {
			t.Fatalf("share status after create = %+v, want active with a token", s)
		}
		firstToken = *s.Token
	})

	t.Run("creating a share again is idempotent", func(t *testing.T) {
		var s shareStatusDTO
		json.Unmarshal([]byte(postAndGetBody(t, owner, shareURL, http.StatusOK)), &s)
		if s.Token == nil || *s.Token != firstToken {
			t.Fatalf("second create returned a different token: %+v (want %q)", s, firstToken)
		}
	})

	t.Run("public endpoint returns stats only, no other rider's fields leak in", func(t *testing.T) {
		publicBody := getBody(t, &http.Client{}, server.URL+"/api/share/"+firstToken, http.StatusOK)
		var raw map[string]any
		if err := json.Unmarshal([]byte(publicBody), &raw); err != nil {
			t.Fatalf("decode public share: %v (body: %s)", err, publicBody)
		}
		wantKeys := []string{"started_at", "sport", "moving_seconds", "distance_meters", "elevation_gain_meters", "training_stress_score"}
		if len(raw) != len(wantKeys) {
			t.Fatalf("public share has %d fields %v, want exactly %v", len(raw), keysOf(raw), wantKeys)
		}
		for _, k := range wantKeys {
			if _, ok := raw[k]; !ok {
				t.Errorf("public share missing field %q: %s", k, publicBody)
			}
		}
		if _, ok := raw["lat"]; ok {
			t.Error("public share exposes lat — GPS must never appear in the public DTO")
		}
	})

	t.Run("public endpoint works without any auth", func(t *testing.T) {
		anon := &http.Client{}
		resp, err := anon.Get(server.URL + "/api/share/" + firstToken)
		if err != nil {
			t.Fatalf("GET public share: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("anonymous GET public share: status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("another rider cannot see or manage this rider's share", func(t *testing.T) {
		resp, err := other.Get(shareURL)
		if err != nil {
			t.Fatalf("GET share as other rider: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("other rider GET share: status = %d, want 404", resp.StatusCode)
		}

		req, _ := http.NewRequest(http.MethodDelete, shareURL, nil)
		delResp, err := other.Do(req)
		if err != nil {
			t.Fatalf("DELETE share as other rider: %v", err)
		}
		defer delResp.Body.Close()
		if delResp.StatusCode != http.StatusNotFound {
			t.Fatalf("other rider DELETE share: status = %d, want 404", delResp.StatusCode)
		}
	})

	t.Run("revoking turns the public link into a 404, not stale data", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, shareURL, nil)
		resp, err := owner.Do(req)
		if err != nil {
			t.Fatalf("DELETE share: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("DELETE share: status = %d, want 200", resp.StatusCode)
		}

		var s shareStatusDTO
		json.Unmarshal([]byte(getBody(t, owner, shareURL, http.StatusOK)), &s)
		if s.Active {
			t.Fatal("share still reports active after revoke")
		}

		anon := &http.Client{}
		publicResp, err := anon.Get(server.URL + "/api/share/" + firstToken)
		if err != nil {
			t.Fatalf("GET revoked public share: %v", err)
		}
		defer publicResp.Body.Close()
		if publicResp.StatusCode != http.StatusNotFound {
			t.Fatalf("revoked share: status = %d, want 404", publicResp.StatusCode)
		}
	})

	t.Run("a nonexistent token 404s", func(t *testing.T) {
		anon := &http.Client{}
		resp, err := anon.Get(server.URL + "/api/share/this-token-does-not-exist")
		if err != nil {
			t.Fatalf("GET nonexistent share: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("nonexistent token: status = %d, want 404", resp.StatusCode)
		}
	})
}

func postAndGetBody(t *testing.T, client *http.Client, url string, wantStatus int) string {
	t.Helper()
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s = %d, want %d, body: %s", url, resp.StatusCode, wantStatus, body)
	}
	return string(body)
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
