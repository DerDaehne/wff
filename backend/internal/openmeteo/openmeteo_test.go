package openmeteo_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DerDaehne/wff/internal/openmeteo"
)

func hour(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02T15:04", s)
	if err != nil {
		t.Fatalf("parse hour %q: %v", s, err)
	}
	return ts.UTC()
}

func TestFetchMultiLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"hourly":{"time":["2026-07-20T07:00","2026-07-20T08:00"],"temperature_2m":[18.5,19.0],"wind_speed_10m":[3.2,3.5],"wind_direction_10m":[270,275],"precipitation":[0.0,0.0]}},
			{"hourly":{"time":["2026-07-20T09:00","2026-07-20T10:00"],"temperature_2m":[20.1,21.0],"wind_speed_10m":[4.0,4.2],"wind_direction_10m":[280,285],"precipitation":[0.1,0.0]}}
		]`))
	}))
	defer server.Close()

	client := openmeteo.New(server.URL)
	points := []openmeteo.Point{
		{Lat: 48.1, Lon: 11.5, HourBucket: hour(t, "2026-07-20T08:00")},
		{Lat: 48.2, Lon: 11.6, HourBucket: hour(t, "2026-07-20T09:00")},
	}
	results, err := client.Fetch(context.Background(), points)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	if got := results[0].TemperatureCelsius; got == nil || *got != 19.0 {
		t.Errorf("results[0].TemperatureCelsius = %v, want 19.0", got)
	}
	if got := results[0].WindDirectionDeg; got == nil || *got != 275 {
		t.Errorf("results[0].WindDirectionDeg = %v, want 275", got)
	}
	if got := results[1].TemperatureCelsius; got == nil || *got != 20.1 {
		t.Errorf("results[1].TemperatureCelsius = %v, want 20.1 (second location's first hour)", got)
	}
}

func TestFetchSingleLocationObjectShape(t *testing.T) {
	// Open-Meteo returns a bare JSON object (not an array) when only one
	// location is requested — the client must handle both response shapes.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hourly":{"time":["2026-07-20T08:00"],"temperature_2m":[19.0],"wind_speed_10m":[3.5],"wind_direction_10m":[275],"precipitation":[0.0]}}`))
	}))
	defer server.Close()

	client := openmeteo.New(server.URL)
	points := []openmeteo.Point{{Lat: 48.1, Lon: 11.5, HourBucket: hour(t, "2026-07-20T08:00")}}
	results, err := client.Fetch(context.Background(), points)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(results) != 1 || results[0].TemperatureCelsius == nil || *results[0].TemperatureCelsius != 19.0 {
		t.Fatalf("results = %+v, want single result with TemperatureCelsius=19.0", results)
	}
}

func TestFetchHourNotYetAvailable(t *testing.T) {
	// ERA5 has ~5 day ingest lag: null entries are the normal case for a
	// recently recorded ride, not an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hourly":{"time":["2026-07-20T08:00"],"temperature_2m":[null],"wind_speed_10m":[null],"wind_direction_10m":[null],"precipitation":[null]}}`))
	}))
	defer server.Close()

	client := openmeteo.New(server.URL)
	points := []openmeteo.Point{{Lat: 48.1, Lon: 11.5, HourBucket: hour(t, "2026-07-20T08:00")}}
	results, err := client.Fetch(context.Background(), points)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	r := results[0]
	if r.TemperatureCelsius != nil || r.WindSpeedMps != nil || r.WindDirectionDeg != nil || r.PrecipitationMm != nil {
		t.Fatalf("expected all-nil PointResult for not-yet-available hour, got %+v", r)
	}
}

func TestFetchServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := openmeteo.New(server.URL)
	points := []openmeteo.Point{{Lat: 48.1, Lon: 11.5, HourBucket: hour(t, "2026-07-20T08:00")}}
	_, err := client.Fetch(context.Background(), points)
	if err == nil {
		t.Fatal("Fetch with 500 response = nil error, want an error")
	}
}

func TestFetchNetworkFailure(t *testing.T) {
	// Point at a server that's immediately closed, so the connection is
	// refused — simulates a real network-level failure, not an HTTP error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := server.URL
	server.Close()

	client := openmeteo.New(unreachableURL)
	points := []openmeteo.Point{{Lat: 48.1, Lon: 11.5, HourBucket: hour(t, "2026-07-20T08:00")}}
	_, err := client.Fetch(context.Background(), points)
	if err == nil {
		t.Fatal("Fetch against unreachable server = nil error, want an error")
	}
}
