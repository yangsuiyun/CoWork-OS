# CoWork OS v2 (B/S · Go server · multi-client)

New implementation per `docs/cowork-os-complete-design-spec.html`.

This is a clean rewrite of the legacy TypeScript/Electron local-first
implementation (now under `../legacy/`). The legacy tree is kept read-only
as a feature reference; v2 does not depend on it.

## Layout

```
v2/
├─ contracts/            # Single source of truth (M0)
│   ├─ openapi.yaml      # Commands + queries HTTP contract
│   ├─ events/           # Event payload JSON Schemas (full catalog)
│   └─ capability.schema.json
├─ server/               # Go backend (own go.mod)
│   ├─ cmd/coworkd/      # main entrypoint
│   ├─ internal/kernel/  # stable kernel: task, events, permission, workspace, session
│   ├─ internal/cap/     # user-space capabilities: orchestration, skills, gateway, memory
│   ├─ internal/adapter/ # anti-corruption: mcp, acp, channel
│   ├─ pkg/contracts/    # codegen output (Go types)
│   └─ migrations/       # goose SQL migrations
├─ web/                  # thin React client (Vite)
└─ tools/codegen/        # OpenAPI + JSON Schema -> Go & TS
```

## Development

```bash
# Backend (needs Postgres + goose migrations applied; roles cowork_app/cowork_projector):
export COWORK_DATABASE_URL='postgres://cowork_app:cowork@localhost:5432/coworkos?sslmode=disable'
export COWORK_PROJECTOR_DATABASE_URL='postgres://cowork_projector:cowork@localhost:5432/coworkos?sslmode=disable'
cd server && go run ./cmd/coworkd            # serves :8080

# Frontend thin client (proxies /v1 to :8080):
cd web && npm install && npm run dev          # serves :5173

# Regenerate all contract types (Go + TS) after editing contracts/:
make codegen
```

The web client needs a JWT with `tid` (tenant) and `sub` (actor) claims signed
with `COWORK_JWT_SECRET` (dev default `dev-insecure-secret`); paste it into the
token field. External API clients use `/v1/sessions` instead.

## Milestones (see spec §18)

Detailed progress: [`PROGRESS.md`](PROGRESS.md).

- **M0** ✅ Contracts frozen: OpenAPI + event/capability schemas + read-model DDL
  + permission rule matrix + codegen + CI boundary guard (`depguard`).
- **M1** ✅ Walking skeleton: Task aggregate end-to-end
  (command → event_log + outbox → projector → read model → WS push → thin web),
  auth/tenant/RLS.
- **M2** ✅ Capability authorization + Pre/PostToolUse hooks + external
  `/v1/sessions` API + React thin client.
- **M3** ⏳ Local Agent Runner or cloud sandbox.
- **M4** 🚧 Orchestration + external agents + self-learning (Workspace aggregate done).
- **M5** ⏳ Desktop convergence.
