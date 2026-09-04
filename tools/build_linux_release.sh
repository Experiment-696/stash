#!/bin/sh
set -eu

output=${1:-dist/stash-linux-amd64}
build_date=${BUILD_DATE:-$(date -u +%Y%m%d)}
git_hash=${GITHASH:-unknown}
version=${STASH_VERSION:-stash-rework-dev}

mkdir -p "$(dirname "$output")"

CGO_ENABLED=1 go build \
  -o "$output" \
  -tags "sqlite_stat4 sqlite_math_functions sqlite_omit_load_extension osusergo netgo" \
  -buildvcs=false \
  -trimpath \
  -buildmode=pie \
  -ldflags "-s -w -extldflags=-static-pie -X github.com/stashapp/stash/internal/build.buildstamp=$build_date -X github.com/stashapp/stash/internal/build.githash=$git_hash -X github.com/stashapp/stash/internal/build.version=$version -X github.com/stashapp/stash/internal/build.officialBuild=false" \
  ./cmd/stash

sha256sum "$output" > "$output.sha256"
