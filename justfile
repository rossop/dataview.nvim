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

# §V.5/§V.6 budgets on a synthetic 10k-note vault, against the real binary.
perf:
    DATAVIEW_PERF=1 go test ./internal/perf -count=1 -v -run TestBudgets

# Cross-compile release binaries + checksums.txt into dist/.
# Static (CGO off) and stripped so one small file runs anywhere (§C).
dist version=`nvim --headless -u NONE -l /dev/stdin <<< 'io.write(dofile("lua/dataview/version.lua"))'`:
    #!/usr/bin/env bash
    set -euo pipefail
    rm -rf dist && mkdir dist
    for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
      os=${target%/*}; arch=${target#*/}; ext=""; [ "$os" = windows ] && ext=.exe
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
        -ldflags "-s -w -X main.version={{version}}" \
        -o "dist/dataview_${os}_${arch}${ext}" ./cmd/dataview
    done
    (cd dist && shasum -a 256 dataview_* > checksums.txt)
    ls -lh dist

# Tag a release: version.lua must already say <version>. CI builds and
# publishes the assets when the tag is pushed.
release version: check
    #!/usr/bin/env bash
    set -euo pipefail
    have=$(nvim --headless -u NONE -l /dev/stdin <<< 'io.write(dofile("lua/dataview/version.lua"))')
    [ "$have" = "{{version}}" ] || { echo "lua/dataview/version.lua says $have, not {{version}}"; exit 1; }
    git tag -a "v{{version}}" -m "v{{version}}"
    echo "tagged v{{version}}; push the tag to publish: git push origin v{{version}}"
