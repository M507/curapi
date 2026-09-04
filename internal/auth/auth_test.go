package auth

import (
	"net/http"
	"testing"
)

func TestValidToken(t *testing.T) {
	a := New(true, []string{"alpha", "beta"})
	if !a.Valid("alpha") || !a.Valid("beta") {
		t.Fatal("expected configured tokens to match")
	}
	if a.Valid("gamma") || a.Valid("") || a.Valid("not-needed") {
		t.Fatal("unexpected match")
	}
}

func TestDisabledWhenNotRequired(t *testing.T) {
	a := New(false, []string{"alpha"})
	if a.Enabled() {
		t.Fatal("auth should be off when not required")
	}
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	if err := a.AuthorizeRequest(req); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizeRequest(t *testing.T) {
	a := New(true, []string{"secret-token"})

	missing, _ := http.NewRequest(http.MethodGet, "/", nil)
	if err := a.AuthorizeRequest(missing); err != ErrMissing {
		t.Fatalf("got %v", err)
	}

	bad, _ := http.NewRequest(http.MethodGet, "/", nil)
	bad.Header.Set("Authorization", "Bearer nope")
	if err := a.AuthorizeRequest(bad); err != ErrInvalid {
		t.Fatalf("got %v", err)
	}

	placeholder, _ := http.NewRequest(http.MethodGet, "/", nil)
	placeholder.Header.Set("Authorization", "Bearer not-needed")
	if err := a.AuthorizeRequest(placeholder); err != ErrInvalid {
		t.Fatalf("placeholder should not bypass auth, got %v", err)
	}

	ok, _ := http.NewRequest(http.MethodGet, "/", nil)
	ok.Header.Set("Authorization", "Bearer secret-token")
	if err := a.AuthorizeRequest(ok); err != nil {
		t.Fatal(err)
	}
}

func TestBearerToken(t *testing.T) {
	tok, ok := BearerToken("Bearer abc")
	if !ok || tok != "abc" {
		t.Fatalf("%q %v", tok, ok)
	}
	if _, ok := BearerToken("Basic abc"); ok {
		t.Fatal("basic should fail")
	}
	if _, ok := BearerToken("Bearer   "); ok {
		t.Fatal("empty bearer should fail")
	}
}

func TestEnabledRequiresTokens(t *testing.T) {
	a := New(true, []string{"  ", ""})
	if a.Enabled() {
		t.Fatal("empty tokens should disable")
	}
}
