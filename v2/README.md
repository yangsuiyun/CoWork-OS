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

## Milestones (see spec §18)

- **M0** Contracts frozen: OpenAPI + event/capability schemas + read-model DDL
  + permission rule matrix + codegen + CI boundary guard (`depguard`).
- **M1** Walking skeleton: Task aggregate end-to-end
  (command → event_log + outbox → projector → read model → WS push → thin web),
  auth/tenant/RLS.
- **M2** Capability authorization + Pre/PostToolUse hooks.
- **M3** Local Agent Runner or cloud sandbox.
- **M4** Orchestration + external agents + self-learning.
- **M5** Desktop convergence.
