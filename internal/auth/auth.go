// Package auth validates Authorization bearer tokens against env.json.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

const (
	headerAuthorization = "Authorization"
	prefixBearer        = "Bearer "
)

// Authenticator checks client bearer tokens using constant-time compares.
type Authenticator struct {
	Required bool
	Tokens   []string
}

func New(required bool, tokens []string) Authenticator {
	cleaned := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t != "" {
			cleaned = append(cleaned, t)
		}
	}
	return Authenticator{Required: required, Tokens: cleaned}
}

func (a Authenticator) Enabled() bool {
	return a.Required && len(a.Tokens) > 0
}

// AuthorizeRequest returns nil when the request is allowed.
func (a Authenticator) AuthorizeRequest(r *http.Request) error {
	if !a.Enabled() {
		return nil
	}
	token, ok := BearerToken(r.Header.Get(headerAuthorization))
	if !ok {
		return ErrMissing
	}
	if !a.Valid(token) {
		return ErrInvalid
	}
	return nil
}

func (a Authenticator) Valid(token string) bool {
	if token == "" || len(a.Tokens) == 0 {
		return false
	}
	provided := sha256.Sum256([]byte(token))
	ok := false
	for _, allowed := range a.Tokens {
		want := sha256.Sum256([]byte(allowed))
		if subtle.ConstantTimeCompare(provided[:], want[:]) == 1 {
			ok = true
		}
	}
	return ok
}

func BearerToken(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefixBearer):])
	if token == "" {
		return "", false
	}
	return token, true
}

type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrMissing Error = "missing authorization bearer token"
	ErrInvalid Error = "invalid authorization token"
)
