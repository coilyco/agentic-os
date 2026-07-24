#!/usr/bin/env bash
# Select the first reachable Docker daemon on the image runner.

set -euo pipefail

gw=$(node -e "const fs=require('fs');const l=(fs.readFileSync('/proc/net/route','utf8').split('\n').find(x=>x.split('\t')[1]==='00000000'))||'';const h=l.split(/\s+/)[2]||'';if(h){console.log(h.match(/../g).reverse().map(x=>parseInt(x,16)).join('.'))}" || true)
for host in "tcp://${gw}:2375" "${DOCKER_HOST:-}" tcp://localhost:2375 unix:///var/run/docker.sock; do
  [ -z "$host" ] && continue
  [ "$host" = "tcp://:2375" ] && continue
  if docker -H "$host" info >/dev/null 2>&1; then
    echo "docker daemon reachable at $host"
    echo "DOCKER_HOST=$host" >> "$GITHUB_ENV"
    echo "selected DOCKER_HOST=$host"
    docker -H "$host" version
    exit 0
  fi
  echo "no docker daemon at $host"
done
echo "no reachable docker daemon" >&2
exit 1
