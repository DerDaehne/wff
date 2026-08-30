package bikes_test

// End-to-end test against a real Postgres instance and a real HTTP server,
// reusing the passkey-login pattern from #551. Skipped if DATABASE_URL is
// unset — see backend/README.md for the scratch-cluster invocation.

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
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/bikes"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/fitparse/fitfixture"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/descope/virtualwebauthn"
)

type bike struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	Active              bool       `json:"active"`
	RetiredAt           *time.Time `json:"retired_at"`
	DistanceKm          float64    `json:"distance_km"`
	ChainIntervalKm     float64    `json:"chain_interval_km"`
	ChainDueKm          float64    `json:"chain_due_km"`
	RideCount           int        `json:"ride_count"`
	MovingSeconds       int64      `json:"moving_seconds"`
	ElevationGainMeters float64    `json:"elevation_gain_meters"`
	AvgSpeedKmh         float64    `json:"avg_speed_kmh"`
}

func TestBikesLifecycle(t *testing.T) {
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
	bikes.NewHandlers(pool).Register(mux)
	server.Config.Handler = mux
	server.Start()
	defer server.Close()

	rp := virtualwebauthn.RelyingParty{Name: "WFF", ID: "localhost", Origin: server.URL}
	stamp := time.Now().UnixNano()
	username := fmt.Sprintf("bikes-test-%d", stamp)

	token, err := auth.CreateInvite(ctx, pool, username, "Bikes Test")
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

	// A rider's first bike becomes active automatically.
	gravel := createBike(t, client, server.URL, "Gravelbike")
	if !gravel.Active {
		t.Fatal("first bike created was not made active automatically")
	}

	// A second bike must NOT steal active status from the first.
	road := createBike(t, client, server.URL, "Rennrad")
	if road.Active {
		t.Fatal("second bike was made active automatically — should have left the first one alone")
	}

	// Upload a ride: it must attach to the currently active bike (Gravelbike).
	created := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	validFIT := fitfixture.ValidActivity(313131, created, 10)
	if _, status := doUploadMultipart(t, client, server.URL, validFIT); status != http.StatusCreated {
		t.Fatalf("upload: status = %d", status)
	}

	list := listBikes(t, client, server.URL)
	gravelAfter := findBike(t, list, gravel.ID)
	if gravelAfter.DistanceKm <= 0 {
		t.Errorf("Gravelbike distance_km = %v, want > 0 after a ride was uploaded to it", gravelAfter.DistanceKm)
	}
	roadAfter := findBike(t, list, road.ID)
	if roadAfter.DistanceKm != 0 {
		t.Errorf("Rennrad distance_km = %v, want 0 — the ride went to the active bike, not this one", roadAfter.DistanceKm)
	}

	// Comparison aggregates (#731): the untouched bike stays at zero across
	// the board, the ridden one reflects the upload with an internally
	// consistent avg speed — checked against the ridden bike's own numbers
	// rather than a hardcoded fixture value, since what matters here is the
	// unit conversion (distance/time), not the fixture's exact speed.
	if roadAfter.RideCount != 0 || roadAfter.MovingSeconds != 0 || roadAfter.AvgSpeedKmh != 0 {
		t.Errorf("untouched Rennrad aggregates = %+v, want all zero", roadAfter)
	}
	if gravelAfter.RideCount != 1 {
		t.Errorf("Gravelbike ride_count = %d, want 1", gravelAfter.RideCount)
	}
	if gravelAfter.MovingSeconds <= 0 {
		t.Errorf("Gravelbike moving_seconds = %d, want > 0", gravelAfter.MovingSeconds)
	}
	wantAvgSpeed := gravelAfter.DistanceKm / (float64(gravelAfter.MovingSeconds) / 3600.0)
	if got, want := gravelAfter.AvgSpeedKmh, wantAvgSpeed; got < want-0.01 || got > want+0.01 {
		t.Errorf("Gravelbike avg_speed_kmh = %v, want ≈ %v (distance_km/moving_hours)", got, want)
	}

	// Chain due shrinks below the default interval once there's real distance.
	if gravelAfter.ChainDueKm >= gravelAfter.ChainIntervalKm {
		t.Errorf("ChainDueKm = %v, want less than the interval (%v) once the bike has ridden distance",
			gravelAfter.ChainDueKm, gravelAfter.ChainIntervalKm)
	}

	// Marking the chain replaced resets the counter back up to the full interval.
	afterChainChange := patchAction(t, client, server.URL, gravel.ID, "chain-replaced")
	gravelReset := findBike(t, afterChainChange, gravel.ID)
	if got, want := gravelReset.ChainDueKm, gravelReset.ChainIntervalKm; got < want-0.01 || got > want+0.01 {
		t.Errorf("ChainDueKm after chain-replaced = %v, want ≈ %v (the full interval)", got, want)
	}

	// Switching the active bike moves it to the other one.
	afterActivate := patchAction(t, client, server.URL, road.ID, "activate")
	roadNowActive := findBike(t, afterActivate, road.ID)
	gravelNowInactive := findBike(t, afterActivate, gravel.ID)
	if !roadNowActive.Active || gravelNowInactive.Active {
		t.Fatalf("activate did not switch the active bike: road=%v gravel=%v", roadNowActive.Active, gravelNowInactive.Active)
	}

	// Retiring the active bike clears active status rather than leaving new
	// uploads pointed at a bike no longer in the rotation.
	afterRetire := patchBike(t, client, server.URL, road.ID, map[string]any{"retired": true})
	roadRetired := findBike(t, afterRetire, road.ID)
	if roadRetired.RetiredAt == nil {
		t.Fatal("retired bike has no retired_at set")
	}
	if roadRetired.Active {
		t.Fatal("a retired bike is still marked active")
	}

	// A retired bike must refuse to be reactivated.
	resp, err := client.Post(fmt.Sprintf("%s/api/bikes/%d/activate", server.URL, road.ID), "application/json", nil)
	if err != nil {
		t.Fatalf("POST activate: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("activating a retired bike: status = %d, want 400", resp.StatusCode)
	}

	// Another rider cannot touch this rider's bikes.
	otherToken, err := auth.CreateInvite(ctx, pool, fmt.Sprintf("bikes-other-%d", stamp), "Other Rider")
	if err != nil {
		t.Fatalf("CreateInvite (other): %v", err)
	}
	otherJar, _ := cookiejar.New(nil)
	otherClient := &http.Client{Jar: otherJar}
	otherAttestationBody := getBody(t, otherClient, server.URL+"/auth/invite/"+otherToken, http.StatusOK)
	otherOptions, err := virtualwebauthn.ParseAttestationOptions(otherAttestationBody)
	if err != nil {
		t.Fatalf("ParseAttestationOptions (other): %v", err)
	}
	otherAuthenticator := virtualwebauthn.NewAuthenticator()
	otherCredential := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	otherResponse := virtualwebauthn.CreateAttestationResponse(rp, otherAuthenticator, otherCredential, *otherOptions)
	postJSON(t, otherClient, server.URL+"/auth/invite/"+otherToken, otherResponse, http.StatusCreated)

	otherResp, err := otherClient.Post(fmt.Sprintf("%s/api/bikes/%d/activate", server.URL, gravel.ID), "application/json", nil)
	if err != nil {
		t.Fatalf("POST activate as other rider: %v", err)
	}
	otherResp.Body.Close()
	if otherResp.StatusCode != http.StatusNotFound {
		t.Fatalf("other rider activating this rider's bike: status = %d, want 404", otherResp.StatusCode)
	}
}

func createBike(t *testing.T, client *http.Client, baseURL, name string) bike {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := client.Post(baseURL+"/api/bikes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/bikes: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create bike: status = %d, body: %s", resp.StatusCode, respBody)
	}
	var list []bike
	if err := json.Unmarshal(respBody, &list); err != nil {
		t.Fatalf("decode create-bike response: %v (body: %s)", err, respBody)
	}
	for _, b := range list {
		if b.Name == name {
			return b
		}
	}
	t.Fatalf("created bike %q not found in response: %s", name, respBody)
	return bike{}
}

