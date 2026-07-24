package enrich

import "testing"

func almostEqual(a, b, tolerance float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}

func TestBearingDegCardinalDirections(t *testing.T) {
	cases := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		wantDeg                float64
	}{
		{"due east", 0, 0, 0, 1, 90},
		{"due north", 0, 0, 1, 0, 0},
		{"due west", 0, 1, 0, 0, 270},
		{"due south", 1, 0, 0, 0, 180},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := bearingDeg(c.lat1, c.lon1, c.lat2, c.lon2)
			if !almostEqual(got, c.wantDeg, 0.5) {
				t.Errorf("bearingDeg(%v,%v -> %v,%v) = %v, want ~%v", c.lat1, c.lon1, c.lat2, c.lon2, got, c.wantDeg)
			}
		})
	}
}

func TestHeadwindComponent(t *testing.T) {
	bearing := 90.0 // heading due east
	speed := 5.0

	headwindDir := 90.0 // wind blowing FROM the east -> into the rider's face
	if got := headwindComponent(&bearing, &headwindDir, &speed); got == nil || !almostEqual(*got, 5.0, 1e-9) {
		t.Errorf("headwind case: got %v, want +5.0 (headwind)", got)
	}

	tailwindDir := 270.0 // wind blowing FROM the west -> pushing the rider east
	if got := headwindComponent(&bearing, &tailwindDir, &speed); got == nil || !almostEqual(*got, -5.0, 1e-9) {
		t.Errorf("tailwind case: got %v, want -5.0 (tailwind)", got)
	}

	crosswindDir := 0.0 // wind from due north, rider heading east -> pure crosswind
	if got := headwindComponent(&bearing, &crosswindDir, &speed); got == nil || !almostEqual(*got, 0.0, 1e-9) {
		t.Errorf("crosswind case: got %v, want ~0.0", got)
	}

	if got := headwindComponent(nil, &headwindDir, &speed); got != nil {
		t.Errorf("nil bearing: got %v, want nil", got)
	}
}
