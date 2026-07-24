package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

const ceremonyTTL = 5 * time.Minute

// registrationCeremony snapshots everything FinishRegistration needs to
// reconstruct the exact same webauthnUser passed to BeginRegistration.
type registrationCeremony struct {
	session     webauthn.SessionData
	inviteID    int64
	webAuthnID  []byte
	username    string
	displayName string
}

// loginCeremony snapshots the user + credentials looked up at BeginLogin,
// so FinishLogin can reconstruct the same webauthnUser without a second query.
type loginCeremony struct {
	session     webauthn.SessionData
	userID      int64
	webAuthnID  []byte
	username    string
	displayName string
	credentials []webauthn.Credential
}

type ceremonyStore[T any] struct {
	mu      sync.Mutex
	entries map[string]ceremonyEntry[T]
}

type ceremonyEntry[T any] struct {
	value     T
	expiresAt time.Time
}

func newCeremonyStore[T any]() *ceremonyStore[T] {
	return &ceremonyStore[T]{entries: make(map[string]ceremonyEntry[T])}
}

func (s *ceremonyStore[T]) put(value T) string {
	id := randomToken(18)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapLocked()
	s.entries[id] = ceremonyEntry[T]{value: value, expiresAt: time.Now().Add(ceremonyTTL)}
	return id
}

func (s *ceremonyStore[T]) take(id string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	delete(s.entries, id)
	if !ok || time.Now().After(e.expiresAt) {
		var zero T
		return zero, false
	}
	return e.value, true
}

// reapLocked drops expired entries. Called opportunistically on put() since
// ceremonies are short-lived and volume is tiny (few users) — no need for a
// background goroutine.
func (s *ceremonyStore[T]) reapLocked() {
	now := time.Now()
	for id, e := range s.entries {
		if now.After(e.expiresAt) {
			delete(s.entries, id)
		}
	}
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func randomToken(n int) string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(n))
}
