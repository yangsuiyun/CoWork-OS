// Package realtime fans out committed-event notifications to WebSocket
// subscribers (spec realtime fan-out). It holds ONE Postgres LISTEN connection
// and pokes every subscriber on NOTIFY; each subscriber then re-queries its own
// tenant's events since its cursor (RLS-scoped), so no event data flows through
// the hub itself and tenant isolation is preserved.
package realtime

import (
	"context"
	"sync"

	"github.com/coworkos/cowork-os/v2/server/internal/kernel/app"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Hub broadcasts a poke to all subscribers whenever the event log advances.
type Hub struct {
	pool *pgxpool.Pool
	mu   sync.Mutex
	next int64
	subs map[int64]chan struct{}
}

// NewHub constructs a Hub over a pool used solely for the LISTEN connection.
func NewHub(pool *pgxpool.Pool) *Hub {
	return &Hub{pool: pool, subs: make(map[int64]chan struct{})}
}

// Run holds a dedicated connection on LISTEN cowork_events and pokes
// subscribers on each notification until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) error {
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+app.NotifyChannel); err != nil {
		return err
	}
	for {
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			return err // ctx cancelled or conn dropped: fail-fast to caller
		}
		h.pokeAll()
	}
}

// Subscribe registers a subscriber and returns its poke channel plus a release
// func. The channel receives a (coalesced) signal whenever new events may exist.
func (h *Hub) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	id := h.next
	h.next++
	h.subs[id] = ch
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) pokeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default: // already pending; coalesce
		}
	}
}
