package activities_test

// End-to-end test against a real Postgres instance and a real HTTP server:
// upload flow through auth (real passkey login, reusing the #551 pattern),
// the actual /api/activities endpoint, and real files on disk. Skipped if
// DATABASE_URL is unset — see backend/README.md for the scratch-cluster
// invocation.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/activities"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/descope/virtualwebauthn"
)

func TestUploadEndpoint(t *testing.T) {
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
	uploadDir := t.TempDir()

	// Fake Open-Meteo server: upload() fires a best-effort enrichment
	// attempt in a background goroutine, which must never reach the real
	// internet during tests. Returning null values ("not yet available")
	// keeps that goroutine fast and deterministic.
	weatherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hourly":{"time":[],"temperature_2m":[],"wind_speed_10m":[],"wind_direction_10m":[],"precipitation":[]}}`))
	}))
	defer weatherServer.Close()

	mux := http.NewServeMux()
	auth.NewHandlers(pool, wa).Register(mux)
	activities.NewHandlers(pool, uploadDir, openmeteo.New(weatherServer.URL)).Register(mux)
	server.Config.Handler = mux
	server.Start()
	defer server.Close()

	rp := virtualwebauthn.RelyingParty{Name: "WFF", ID: "localhost", Origin: server.URL}
	stamp := time.Now().UnixNano()
	username := fmt.Sprintf("upload-test-%d", stamp)

	token, err := auth.CreateInvite(ctx, pool, username, "Upload Test")
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

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(555111, created, 15)

	// Unauthenticated upload must be rejected before touching anything else.
	anon := &http.Client{}
	uploadMultipart(t, anon, server.URL, validFIT, http.StatusUnauthorized)

	body, status := doUploadMultipart(t, client, server.URL, validFIT)
	if status != http.StatusCreated {
		t.Fatalf("upload valid file: status = %d, body: %s", status, body)
	}
	var created201 struct {
		ActivityID int64 `json:"activity_id"`
	}
	if err := json.Unmarshal(body, &created201); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, body)
	}
	if created201.ActivityID == 0 {
		t.Fatalf("activity_id = 0, want non-zero")
	}

	var rawPath string
	if err := pool.QueryRow(ctx, `SELECT raw_file_path FROM activities WHERE id = $1`, created201.ActivityID).Scan(&rawPath); err != nil {
		t.Fatalf("query raw_file_path: %v", err)
	}
	if _, err := os.Stat(rawPath); err != nil {
		t.Fatalf("raw file missing on disk at %q: %v", rawPath, err)
	}
	if filepath.Dir(filepath.Dir(rawPath)) != filepath.Clean(uploadDir) {
		t.Fatalf("raw file %q not under upload dir %q", rawPath, uploadDir)
	}

	// Duplicate upload of the same file must be rejected, and must not leave
	// an orphaned raw file behind.
	filesBefore, _ := os.ReadDir(filepath.Dir(rawPath))
	_, dupStatus := doUploadMultipart(t, client, server.URL, validFIT)
	if dupStatus != http.StatusConflict {
		t.Fatalf("duplicate upload: status = %d, want 409", dupStatus)
	}
	filesAfter, _ := os.ReadDir(filepath.Dir(rawPath))
	if len(filesAfter) != len(filesBefore) {
		t.Fatalf("duplicate upload left an orphaned file: before=%d after=%d", len(filesBefore), len(filesAfter))
	}

	// Corrupted and empty files must be rejected cleanly.
	corrupted := fitfixture.Truncate(fitfixture.ValidActivity(1, created, 5))
	_, corruptStatus := doUploadMultipart(t, client, server.URL, corrupted)
	if corruptStatus != http.StatusBadRequest {
		t.Fatalf("corrupt upload: status = %d, want 400", corruptStatus)
	}
	_, emptyStatus := doUploadMultipart(t, client, server.URL, nil)
	if emptyStatus != http.StatusBadRequest {
		t.Fatalf("empty upload: status = %d, want 400", emptyStatus)
	}
}

