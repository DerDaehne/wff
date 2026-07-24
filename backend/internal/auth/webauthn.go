package auth

import (
	"cmp"
	"os"

	"github.com/go-webauthn/webauthn/webauthn"
)

func NewWebAuthn() (*webauthn.WebAuthn, error) {
	rpID := cmp.Or(os.Getenv("WEBAUTHN_RPID"), "localhost")
	origin := cmp.Or(os.Getenv("WEBAUTHN_ORIGIN"), "http://localhost:8080")
	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: "WFF",
		RPOrigins:     []string{origin},
	})
}

// webauthnUser adapts a WFF user (existing or pending-registration) to the
// webauthn.User interface. WebAuthnID does not need to match a real users.id
// during registration since the DB row is only created in FinishRegistration.
type webauthnUser struct {
	id          []byte
	username    string
	displayName string
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte                         { return u.id }
func (u *webauthnUser) WebAuthnName() string                       { return u.username }
func (u *webauthnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }
func (u *webauthnUser) WebAuthnIcon() string                       { return "" }
