#!/usr/bin/env bash
set -euo pipefail

binary=/home/default/stash-releases/stash-p1a-f031-linux-amd64
config=/home/default/.stash/config.yml
log=/home/default/.stash/stash-p1a-f031.log
pidfile=/home/default/.stash/stash-p1a-f031.pid

test -x "$binary"
test -f "$config"

nohup "$binary" --config "$config" --nobrowser >"$log" 2>&1 &
pid=$!
echo "$pid" >"$pidfile"

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:9999/ >/dev/null; then
    echo "PID=$pid"
    echo "LOG=$log"
    exit 0
  fi
  sleep 1
done

tail -n 80 "$log"
exit 1
