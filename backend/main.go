package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/DerDaehne/wff/internal/activities"
	"github.com/DerDaehne/wff/internal/analyze"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
	"github.com/DerDaehne/wff/internal/enrich"
	"github.com/DerDaehne/wff/internal/openmeteo"
	"github.com/DerDaehne/wff/internal/profile"
	"github.com/DerDaehne/wff/internal/webui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "invite" {
		runInviteCLI(os.Args[2:])
		return
	}
	runServer()
}

func runServer() {
	ctx := context.Background()

	pool, err := db.Open(ctx, requireEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(pool, migrationsFS); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	wa, err := auth.NewWebAuthn()
	if err != nil {
		log.Fatalf("webauthn: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	weather := openmeteo.New(os.Getenv("OPENMETEO_BASE_URL"))

	auth.NewHandlers(pool, wa).Register(mux)
	activities.NewHandlers(pool, cmp.Or(os.Getenv("UPLOAD_DIR"), "./data/uploads"), weather).Register(mux)
	profile.NewHandlers(pool).Register(mux)
	analyze.NewHandlers(pool).Register(mux)

	// Catch-all: the embedded frontend build. Registered last for
	// readability only — Go 1.22+'s ServeMux prioritizes the more specific
	// patterns above regardless of registration order.
	mux.Handle("/", webui.Handler())

	go enrich.RunPoller(ctx, pool, weather, enrichmentPollInterval())

	addr := ":" + cmp.Or(os.Getenv("PORT"), "8080")
	log.Printf("wff backend listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// runInviteCLI implements `wff invite create <username> <display-name>` —
// the only way new users get created (no self-signup, no admin UI yet).
func runInviteCLI(args []string) {
	if len(args) < 3 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "usage: wff invite create <username> <display-name>")
		os.Exit(1)
	}
	username, displayName := args[1], args[2]

	ctx := context.Background()
	pool, err := db.Open(ctx, requireEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	token, err := auth.CreateInvite(ctx, pool, username, displayName)
	if err != nil {
		log.Fatalf("create invite: %v", err)
	}

	base := os.Getenv("PUBLIC_BASE_URL")
	if base == "" {
		fmt.Printf("Invite token for %s (valid %s): %s\n", username, auth.InviteTTL, token)
		// /invite/{token} is the frontend page (calls /auth/invite/{token} itself)
		fmt.Println("Redeem at: <PUBLIC_BASE_URL>/invite/" + token)
		return
	}
	fmt.Printf("Invite for %s (valid %s): %s/invite/%s\n", username, auth.InviteTTL, base, token)
}

// enrichmentPollInterval reads ENRICHMENT_POLL_INTERVAL (Go duration
// string, e.g. "1h"). ERA5 has ~5 day ingest lag, so polling more often
// than hourly wouldn't find anything new — 1h is a sane default, not a
// performance-tuned one.
func enrichmentPollInterval() time.Duration {
	const defaultInterval = time.Hour
	v := os.Getenv("ENRICHMENT_POLL_INTERVAL")
	if v == "" {
		return defaultInterval
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("invalid ENRICHMENT_POLL_INTERVAL %q, using default %s: %v", v, defaultInterval, err)
		return defaultInterval
	}
	return d
}

func requireEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		log.Fatalf("missing required env var %s", name)
	}
	return v
}
