#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
command -v go >/dev/null || { echo 'Go >=1.26 required'; exit 1; }
command -v npm >/dev/null || { echo 'Node.js >=20.19 with npm required'; exit 1; }
(cd web && npm install && npm run build)
go mod tidy
gofmt -w cmd internal
go test ./...
go vet ./...
mkdir -p bin
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/yuanyang ./cmd/panel
echo 'Built bin/yuanyang and web/dist'
