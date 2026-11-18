# Definition of green: run before saying done.
check: vet test nvim-test

vet:
    go vet ./...

test:
    go test ./...

# Headless Neovim specs; e2e specs drive the freshly built binary.
nvim-test: build
    nvim --headless -u NONE -l tests/run.lua

build:
    go build -o bin/dataview ./cmd/dataview