func listBikes(t *testing.T, client *http.Client, baseURL string) []bike {
	t.Helper()
	body := getBody(t, client, baseURL+"/api/bikes", http.StatusOK)
	var list []bike
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode bikes list: %v (body: %s)", err, body)
	}
	return list
}

func patchAction(t *testing.T, client *http.Client, baseURL string, bikeID int64, action string) []bike {
	t.Helper()
	resp, err := client.Post(fmt.Sprintf("%s/api/bikes/%d/%s", baseURL, bikeID, action), "application/json", nil)
	if err != nil {
		t.Fatalf("POST action %s: %v", action, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("action %s: status = %d, body: %s", action, resp.StatusCode, respBody)
	}
	var list []bike
	if err := json.Unmarshal(respBody, &list); err != nil {
		t.Fatalf("decode response for action %s: %v (body: %s)", action, err, respBody)
	}
	return list
}

func patchBike(t *testing.T, client *http.Client, baseURL string, bikeID int64, patch map[string]any) []bike {
	t.Helper()
	body, _ := json.Marshal(patch)
	req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/api/bikes/%d", baseURL, bikeID), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build PATCH request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PATCH bike: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH bike: status = %d, body: %s", resp.StatusCode, respBody)
	}
	var list []bike
	if err := json.Unmarshal(respBody, &list); err != nil {
		t.Fatalf("decode PATCH response: %v (body: %s)", err, respBody)
	}
	return list
}

func findBike(t *testing.T, list []bike, id int64) bike {
	t.Helper()
	for _, b := range list {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("bike %d not found in list", id)
	return bike{}
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
