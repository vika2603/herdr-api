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

# Live tests against a dedicated named Herdr session (see internal/e2e).
e2e:
    go test -tags e2e -count=1 ./internal/e2e/...

# Refresh the schema snapshot from the installed herdr binary.
schema-update:
    herdr api schema --output schema/herdr-api.schema.json
    herdr --version
