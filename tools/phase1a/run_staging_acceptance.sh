#!/usr/bin/env bash
set -euo pipefail

repo="${1:-/mnt/c/stash-master}"
staging_config="${2:-/home/default/.stash-p1a-staging/config.yml}"
port="${3:-19999}"

staging_dir="$(dirname "$(realpath -m "$staging_config")")"
case "$staging_dir" in
  /home/default/.stash|/home/default/.stash/*)
    echo "refusing to run acceptance against the original installation" >&2
    exit 2
    ;;
esac
if [[ ! -f "$staging_config" ]]; then
  echo "staging config does not exist: $staging_config" >&2
  exit 2
fi

cd "$repo"
go generate ./cmd/stash
go test -buildvcs=false ./internal/authz ./internal/authservice ./internal/api ./pkg/sqlite ./cmd/authz-inventory
go run -buildvcs=false ./cmd/authz-inventory --check-policy internal/authz/graphql_policy.json
go build -buildvcs=false -o /tmp/stash-p1a-acceptance ./cmd/stash

echo "starting disposable acceptance server on 127.0.0.1:$port using $staging_config"
echo "the original /home/default/.stash directory is explicitly rejected by this script"
exec /tmp/stash-p1a-acceptance --config "$staging_config" --host 127.0.0.1 --port "$port" --nobrowser
