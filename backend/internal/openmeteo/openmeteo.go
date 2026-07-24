// Package openmeteo is a small client for Open-Meteo's Historical Weather
// API (ERA5 archive). Non-commercial, keyless usage — see arch-wff-enrichment
// for the usage-limit rationale.
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://archive-api.open-meteo.com/v1/archive"

type Point struct {
	Lat, Lon   float64
	HourBucket time.Time // must fall exactly on the hour; compared in UTC
}

// PointResult mirrors fitparse's nil-means-missing convention. All four
// fields nil together means the hour isn't in the ERA5 archive yet — the
// normal case for recently recorded rides (~5 day ingest lag), not an error.
type PointResult struct {
	Point
	TemperatureCelsius *float64
	WindSpeedMps       *float64
	WindDirectionDeg   *float64
	PrecipitationMm    *float64
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
}

// Fetch retrieves weather for every point in a single batched request
// (Open-Meteo's multi-location comma-separated coordinates). Returns one
// PointResult per input point, in the same order.
func (c *Client) Fetch(ctx context.Context, points []Point) ([]PointResult, error) {
	if len(points) == 0 {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.buildURL(points), nil)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openmeteo: unexpected status %d: %s", resp.StatusCode, body)
	}

	locations, err := parseLocations(body)
	if err != nil {
		return nil, fmt.Errorf("openmeteo: parse response: %w", err)
	}
	if len(locations) != len(points) {
		return nil, fmt.Errorf("openmeteo: got %d locations in response, want %d", len(locations), len(points))
	}

	results := make([]PointResult, len(points))
	for i, p := range points {
		results[i] = PointResult{Point: p}
		idx := locations[i].Hourly.indexOf(p.HourBucket)
		if idx < 0 {
			continue // hour not covered by the response at all
		}
		h := locations[i].Hourly
		results[i].TemperatureCelsius = at(h.Temperature2m, idx)
		results[i].WindSpeedMps = at(h.WindSpeed10m, idx)
		results[i].WindDirectionDeg = at(h.WindDirection10m, idx)
		results[i].PrecipitationMm = at(h.Precipitation, idx)
	}
	return results, nil
}

func (c *Client) buildURL(points []Point) string {
	lats := make([]string, len(points))
	lons := make([]string, len(points))
	minDate, maxDate := points[0].HourBucket, points[0].HourBucket
	for i, p := range points {
		lats[i] = strconv.FormatFloat(p.Lat, 'f', 6, 64)
		lons[i] = strconv.FormatFloat(p.Lon, 'f', 6, 64)
		if p.HourBucket.Before(minDate) {
			minDate = p.HourBucket
		}
		if p.HourBucket.After(maxDate) {
			maxDate = p.HourBucket
		}
	}

	q := url.Values{}
	q.Set("latitude", strings.Join(lats, ","))
	q.Set("longitude", strings.Join(lons, ","))
	q.Set("start_date", minDate.UTC().Format("2006-01-02"))
	q.Set("end_date", maxDate.UTC().Format("2006-01-02"))
	q.Set("hourly", "temperature_2m,wind_speed_10m,wind_direction_10m,precipitation")
	q.Set("wind_speed_unit", "ms")
	q.Set("timezone", "UTC")

	return c.baseURL + "?" + q.Encode()
}

type hourlyData struct {
	Time             []string   `json:"time"`
	Temperature2m    []*float64 `json:"temperature_2m"`
	WindSpeed10m     []*float64 `json:"wind_speed_10m"`
	WindDirection10m []*float64 `json:"wind_direction_10m"`
	Precipitation    []*float64 `json:"precipitation"`
}

func (h hourlyData) indexOf(bucket time.Time) int {
	target := bucket.UTC().Format("2006-01-02T15:04")
	for i, t := range h.Time {
		if t == target {
			return i
		}
	}
	return -1
}

type locationResponse struct {
	Hourly hourlyData `json:"hourly"`
}

// parseLocations handles Open-Meteo's response-shape quirk: a single
// location returns one JSON object, multiple locations return a JSON array.
func parseLocations(body []byte) ([]locationResponse, error) {
	var asArray []locationResponse
	if err := json.Unmarshal(body, &asArray); err == nil {
		return asArray, nil
	}
	var single locationResponse
	if err := json.Unmarshal(body, &single); err != nil {
		return nil, err
	}
	return []locationResponse{single}, nil
}

func at(vals []*float64, idx int) *float64 {
	if idx < 0 || idx >= len(vals) {
		return nil
	}
	return vals[idx]
}
