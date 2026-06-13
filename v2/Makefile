# CoWork OS v2 build entry. Toolchain is provisioned in M1 (env-resolve).
.PHONY: codegen codegen-check lint test contracts-validate

# Regenerate types from contracts (single source of truth, ADR-02).
# oapi-codegen -> Go HTTP wire models; go-jsonschema -> Go event payload types;
# openapi-typescript -> TS client types (web/src/contracts).
codegen:
	cd tools/codegen && oapi-codegen --config oapi-codegen.yaml ../../contracts/openapi.yaml
	go-jsonschema -p contracts --output server/pkg/contracts/events_gen.go contracts/events/*.schema.json contracts/capability.schema.json
	cd web && npm run codegen

# CI drift guard: regenerate and diff; non-empty diff fails the build.
codegen-check: codegen
	git diff --exit-code -- server/pkg/contracts web/src/contracts

# Validate contracts are well-formed (OpenAPI 3.1 + JSON Schema 2020-12).
contracts-validate:
	npx --yes @redocly/cli lint contracts/openapi.yaml
	npx --yes ajv-cli compile -s "contracts/events/*.schema.json" -s contracts/capability.schema.json

# Boundary guard + vet (spec P1/P4).
lint:
	cd server && golangci-lint run ./...

test:
	cd server && go test ./...
