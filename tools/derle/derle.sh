#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd -- "$root"

if command -v go >/dev/null 2>&1; then
    go_bin=$(command -v go)
elif [[ -x '/c/Program Files/Go/bin/go.exe' ]]; then
    go_bin='/c/Program Files/Go/bin/go.exe'
else
    printf 'hata: Go bulunamadı (PATH ya da C:\\Program Files\\Go\\bin)\n' >&2
    exit 1
fi

cache="$root/.gocache"
if command -v cygpath >/dev/null 2>&1; then
    cache=$(cygpath -w "$cache")
fi
export GOFLAGS="${GOFLAGS--mod=vendor}"
export GOPROXY="${GOPROXY-off}"
export GOCACHE="${GOCACHE-$cache}"
export CGO_ENABLED=0

version=$(git describe --tags --always --dirty)
commit=$(git rev-parse --short HEAD)
pkg='github.com/MehmetKoc16/Agent-message/internal/client'
ldflags="-s -w -X $pkg.version=$version -X $pkg.commit=$commit"
mkdir -p dist
outputs=()

build() {
    local os="$1" arch="$2" name="$3" command="$4"
    GOOS="$os" GOARCH="$arch" "$go_bin" build -trimpath -ldflags "$ldflags" -o "dist/$name" "$command"
    outputs+=("$name")
    printf 'derlendi: %s\n' "$name"
}

build windows amd64 relay-windows-amd64.exe ./cmd/relay
build windows arm64 relay-windows-arm64.exe ./cmd/relay
build linux amd64 relay-linux-amd64 ./cmd/relay
build linux arm64 relay-linux-arm64 ./cmd/relay
build darwin amd64 relay-darwin-amd64 ./cmd/relay
build darwin arm64 relay-darwin-arm64 ./cmd/relay
build linux amd64 agent-relay-linux-amd64 ./cmd/agent-relay
build linux arm64 agent-relay-linux-arm64 ./cmd/agent-relay

if command -v sha256sum >/dev/null 2>&1; then
    (cd dist && sha256sum "${outputs[@]}") > dist/SHA256SUMS
else
    "$go_bin" run ./tools/derle/sha256 dist "${outputs[@]}" > dist/SHA256SUMS
fi
printf 'tamam: 8 ikili ve dist/SHA256SUMS üretildi\n'
