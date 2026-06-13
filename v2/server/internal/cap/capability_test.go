package cap

import (
	"errors"
	"testing"
	"time"

	"github.com/coworkos/cowork-os/v2/server/pkg/contracts"
)

func sampleCap() Capability {
	return Capability{
		TokenId: "tok-1", Version: 1, Subject: "task:1", Resource: "fs.write",
		Scope:     contracts.CapabilitySchemaJsonScope{WorkspaceId: "w1", Paths: []string{"/tmp/**"}},
		Revocable: true,
	}
}

func TestMintParseRoundTrip(t *testing.T) {
	iss := NewIssuer("secret")
	tok, err := iss.Mint(sampleCap())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	got, err := iss.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.TokenId != "tok-1" || got.Resource != "fs.write" || got.Scope.WorkspaceId != "w1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	iss := NewIssuer("secret")
	tok, _ := iss.Mint(sampleCap())
	if _, err := iss.Parse(tok + "x"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
	// Wrong signing key must not verify.
	if _, err := NewIssuer("other").Parse(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken for wrong key, got %v", err)
	}
}

func TestMintRejectsNonRevocable(t *testing.T) {
	c := sampleCap()
	c.Revocable = false
	if _, err := NewIssuer("secret").Mint(c); !errors.Is(err, ErrNonRevocable) {
		t.Fatalf("want ErrNonRevocable, got %v", err)
	}
}

func TestExpired(t *testing.T) {
	c := sampleCap()
	past := time.Now().Add(-time.Hour)
	c.Constraints = &contracts.CapabilitySchemaJsonConstraints{ExpiresAt: &past}
	if !Expired(c, time.Now()) {
		t.Fatal("expected expired")
	}
	future := time.Now().Add(time.Hour)
	c.Constraints.ExpiresAt = &future
	if Expired(c, time.Now()) {
		t.Fatal("expected not expired")
	}
}
