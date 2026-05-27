# note-searcher build targets
# Run inside `nix develop`

default:
    @just --list

build:
    mkdir -p bin
    go build -o bin/note-searcher ./cmd/note-searcher

test:
    go test ./...

lint:
    staticcheck ./...
    govulncheck ./...

fmt:
    goimports -w .

gomod2nix:
    gomod2nix
