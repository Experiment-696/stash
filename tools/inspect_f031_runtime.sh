#!/usr/bin/env bash
set -euo pipefail

echo "PROCESSES"
ps -eo pid,lstart,args | grep '[s]tash'

echo "RELEASES"
ls -l --time-style=long-iso /home/default/stash-releases

echo "CONFIG"
find /home/default -maxdepth 3 -type f \( -name config.yml -o -name config.yaml \) -print
