package events

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestStore connects to the integration Postgres. Skips when
// COWORK_DATABASE_URL is unset so unit-only runs stay green.
func newTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dsn := os.Getenv("COWORK_DATABASE_URL")
	if dsn == "" {
		t.Skip("COWORK_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewStore(pool), ctx
}

func ev(typ string) ToAppend {
	return ToAppend{Type: typ, SchemaVer: 1, Payload: []byte(`{"x":1}`), Actor: "tester"}
}

func TestAppendAndLoad(t *testing.T) {
	s, ctx := newTestStore(t)
	stream := fmt.Sprintf("task:%d", time.Now().UnixNano())
	tenant := "t-append"

	var committed []Committed
	err := s.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		var e error
		committed, e = s.Append(ctx, tx, tenant, stream, 0, []ToAppend{ev("TaskCreated"), ev("TaskPlanned")})
		return e
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if len(committed) != 2 || committed[0].StreamSeq != 1 || committed[1].StreamSeq != 2 {
		t.Fatalf("unexpected stream_seq: %+v", committed)
	}

	err = s.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		loaded, e := s.LoadStream(ctx, tx, stream)
		if e != nil {
			return e
		}
		if len(loaded) != 2 || loaded[0].Type != "TaskCreated" || loaded[1].Type != "TaskPlanned" {
			return fmt.Errorf("unexpected load: %+v", loaded)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestConcurrencyConflict(t *testing.T) {
	s, ctx := newTestStore(t)
	stream := fmt.Sprintf("task:%d", time.Now().UnixNano())
	tenant := "t-conflict"

	if err := s.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		_, e := s.Append(ctx, tx, tenant, stream, 0, []ToAppend{ev("TaskCreated")})
		return e
	}); err != nil {
		t.Fatalf("first append: %v", err)
	}

	// Reuse expectedSeq=0 (stale): must collide on (stream_id, stream_seq=1).
	err := s.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		_, e := s.Append(ctx, tx, tenant, stream, 0, []ToAppend{ev("TaskPlanned")})
		return e
	})
	if err == nil {
		t.Fatal("expected concurrency conflict, got nil")
	}
}

func TestGlobalSeqMonotonic(t *testing.T) {
	s, ctx := newTestStore(t)
	tenant := "t-mono"
	streamA := fmt.Sprintf("task:a-%d", time.Now().UnixNano())
	streamB := fmt.Sprintf("task:b-%d", time.Now().UnixNano())

	var a, b []Committed
	if err := s.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		var e error
		a, e = s.Append(ctx, tx, tenant, streamA, 0, []ToAppend{ev("TaskCreated")})
		return e
	}); err != nil {
		t.Fatalf("append A: %v", err)
	}
	if err := s.WithTenantTx(ctx, tenant, func(tx pgx.Tx) error {
		var e error
		b, e = s.Append(ctx, tx, tenant, streamB, 0, []ToAppend{ev("TaskCreated")})
		return e
	}); err != nil {
		t.Fatalf("append B: %v", err)
	}
	if b[0].GlobalSeq <= a[0].GlobalSeq {
		t.Fatalf("global_seq not monotonic across streams: A=%d B=%d", a[0].GlobalSeq, b[0].GlobalSeq)
	}
}
