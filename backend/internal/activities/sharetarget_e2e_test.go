package activities_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
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

// The share target's contract is where it sends the browser, not what it
// returns as a body: Android performs a navigation, so the redirect IS the
// user-visible result. Every branch is therefore asserted on the Location
// header — including the failures, which must land somewhere a person can
// read rather than on a bare status code (#617).
func TestShareTargetRedirects(t *testing.T) {
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
	username := fmt.Sprintf("share-test-%d", time.Now().UnixNano())
	token, err := auth.CreateInvite(ctx, pool, username, "Share Test")
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	// Redirects must not be followed: the Location header is what is being
	// tested, and following it would just fetch the SPA shell.
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	attestationBody := getBody(t, client, server.URL+"/auth/invite/"+token, http.StatusOK)
	attestationOptions, err := virtualwebauthn.ParseAttestationOptions(attestationBody)
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	authenticator := virtualwebauthn.NewAuthenticator()
	credential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestationResponse := virtualwebauthn.CreateAttestationResponse(rp, authenticator, credential, *attestationOptions)
	postMultipart(t, client, server.URL+"/auth/invite/"+token, "application/json", []byte(attestationResponse), http.StatusCreated)

	share := func(t *testing.T, c *http.Client, fitBytes []byte) (int, string) {
		t.Helper()
		contentType, body := buildMultipartUpload(t, fitBytes)
		resp, err := c.Post(server.URL+"/share-target", contentType, body)
		if err != nil {
			t.Fatalf("POST /share-target: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Location")
	}

	created := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(424242, created, 15)

	t.Run("shared ride lands on the ride", func(t *testing.T) {
		status, location := share(t, client, validFIT)
		if status != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303 (so a reload doesn't repost the file)", status)
		}
		if !strings.HasPrefix(location, "/rides/") {
			t.Errorf("Location = %q, want the newly created ride", location)
		}
	})

	t.Run("sharing the same ride twice is not an error page", func(t *testing.T) {
		status, location := share(t, client, validFIT)
		if status != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", status)
		}
		if location != "/upload?geteilt=schon-vorhanden" {
			t.Errorf("Location = %q, want the upload page saying it's already there", location)
		}
	})

	t.Run("something that isn't a ride says so", func(t *testing.T) {
		status, location := share(t, client, fitfixture.Truncate(fitfixture.ValidActivity(1, created, 5)))
		if status != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", status)
		}
		if location != "/upload?geteilt=keine-fit-datei" {
			t.Errorf("Location = %q, want the upload page saying it wasn't a .fit", location)
		}
	})

	t.Run("a share from a logged-out browser is refused", func(t *testing.T) {
		anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		status, _ := share(t, anon, validFIT)
		if status != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 — a share must not bypass auth", status)
		}
	})
}
