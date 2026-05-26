# note-searcher build targets
# Run inside `nix develop`

default:
    @just --list

build:
    go build ./...

test:
    go test ./...

lint:
    staticcheck ./...
    govulncheck ./...

fmt:
    goimports -w .

gomod2nix:
    gomod2nix
