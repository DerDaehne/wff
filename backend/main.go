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
	if len(os.Args) > 1 && os.Args[1] == "user" {
		runUserCLI(os.Args[2:])
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

	// Heart-rate-based training load is computed from the pulse trace rather
	// than the ride's average (#622). Rides scored under the old formula are
	// brought onto the new one here — a history half on each would put a step
	// in the fitness curve that no ride caused. Off the request path, and a
	// failure only costs accuracy on old rides, so it logs rather than exits.
	go func() {
		if err := analyze.RecomputeHeartRateLoad(ctx, pool); err != nil {
			log.Printf("recompute heart-rate load: %v", err)
		}
	}()

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

// runUserCLI implements `wff user list` and `wff user delete <username>`.
func runUserCLI(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: wff user list | wff user delete <username>")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, requireEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	switch args[0] {
	case "list":
		users, err := auth.ListUsers(ctx, pool)
		if err != nil {
			log.Fatalf("list users: %v", err)
		}
		if len(users) == 0 {
			fmt.Println("no users")
			return
		}
		for _, u := range users {
			fmt.Printf("%d\t%s\t%s\tcreated %s\t%d credential(s)\n",
				u.ID, u.Username, u.DisplayName, u.CreatedAt.Format(time.DateOnly), u.CredentialCount)
		}
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: wff user delete <username>")
			os.Exit(1)
		}
		username := args[1]
		if err := auth.DeleteUser(ctx, pool, username); err != nil {
			log.Fatalf("delete user %s: %v", username, err)
		}
		fmt.Printf("deleted user %s (and their credentials/sessions/activities)\n", username)
	default:
		fmt.Fprintln(os.Stderr, "usage: wff user list | wff user delete <username>")
		os.Exit(1)
	}
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
