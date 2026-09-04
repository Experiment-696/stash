#!/usr/bin/env bash
set -euo pipefail

export PATH="/home/default/.local/go/bin:$PATH"
export GOCACHE="/home/default/.cache/go-build"

cd /mnt/c/stash-master

go generate ./ui
test -s ui/login/locales/en-GB.json

go test ./internal/api \
  -run 'TestRoleSafeConfiguration|TestAuthenticatedAppUsesFullConfigurationProvider|TestPreMigrationSPAOperationManifest' \
  -count=1

mkdir -p /home/default/stash-releases
go build \
  -buildvcs=false \
  -trimpath \
  -ldflags='-s -w' \
  -o /home/default/stash-releases/stash-p1a-f031-linux-amd64 \
  ./cmd/stash

sha256sum /home/default/stash-releases/stash-p1a-f031-linux-amd64
