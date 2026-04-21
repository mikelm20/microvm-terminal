// The module path is nested under control-plane/ so this package is allowed
// to import control-plane/internal/* per Go's `internal` rule, which is
// path-hierarchy based (not module-boundary based). The directory lives at
// tests/learner-bot/ per CONTRACTS.md; the mismatch between on-disk path and
// module path is intentional and isolated to this one test module.
module github.com/mikelm20/learn-platform/control-plane/tests/learner-bot

go 1.25.0

require (
	github.com/mikelm20/learn-platform/control-plane v0.0.0
	github.com/mikelm20/learn-platform/vm-image/claude-wrap v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.1 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/mikelm20/learn-platform/control-plane => ../../control-plane

replace github.com/mikelm20/learn-platform/vm-image/claude-wrap => ../../vm-image/claude-wrap
