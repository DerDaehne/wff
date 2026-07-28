package activities_test

// End-to-end test against a real Postgres instance and a real HTTP server:
// real upload, real files on disk, real ZIP decoding. Skipped if
// DATABASE_URL is unset — see backend/README.md for the scratch-cluster
// invocation.

import (
	"archive/zip"
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

	"github.com/DerDaehne/wff/internal/activities"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/descope/virtualwebauthn"
)

func TestExportEndpoints(t *testing.T) {
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

	rider := register(fmt.Sprintf("export-rider-%d", stamp))
	otherRider := register(fmt.Sprintf("export-other-%d", stamp))

	created := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(444555, created, 10)

	body, status := doUploadMultipart(t, rider, server.URL, validFIT)
	if status != http.StatusCreated {
		t.Fatalf("upload: status = %d, body: %s", status, body)
	}
	var uploaded struct {
		ActivityID int64 `json:"activity_id"`
	}
	if err := json.Unmarshal(body, &uploaded); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}

	t.Run("single-ride export returns the original bytes", func(t *testing.T) {
		exportURL := fmt.Sprintf("%s/api/activities/%d/export", server.URL, uploaded.ActivityID)
		resp, err := rider.Get(exportURL)
		if err != nil {
			t.Fatalf("GET export: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if disp := resp.Header.Get("Content-Disposition"); disp == "" {
			t.Error("no Content-Disposition header — browser would not treat this as a download")
		}
		got, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !bytes.Equal(got, validFIT) {
			t.Errorf("exported file (%d bytes) does not match the originally uploaded file (%d bytes)", len(got), len(validFIT))
		}
	})

	t.Run("single-ride export 404s for another rider's activity", func(t *testing.T) {
		exportURL := fmt.Sprintf("%s/api/activities/%d/export", server.URL, uploaded.ActivityID)
		resp, err := otherRider.Get(exportURL)
		if err != nil {
			t.Fatalf("GET export: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("other rider: status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("single-ride export 404s for a nonexistent activity", func(t *testing.T) {
		resp, err := rider.Get(server.URL + "/api/activities/999999999/export")
		if err != nil {
			t.Fatalf("GET export: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("nonexistent activity: status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("full account export is a ZIP with profile, activities and the raw file", func(t *testing.T) {
		resp, err := rider.Get(server.URL + "/api/me/export")
		if err != nil {
			t.Fatalf("GET /api/me/export: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
			t.Errorf("Content-Type = %q, want application/zip", ct)
		}

		zipBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatalf("not a valid ZIP: %v", err)
		}

		files := map[string]*zip.File{}
		for _, f := range zr.File {
			files[f.Name] = f
		}
		if _, ok := files["profil.json"]; !ok {
			t.Error("ZIP has no profil.json")
		}
		fahrtenJSON, ok := files["fahrten.json"]
		if !ok {
			t.Fatal("ZIP has no fahrten.json")
		}
		rc, err := fahrtenJSON.Open()
		if err != nil {
			t.Fatalf("open fahrten.json: %v", err)
		}
		var listed []struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(rc).Decode(&listed); err != nil {
			t.Fatalf("decode fahrten.json: %v", err)
		}
		rc.Close()
		found := false
		for _, a := range listed {
			if a.ID == uploaded.ActivityID {
				found = true
			}
		}
		if !found {
			t.Errorf("uploaded activity %d not present in fahrten.json", uploaded.ActivityID)
		}

		rawName := fmt.Sprintf("fahrten/2026-07-22-%d.fit", uploaded.ActivityID)
		rawFile, ok := files[rawName]
		if !ok {
			var names []string
			for name := range files {
				names = append(names, name)
			}
			t.Fatalf("ZIP has no %q — entries: %v", rawName, names)
		}
		rrc, err := rawFile.Open()
		if err != nil {
			t.Fatalf("open %s: %v", rawName, err)
		}
		rawGot, err := io.ReadAll(rrc)
		rrc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", rawName, err)
		}
		if !bytes.Equal(rawGot, validFIT) {
			t.Errorf("%s (%d bytes) does not match the originally uploaded file (%d bytes)", rawName, len(rawGot), len(validFIT))
		}
	})
}
