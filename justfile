set shell := ["zsh", "-cu"]

default: check

# Regenerate the Go API surface from the schema snapshot.
gen:
    go generate ./...

# Fail when the checked-in generated code differs from the generator output.
check-gen: gen
    git diff --exit-code -- '*_gen.go'

build:
    go build ./...

test:
    go test -race ./...

lint:
    golangci-lint run ./...

check: build test lint check-gen

# Live tests against a Herdr server the suite starts itself (see internal/e2e).
# -v keeps the coverage report the suite prints visible on a passing run.
e2e:
    go test -tags e2e -count=1 -v ./internal/e2e/...

# Report how far the installed herdr and the running server have moved from
# the schema snapshot. Exits non-zero on drift that known-gaps.json does not
# already account for.
herdr-check:
    go run ./internal/cmd/herdrcheck

# Refresh the schema snapshot from the installed herdr binary.
schema-update:
    herdr api schema --output schema/herdr-api.schema.json
    herdr --version
