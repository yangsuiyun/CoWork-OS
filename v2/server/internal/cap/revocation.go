package cap

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RevocationStore is the use-time revocation check backing capability tokens.
type RevocationStore struct {
	pool *pgxpool.Pool
}

// NewRevocationStore constructs a RevocationStore.
func NewRevocationStore(pool *pgxpool.Pool) *RevocationStore {
	return &RevocationStore{pool: pool}
}

func (s *RevocationStore) inTenant(ctx context.Context, tenant string, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Revoke marks all token versions <= version as revoked. Idempotent and
// monotonic: re-revoking keeps the highest revoked_version.
func (s *RevocationStore) Revoke(ctx context.Context, tenant, tokenID string, version int, reason string) error {
	return s.inTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO cap_revocation (tenant_id, token_id, revoked_version, reason)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant_id, token_id)
			DO UPDATE SET revoked_version = GREATEST(cap_revocation.revoked_version, EXCLUDED.revoked_version),
			              reason = EXCLUDED.reason, revoked_at = now()`,
			tenant, tokenID, version, nullableStr(reason))
		return err
	})
}

// IsRevoked reports whether a token (tokenID@version) is revoked.
func (s *RevocationStore) IsRevoked(ctx context.Context, tenant, tokenID string, version int) (bool, error) {
	var revoked bool
	err := s.inTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM cap_revocation WHERE token_id = $1 AND revoked_version >= $2)`,
			tokenID, version).Scan(&revoked)
	})
	return revoked, err
}

// Verifier is the full use-time capability gate: signature + expiry + revocation.
type Verifier struct {
	issuer *Issuer
	rev    *RevocationStore
}

// NewVerifier constructs a Verifier.
func NewVerifier(issuer *Issuer, rev *RevocationStore) *Verifier {
	return &Verifier{issuer: issuer, rev: rev}
}

// Verify parses the token and rejects it if expired or revoked (fail-fast with
// a typed error per class).
func (v *Verifier) Verify(ctx context.Context, tenant, token string) (Capability, error) {
	c, err := v.issuer.Parse(token)
	if err != nil {
		return Capability{}, err
	}
	if Expired(c, time.Now()) {
		return c, ErrExpired
	}
	revoked, err := v.rev.IsRevoked(ctx, tenant, c.TokenId, c.Version)
	if err != nil {
		return c, err
	}
	if revoked {
		return c, ErrRevoked
	}
	return c, nil
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
