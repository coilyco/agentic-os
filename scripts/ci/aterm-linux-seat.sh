#!/usr/bin/env bash
# Linux seat proof (COI-3498): an unprivileged daemon with no TCP listener, a stub seat
# that takes an image, a second uid shut out. Why: references/linux-seat.md.

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
out="${ATERM_LINUX_OUT:-$repo_root/aterm-linux-out}"
SEAT_USER=aterm-seat
OTHER_USER=aterm-other
PYTHON=(python3 -I)

fail() {
  echo "::error::$*" >&2
  exit 1
}

# listeners PID...: the TCP listeners those processes hold, one per line, none when quiet.
listeners() {
  sudo ss -ltnpH | grep -E "pid=($(IFS='|'; echo "$*"))[,)]" || true
}

make_users() {
  for name in "$SEAT_USER" "$OTHER_USER"; do
    id "$name" >/dev/null 2>&1 || sudo useradd --create-home --shell /bin/bash "$name"
  done
  mkdir -p "$out"
  chmod 755 "$HOME" "$repo_root" "$out"
}

# as USER ENV... -- CMD...: a command as that user with only the given environment.
as() {
  local user="$1"
  shift
  sudo -u "$user" env -i PATH="$PATH" HOME="/home/$user" "$@"
}

build() {
  (cd "$repo_root/aterm" && go build -o "$out/aterm-bin" .)
  chmod 755 "$out/aterm-bin"
}

start_daemon() {
  local uid socket_dir
  uid=$(id -u "$SEAT_USER")
  socket_dir="/tmp/aterm-$uid"
  sudo mkdir -p "$socket_dir"
  sudo chown "$SEAT_USER" "$socket_dir"
  sudo chmod 700 "$socket_dir"
  as "$SEAT_USER" ATERM_DAEMON_WS= ATERM_DAEMON_TAILNET_PORT= \
    "$out/aterm-bin" daemon --socket "$socket_dir/d.sock" --idle 0 >"$out/daemon.log" 2>&1 &
  local waited=0
  until sudo test -S "$socket_dir/d.sock"; do
    [ "$waited" -lt 20 ] || { cat "$out/daemon.log" >&2; fail "the daemon never opened its socket"; }
    sleep 1
    waited=$((waited + 1))
  done
  echo "$socket_dir" >"$out/socket-dir"
}

# pids_of USER: every process the user runs, the daemon and its holders included.
pids_of() {
  pgrep -u "$1" -d ' ' . || true
}

step_no_listener() {
  local pids found
  pids=$(pids_of "$SEAT_USER")
  [ -n "$pids" ] || fail "no process runs as $SEAT_USER"
  # shellcheck disable=SC2086
  found=$(listeners $pids)
  [ -z "$found" ] || { echo "$found" >&2; fail "the seat user holds a TCP listener"; }
  echo "no TCP listener for $SEAT_USER (pids: $pids)"
}

# A control, so a silent ss is not a pass: the same binary with the websocket on.
step_control_listener() {
  local dir=/tmp/aterm-control-dir
  sudo -u "$SEAT_USER" mkdir -m 700 -p "$dir"
  as "$SEAT_USER" ATERM_DAEMON_WS=127.0.0.1:17421 ATERM_DAEMON_TAILNET_PORT= \
    "$out/aterm-bin" daemon --socket "$dir/c.sock" --idle 0 >"$out/control.log" 2>&1 &
  local pid="" waited=0
  until [ -n "$pid" ] && [ -n "$(listeners "$pid")" ]; do
    [ "$waited" -lt 15 ] || { cat "$out/control.log" >&2; fail "the control daemon never showed a listener, so ss proves nothing"; }
    sleep 1
    waited=$((waited + 1))
    pid=$(pgrep -u "$SEAT_USER" -f "daemon --socket $dir/c.sock" | head -n1 || true)
  done
  echo "control: $(listeners "$pid" | head -n1 | cut -c1-100)"
  sudo kill "$pid"
}

step_seat_takes_an_image() {
  local dir socket png="$out/seed.png"
  dir=$(cat "$out/socket-dir")
  socket="$dir/d.sock"
  "${PYTHON[@]}" -c "import sys; sys.path.insert(0, '$repo_root/scripts/ci'); import aterm_android_journey as j; from pathlib import Path; j.seed_png(Path('$png'))"
  sudo chown "$SEAT_USER" "$png"
  as "$SEAT_USER" "${PYTHON[@]}" "$repo_root/scripts/ci/aterm_android_seat.py" start "$socket" linux-seat "$dir/seat.log"
  as "$SEAT_USER" "${PYTHON[@]}" "$repo_root/scripts/ci/aterm_android_seat.py" upload "$socket" linux-seat "$png"
  as "$SEAT_USER" "${PYTHON[@]}" "$repo_root/scripts/ci/aterm_android_seat.py" expect-image "$socket" linux-seat "$png" "$dir/seat.log" --timeout 20
}

step_other_uid_is_shut_out() {
  local dir
  dir=$(cat "$out/socket-dir")
  if as "$OTHER_USER" "${PYTHON[@]}" "$repo_root/scripts/ci/aterm_android_seat.py" start "$dir/d.sock" intruder /tmp/intruder.log 2>"$out/other.err"; then
    fail "another uid reached the seat user's socket"
  fi
  echo "another uid refused: $(tail -n1 "$out/other.err" | cut -c1-120)"
  mode=$(sudo stat -c '%a' "$dir")
  [ "$mode" = "700" ] || fail "the socket directory is mode $mode, not 700"
  echo "socket directory mode $mode"
}

step_loopback_is_closed() {
  local code=0
  "${PYTHON[@]}" "$repo_root/scripts/aterm_listener_probe.py" 127.0.0.1 7419 --origin http://127.0.0.1:7419 --marker "$out/probe-m" || code=$?
  [ "$code" = "2" ] || fail "the loopback probe exited $code, expected 2 (nothing listening)"
  echo "loopback 7419 is closed to a probe"
}

stop_all() {
  sudo pkill -u "$SEAT_USER" || true
}

cmd="${1:-all}"
case "$cmd" in
  all)
    make_users
    build
    start_daemon
    step_no_listener
    step_control_listener
    step_seat_takes_an_image
    step_other_uid_is_shut_out
    step_loopback_is_closed
    step_no_listener
    stop_all
    echo "linux seat proof: passed"
    ;;
  *) fail "usage: aterm-linux-seat.sh [all]" ;;
esac
