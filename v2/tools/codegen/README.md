# Codegen — contracts → Go & TS

The contracts under `../../contracts/` are the single source of truth (ADR-02).
Never hand-write the generated types; run codegen and commit the output.

## Pipeline

| Input | Tool | Output |
|-------|------|--------|
| `contracts/openapi.yaml` | `oapi-codegen` | `server/pkg/contracts/openapi_gen.go` |
| `contracts/events/*.schema.json`, `capability.schema.json` | `go-jsonschema` | `server/pkg/contracts/events_gen.go` |
| `contracts/openapi.yaml` | `openapi-typescript` | `web/src/contracts/openapi.d.ts` |
| `contracts/events/*.schema.json`, `capability.schema.json` | `json-schema-to-typescript` | `web/src/contracts/events.d.ts` |

## Run

```bash
make -C v2 codegen        # regenerate Go + TS types
make -C v2 codegen-check  # CI: fail if generated output drifts from contracts
```

Toolchain (installed in M1 via env-resolve):
`go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest`,
`go install github.com/atombender/go-jsonschema@latest`,
`npm i -D openapi-typescript json-schema-to-typescript`.
