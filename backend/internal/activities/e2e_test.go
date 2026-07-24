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
	mux := http.NewServeMux()
	auth.NewHandlers(pool, wa).Register(mux)
	activities.NewHandlers(pool, uploadDir).Register(mux)
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
