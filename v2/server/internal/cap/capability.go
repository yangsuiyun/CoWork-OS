// Package cap is the user-space capability + permission layer (spec 11.1/11.2):
// it mints/verifies capability tokens, enforces revocation at use-time, and
// runs the deterministic permission decision. It sits above the kernel and
// below the HTTP adapter (DAG: kernel <- cap <- adapter); it never imports an
// adapter.
package cap

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/coworkos/cowork-os/v2/server/pkg/contracts"
)

// Capability is the canonical token claim set (single source of truth: the
// JSON Schema, codegen'd into contracts).
type Capability = contracts.CapabilitySchemaJson

var (
	ErrInvalidToken = errors.New("cap: invalid or tampered token")
	ErrExpired      = errors.New("cap: capability expired")
	ErrRevoked      = errors.New("cap: capability revoked")
	// ErrNonRevocable rejects minting a revocable=false capability: those denote
	// the uncoverable hard-deny set and must never be issued (spec 11.2).
	ErrNonRevocable = errors.New("cap: non-revocable capability cannot be minted")
)

// Issuer mints and parses server-signed capability tokens (HMAC envelope).
type Issuer struct {
	secret []byte
}

// NewIssuer constructs an Issuer with the signing secret.
func NewIssuer(secret string) *Issuer { return &Issuer{secret: []byte(secret)} }

// Mint serializes and signs a capability. A non-revocable capability is
// rejected (uncoverable set).
func (i *Issuer) Mint(c Capability) (string, error) {
	if !c.Revocable {
		return "", ErrNonRevocable
	}
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return enc(body) + "." + enc(i.sign(body)), nil
}

// Parse verifies the signature and returns the capability claims. It does NOT
// check expiry or revocation; use Verifier.Verify for the full use-time gate.
func (i *Issuer) Parse(token string) (Capability, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Capability{}, ErrInvalidToken
	}
	body, err := dec(parts[0])
	if err != nil {
		return Capability{}, ErrInvalidToken
	}
	sig, err := dec(parts[1])
	if err != nil || !hmac.Equal(sig, i.sign(body)) {
		return Capability{}, ErrInvalidToken
	}
	var c Capability
	if err := json.Unmarshal(body, &c); err != nil {
		return Capability{}, ErrInvalidToken
	}
	return c, nil
}

func (i *Issuer) sign(body []byte) []byte {
	m := hmac.New(sha256.New, i.secret)
	m.Write(body)
	return m.Sum(nil)
}

// Expired reports whether the capability's expiry constraint has passed.
func Expired(c Capability, now time.Time) bool {
	return c.Constraints != nil && c.Constraints.ExpiresAt != nil && now.After(*c.Constraints.ExpiresAt)
}

func enc(b []byte) string          { return base64.RawURLEncoding.EncodeToString(b) }
func dec(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }
