package analyze_test

// End-to-end test tying together the whole Analyse epic (#550) through real
// HTTP: set FTP, upload a ride, confirm NP/IF/TSS got computed automatically
// (not just unit-tested), then confirm /api/training-load surfaces the
// resulting series + at least one insight. Reuses the passkey-login pattern
// from #551. Skipped if DATABASE_URL is unset.

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
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/activities"
	"github.com/DerDaehne/wff/internal/analyze"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/DerDaehne/wff/internal/profile"
	"github.com/descope/virtualwebauthn"
)

func TestUploadComputesMetricsAndTrainingLoadReflectsThem(t *testing.T) {
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

	// Weather server that never has data yet - keeps the enrichment
	// goroutine fast/deterministic; irrelevant to this test's assertions.
	weatherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hourly":{"time":[],"temperature_2m":[],"wind_speed_10m":[],"wind_direction_10m":[],"precipitation":[]}}`))
	}))
	defer weatherServer.Close()

	mux := http.NewServeMux()
	auth.NewHandlers(pool, wa).Register(mux)
	profile.NewHandlers(pool).Register(mux)
	activities.NewHandlers(pool, t.TempDir(), openmeteo.New(weatherServer.URL)).Register(mux)
	analyze.NewHandlers(pool).Register(mux)
	server.Config.Handler = mux
	server.Start()
	defer server.Close()

	rp := virtualwebauthn.RelyingParty{Name: "WFF", ID: "localhost", Origin: server.URL}
	stamp := time.Now().UnixNano()
	username := fmt.Sprintf("analyze-e2e-%d", stamp)

	token, err := auth.CreateInvite(ctx, pool, username, "Analyze E2E Test")
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
	postRaw(t, client, server.URL+"/auth/invite/"+token, attestationResponse, http.StatusCreated)

	// Configure FTP before uploading — required for the power path to
	// produce a value at all (see arch-wff-analyze).
	patchJSON(t, client, server.URL+"/api/me/settings", `{"ftp_watts":250}`)

	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(888444, created, 3600) // 1h ride
	uploadBody, status := doUploadMultipart(t, client, server.URL, validFIT)
	if status != http.StatusCreated {
		t.Fatalf("upload: status = %d, body: %s", status, uploadBody)
	}
	var uploaded struct {
		ActivityID int64 `json:"activity_id"`
	}
	if err := json.Unmarshal(uploadBody, &uploaded); err != nil {
		t.Fatalf("decode upload response: %v (body: %s)", err, uploadBody)
	}

	// The upload handler computes NP/IF/TSS synchronously (pure CPU work,
	// no external API) — must already be there by the time the response
	// came back, no polling needed.
	var tss *float64
	if err := pool.QueryRow(ctx,
		`SELECT training_stress_score FROM activities WHERE id = $1`, uploaded.ActivityID,
	).Scan(&tss); err != nil {
		t.Fatalf("query training_stress_score: %v", err)
	}
	if tss == nil || *tss <= 0 {
		t.Fatalf("training_stress_score = %v, want a positive value computed automatically at upload", tss)
	}

	loadResp := getJSON(t, client, server.URL+"/api/training-load")
	series, ok := loadResp["series"].([]any)
	if !ok || len(series) == 0 {
		t.Fatalf("training-load series = %v, want at least one day", loadResp["series"])
	}
	// series[0] is the ride's own day (2026-07-20, fixed in the fixture);
	// later days up to today are rest days with tss=0, not the ride itself.
	rideDay, ok := series[0].(map[string]any)
	if !ok {
		t.Fatalf("series entries have unexpected shape: %v", series[0])
	}
	rideDayTSS, ok := rideDay["tss"].(float64)
	if !ok || rideDayTSS <= 0 {
		t.Fatalf("ride day tss = %v, want it to reflect the uploaded ride's TSS", rideDay["tss"])
	}

	insights, ok := loadResp["insights"].([]any)
	if !ok || len(insights) == 0 {
		t.Fatalf("training-load insights = %v, want at least one tip", loadResp["insights"])
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

func postRaw(t *testing.T, client *http.Client, url, body string, wantStatus int) {
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

func patchJSON(t *testing.T, client *http.Client, url, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("build PATCH request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	defer resp.Body.Close()
	body2, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH %s = %d, want 200, body: %s", url, resp.StatusCode, body2)
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
