module github.com/coworkos/cowork-os/v2/server

go 1.22

// Dependencies are added in M1 (walking skeleton) via the toolchain, e.g.:
//   github.com/jackc/pgx/v5        (DB driver)
//   github.com/pressly/goose/v3    (migrations)
//   github.com/labstack/echo/v4    (HTTP; net/http compatible -- NOT Fiber)
//   go.opentelemetry.io/otel       (tracing)
// Versions are resolved by `go get`, never hand-written.
