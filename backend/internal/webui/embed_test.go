package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The SPA fallback must not swallow missing build assets: a 200 text/html for
// a missing .js turns into a confusing MIME-type error in the browser instead
// of an obvious 404 (#580).
func TestHandlerFallback(t *testing.T) {
	h := Handler()

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/rides/1", http.StatusOK},                      // client-side route → index.html
		{"/", http.StatusOK},                             //
		{"/_app/immutable/gone.js", http.StatusNotFound}, // missing asset → 404
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("GET %s: got %d, want %d", tc.path, rec.Code, tc.want)
		}
	}
}
