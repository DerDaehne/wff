package activities_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/activities"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/descope/virtualwebauthn"
)

// A device token lives in cleartext inside an iOS Shortcut on a phone that
// syncs through iCloud (#617). Its whole justification is that it can do one
// thing and nothing else, so the interesting assertions here are the negative
// ones: everything except uploading must come back 401, and a revoked token
// must stop working immediately.
func TestDeviceTokenScope(t *testing.T) {
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
	username := fmt.Sprintf("device-token-test-%d", time.Now().UnixNano())
	invite, err := auth.CreateInvite(ctx, pool, username, "Device Token Test")
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}

	attestationBody := getBody(t, browser, server.URL+"/auth/invite/"+invite, http.StatusOK)
	attestationOptions, err := virtualwebauthn.ParseAttestationOptions(attestationBody)
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	authenticator := virtualwebauthn.NewAuthenticator()
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, authenticator, credential, *attestationOptions)
	postMultipart(t, browser, server.URL+"/auth/invite/"+invite, "application/json", []byte(attestationResponse), http.StatusCreated)

	// The phone: no cookie jar at all, only the Authorization header — exactly
	// what a Shortcut sends.
	phone := &http.Client{}
	withToken := func(t *testing.T, method, url, token string, body *strings.Reader) *http.Response {
		t.Helper()
		var req *http.Request
		var err error
		if body == nil {
			req, err = http.NewRequest(method, url, nil)
		} else {
			req, err = http.NewRequest(method, url, body)
			req.Header.Set("Content-Type", "application/json")
		}
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := phone.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, url, err)
		}
		return resp
	}

	var created struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	createResp, err := browser.Post(server.URL+"/api/device-tokens", "application/json",
		strings.NewReader(`{"name":"iPhone"}`))
	if err != nil {
		t.Fatalf("create device token: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create device token = %d, want 201", createResp.StatusCode)
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created token: %v", err)
	}
	createResp.Body.Close()
	if !strings.HasPrefix(created.Token, "wff_") {
		t.Fatalf("token = %q, want a recognisable wff_ prefix", created.Token)
	}

	t.Run("the cleartext token is shown only at creation", func(t *testing.T) {
		listed := getBody(t, browser, server.URL+"/api/device-tokens", http.StatusOK)
		if strings.Contains(string(listed), created.Token) {
			t.Error("listing device tokens returned the cleartext token — it must exist exactly once")
		}
		if !strings.Contains(string(listed), "iPhone") {
			t.Errorf("listing = %s, want the device name so the right one can be revoked", listed)
		}
	})

	t.Run("a phone with only the token can upload a ride", func(t *testing.T) {
		contentType, form := buildMultipartUpload(t, fitfixture.ValidActivity(515151, time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC), 15))
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/activities", form)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Authorization", "Bearer "+created.Token)
		resp, err := phone.Do(req)
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201 — this is the one thing the token is for", resp.StatusCode)
		}
	})

	t.Run("the token cannot read the rides it uploads", func(t *testing.T) {
		resp := withToken(t, http.MethodGet, server.URL+"/api/activities", created.Token, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 — upload-only means upload-only", resp.StatusCode)
		}
	})

	t.Run("the token cannot mint another token", func(t *testing.T) {
		resp := withToken(t, http.MethodPost, server.URL+"/api/device-tokens", created.Token,
			strings.NewReader(`{"name":"stolen"}`))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 — otherwise a leaked token renews itself forever", resp.StatusCode)
		}
	})

	t.Run("the token cannot revoke tokens", func(t *testing.T) {
		resp := withToken(t, http.MethodDelete,
			server.URL+"/api/device-tokens/"+strconv.FormatInt(created.ID, 10), created.Token, nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 — a leaked token must not lock the owner out", resp.StatusCode)
		}
	})

	t.Run("a made-up token is refused", func(t *testing.T) {
		resp := withToken(t, http.MethodPost, server.URL+"/api/activities", "wff_not-a-real-token", nil)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("revoking stops the upload immediately", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete,
			server.URL+"/api/device-tokens/"+strconv.FormatInt(created.ID, 10), nil)
		resp, err := browser.Do(req)
		if err != nil {
			t.Fatalf("revoke: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("revoke status = %d, want 204", resp.StatusCode)
		}

		contentType, form := buildMultipartUpload(t, fitfixture.ValidActivity(525252, time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC), 15))
		req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/activities", form)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Authorization", "Bearer "+created.Token)
		after, err := phone.Do(req)
		if err != nil {
			t.Fatalf("upload after revoke: %v", err)
		}
		defer after.Body.Close()
		if after.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 — revocation has to take effect at once", after.StatusCode)
		}
	})
}