// TestUploadTriggersImmediateEnrichment verifies #560's async-trigger half:
// a successful upload kicks off enrichment in the background without
// delaying the HTTP response. Since it's genuinely asynchronous, this polls
// the DB briefly rather than asserting immediately after the response.
func TestUploadTriggersImmediateEnrichment(t *testing.T) {
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

	// This time the fake weather server has data ready immediately, so the
	// background attempt fired by upload() should complete on its own,
	// without waiting for the retry poller.
	weatherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hourly":{"time":["2026-07-20T08:00"],"temperature_2m":[19.5],"wind_speed_10m":[4.0],"wind_direction_10m":[90],"precipitation":[0.2]}}`))
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
	username := fmt.Sprintf("enrich-trigger-test-%d", stamp)

	token, err := auth.CreateInvite(ctx, pool, username, "Enrich Trigger Test")
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

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(777222, created, 15)

	body, status := doUploadMultipart(t, client, server.URL, validFIT)
	if status != http.StatusCreated {
		t.Fatalf("upload: status = %d, body: %s", status, body)
	}
	var created201 struct {
		ActivityID int64 `json:"activity_id"`
	}
	if err := json.Unmarshal(body, &created201); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, body)
	}

	deadline := time.Now().Add(2 * time.Second)
	var rowCount int
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM enrichment WHERE activity_id = $1`, created201.ActivityID,
		).Scan(&rowCount); err != nil {
			t.Fatalf("count enrichment rows: %v", err)
		}
		if rowCount > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rowCount == 0 {
		t.Fatalf("no enrichment row appeared within 2s of upload — immediate background attempt did not run (or did not complete)")
	}
}

// TestListAndSamplesEndpoints covers #572's new read endpoints: the
// Ride-Liste data source (GET /api/activities) and the Ride-Detail data
// source (GET /api/activities/{id}/samples), including the ownership check
// on samples — a second person must not be able to read someone else's ride.
// Also covers the #589 splits endpoint (GET /api/activities/{id}/laps), same
// ownership check.
func TestListAndSamplesEndpoints(t *testing.T) {
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

	registerRider := func(username string) *http.Client {
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

	rider := registerRider(fmt.Sprintf("list-samples-rider-%d", stamp))
	otherRider := registerRider(fmt.Sprintf("list-samples-other-%d", stamp))

	created := time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(999333, created, 20)

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

	t.Run("list returns the rider's own activity", func(t *testing.T) {
		listBody := getBody(t, rider, server.URL+"/api/activities", http.StatusOK)
		var list []struct {
			ID     int64  `json:"id"`
			Sport  string `json:"sport"`
			Moving int    `json:"moving_seconds"`
		}
		if err := json.Unmarshal([]byte(listBody), &list); err != nil {
			t.Fatalf("decode list response: %v (body: %s)", err, listBody)
		}
		found := false
		for _, a := range list {
			if a.ID == uploaded.ActivityID {
				found = true
				if a.Sport != "cycling" {
					t.Fatalf("sport = %q, want cycling", a.Sport)
				}
			}
		}
		if !found {
			t.Fatalf("uploaded activity %d not present in list: %s", uploaded.ActivityID, listBody)
		}
	})

	t.Run("list is empty for a rider with no activities", func(t *testing.T) {
		listBody := getBody(t, otherRider, server.URL+"/api/activities", http.StatusOK)
		var list []any
		if err := json.Unmarshal([]byte(listBody), &list); err != nil {
			t.Fatalf("decode list response: %v (body: %s)", err, listBody)
		}
		if len(list) != 0 {
			t.Fatalf("other rider's list = %v, want empty", list)
		}
	})

	t.Run("samples returns real time-series data for the owner", func(t *testing.T) {
		samplesURL := fmt.Sprintf("%s/api/activities/%d/samples", server.URL, uploaded.ActivityID)
		samplesBody := getBody(t, rider, samplesURL, http.StatusOK)
		var samples []struct {
			Time       time.Time `json:"time"`
			Lat        *float64  `json:"lat"`
			PowerWatts *int      `json:"power_watts"`
		}
		if err := json.Unmarshal([]byte(samplesBody), &samples); err != nil {
			t.Fatalf("decode samples response: %v (body: %s)", err, samplesBody)
		}
		if len(samples) != 20 {
			t.Fatalf("len(samples) = %d, want 20", len(samples))
		}
		if samples[0].PowerWatts == nil {
			t.Fatalf("samples[0].PowerWatts is nil, want a value")
		}
	})

	t.Run("samples 404s for another rider's activity", func(t *testing.T) {
		samplesURL := fmt.Sprintf("%s/api/activities/%d/samples", server.URL, uploaded.ActivityID)
		resp, err := otherRider.Get(samplesURL)
		if err != nil {
			t.Fatalf("GET samples: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("other rider GET samples: status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("samples 404s for a nonexistent activity", func(t *testing.T) {
		samplesURL := fmt.Sprintf("%s/api/activities/999999999/samples", server.URL)
		resp, err := rider.Get(samplesURL)
		if err != nil {
			t.Fatalf("GET samples: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("nonexistent activity: status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("laps returns the fixture's two splits for the owner", func(t *testing.T) {
		lapsURL := fmt.Sprintf("%s/api/activities/%d/laps", server.URL, uploaded.ActivityID)
		lapsBody := getBody(t, rider, lapsURL, http.StatusOK)
		var laps []struct {
			LapIndex       int      `json:"lap_index"`
			ElapsedSeconds int      `json:"elapsed_seconds"`
			AvgPowerWatts  *float64 `json:"avg_power_watts"`
		}
		if err := json.Unmarshal([]byte(lapsBody), &laps); err != nil {
			t.Fatalf("decode laps response: %v (body: %s)", err, lapsBody)
		}
		if len(laps) != 2 {
			t.Fatalf("len(laps) = %d, want 2", len(laps))
		}
		if laps[0].ElapsedSeconds != 10 || laps[0].AvgPowerWatts == nil || *laps[0].AvgPowerWatts != 170 {
			t.Fatalf("laps[0] = %+v, want elapsed=10 avg_power=170", laps[0])
		}
		if laps[1].ElapsedSeconds != 20 {
			t.Fatalf("laps[1].ElapsedSeconds = %d, want 20", laps[1].ElapsedSeconds)
		}
	})

	t.Run("laps 404s for another rider's activity", func(t *testing.T) {
		lapsURL := fmt.Sprintf("%s/api/activities/%d/laps", server.URL, uploaded.ActivityID)
		resp, err := otherRider.Get(lapsURL)
		if err != nil {
			t.Fatalf("GET laps: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("other rider GET laps: status = %d, want 404", resp.StatusCode)
		}
	})
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

func postMultipart(t *testing.T, client *http.Client, url, contentType string, body []byte, wantStatus int) {
	t.Helper()
	resp, err := client.Post(url, contentType, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s = %d, want %d, body: %s", url, resp.StatusCode, wantStatus, respBody)
	}
}

func buildMultipartUpload(t *testing.T, fitBytes []byte) (contentType string, body *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "activity.fit")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(fitBytes); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return w.FormDataContentType(), &buf
}

func doUploadMultipart(t *testing.T, client *http.Client, baseURL string, fitBytes []byte) ([]byte, int) {
	t.Helper()
	contentType, body := buildMultipartUpload(t, fitBytes)
	resp, err := client.Post(baseURL+"/api/activities", contentType, body)
	if err != nil {
		t.Fatalf("POST /api/activities: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode
}

func uploadMultipart(t *testing.T, client *http.Client, baseURL string, fitBytes []byte, wantStatus int) {
	t.Helper()
	_, status := doUploadMultipart(t, client, baseURL, fitBytes)
	if status != wantStatus {
		t.Fatalf("upload: status = %d, want %d", status, wantStatus)
	}
}
