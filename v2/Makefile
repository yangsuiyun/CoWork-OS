# CoWork OS v2 build entry. Toolchain is provisioned in M1 (env-resolve).
.PHONY: codegen codegen-check lint test contracts-validate

# Regenerate Go types from contracts (single source of truth, ADR-02).
# oapi-codegen -> HTTP wire models; go-jsonschema -> event payload types.
# TS generation (openapi-typescript -> web/src/contracts) is added together
# with the frontend project so no orphan artifact is committed before then.
codegen:
	cd tools/codegen && oapi-codegen --config oapi-codegen.yaml ../../contracts/openapi.yaml
	go-jsonschema -p contracts --output server/pkg/contracts/events_gen.go contracts/events/*.schema.json contracts/capability.schema.json

# CI drift guard: regenerate and diff; non-empty diff fails the build.
codegen-check: codegen
	git diff --exit-code -- server/pkg/contracts

# Validate contracts are well-formed (OpenAPI 3.1 + JSON Schema 2020-12).
contracts-validate:
	npx --yes @redocly/cli lint contracts/openapi.yaml
	npx --yes ajv-cli compile -s "contracts/events/*.schema.json" -s contracts/capability.schema.json

# Boundary guard + vet (spec P1/P4).
lint:
	cd server && golangci-lint run ./...

test:
	cd server && go test ./...
