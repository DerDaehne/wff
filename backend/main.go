package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/DerDaehne/wff/internal/activities"
	"github.com/DerDaehne/wff/internal/auth"
	"github.com/DerDaehne/wff/internal/db"
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

	wa, err := auth.NewWebAuthn()
	if err != nil {
		log.Fatalf("webauthn: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	auth.NewHandlers(pool, wa).Register(mux)
	activities.NewHandlers(pool, cmp.Or(os.Getenv("UPLOAD_DIR"), "./data/uploads")).Register(mux)

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
		fmt.Println("Redeem at: <PUBLIC_BASE_URL>/auth/invite/" + token)
		return
	}
	fmt.Printf("Invite for %s (valid %s): %s/auth/invite/%s\n", username, auth.InviteTTL, base, token)
}

func requireEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		log.Fatalf("missing required env var %s", name)
	}
	return v
}
