package cap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newRevStore(t *testing.T) (*RevocationStore, context.Context) {
	t.Helper()
	dsn := os.Getenv("COWORK_DATABASE_URL")
	if dsn == "" {
		t.Skip("COWORK_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return NewRevocationStore(pool), ctx
}

func TestRevokeVersionSemantics(t *testing.T) {
	rev, ctx := newRevStore(t)
	tenant := fmt.Sprintf("rev-%d", os.Getpid())
	tok := fmt.Sprintf("tok-%d", os.Getpid())

	if r, err := rev.IsRevoked(ctx, tenant, tok, 1); err != nil || r {
		t.Fatalf("fresh token must not be revoked: r=%v err=%v", r, err)
	}

	if err := rev.Revoke(ctx, tenant, tok, 1, "leaked"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	// version <= revoked_version is revoked; a higher re-issued version is not.
	if r, _ := rev.IsRevoked(ctx, tenant, tok, 1); !r {
		t.Fatal("v1 must be revoked")
	}
	if r, _ := rev.IsRevoked(ctx, tenant, tok, 2); r {
		t.Fatal("v2 must NOT be revoked by a v1 revocation")
	}
}

func TestVerifyRevokedToken(t *testing.T) {
	rev, ctx := newRevStore(t)
	tenant := fmt.Sprintf("vrev-%d", os.Getpid())
	iss := NewIssuer("secret")
	v := NewVerifier(iss, rev)

	c := sampleCap()
	c.TokenId = fmt.Sprintf("vt-%d", os.Getpid())
	tok, _ := iss.Mint(c)

	if _, err := v.Verify(ctx, tenant, tok); err != nil {
		t.Fatalf("valid token should verify: %v", err)
	}
	if err := rev.Revoke(ctx, tenant, c.TokenId, c.Version, "test"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := v.Verify(ctx, tenant, tok); !errors.Is(err, ErrRevoked) {
		t.Fatalf("want ErrRevoked, got %v", err)
	}
}
