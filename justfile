# Definition of green: run before saying done.
check: vet test nvim-test

vet:
    go vet ./...

test:
    go test ./...

nvim-test:
    nvim --headless -u NONE -l tests/smoke.lua

build:
    go build -o bin/dataview ./cmd/dataview
