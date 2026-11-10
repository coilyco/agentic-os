#!/usr/bin/env bash
# The emulator job's steps (COI-3485), one verb per workflow step. What each does and
# what it cannot prove: tooling-aterm-client references/android.md.

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
export ANDROID_HOME="${ANDROID_HOME:-$HOME/android-sdk}"
IMAGE="system-images;android-34;google_apis;x86_64"
AVD=ci
SEAT=emulator-seat
out="${ATERM_EMULATOR_OUT:-$repo_root/aterm-emulator-out}"
# A unix socket path caps near 100 bytes, which a runner's workspace path can exceed.
state="${ATERM_EMULATOR_STATE:-/tmp/aterm-emulator}"
# The virtual mic: a PulseAudio null sink whose monitor the emulator records (COI-3460).
pulse_dir=/tmp/aterm-pulse

fail() {
  echo "::error::$*" >&2
  exit 1
}

use_sdk() {
  # avdmanager and emulator must agree on where AVDs live, so name it for both.
  export ANDROID_AVD_HOME="$HOME/.android/avd"
  mkdir -p "$ANDROID_AVD_HOME"
  local java_home
  for java_home in /usr/lib/jvm/java-17-openjdk-*; do
    [ -x "$java_home/bin/java" ] && export JAVA_HOME="$java_home" && break
  done
  export PATH="${JAVA_HOME:+$JAVA_HOME/bin:}$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH"
}

kvm() {
  echo 'KERNEL=="kvm", GROUP="kvm", MODE="0666", OPTIONS+="static_node=kvm"' | sudo tee /etc/udev/rules.d/99-kvm4all.rules >/dev/null
  sudo udevadm control --reload-rules
  sudo udevadm trigger --name-match=kvm
  sudo udevadm settle || true
  local waited=0
  until [ -w /dev/kvm ]; do
    if [ "$waited" -ge 15 ]; then
      ls -l /dev/kvm >&2 || true
      echo "cpu virtualization flags: $(grep -c -E 'vmx|svm' /proc/cpuinfo)" >&2
      fail "/dev/kvm is not writable, so this runner cannot accelerate an x86_64 emulator"
    fi
    sleep 1
    waited=$((waited + 1))
  done
  echo "kvm ready after ${waited}s"
}

avd() {
  use_sdk
  # The emulator links libpulse even with -no-audio, and the runner image lacks it.
  sudo apt-get update -qq
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends libpulse0 libgl1
  yes | sdkmanager --licenses >/dev/null || true
  sdkmanager "emulator" "$IMAGE"
  echo no | avdmanager create avd --force --name "$AVD" --package "$IMAGE" --device pixel_6
  command -v emulator || fail "no emulator binary on PATH after sdkmanager"
  emulator -version | head -n3
  emulator -list-avds | grep -qx "$AVD" || fail "emulator does not list $AVD, AVD home is $ANDROID_AVD_HOME"
}

# audio: a virtual mic for the dictation journey, and the spoken phrase it plays. The
# emulator reads the host's default source, so the sink's monitor is the mic.
audio() {
  mkdir -p "$out" "$pulse_dir"
  sudo apt-get update -qq
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends pulseaudio pulseaudio-utils espeak-ng
  # The emulator finds pulse through XDG_RUNTIME_DIR, so both share this one.
  chmod 700 "$pulse_dir"
  export XDG_RUNTIME_DIR="$pulse_dir"
  pulseaudio -D --exit-idle-time=-1 --load="module-native-protocol-unix auth-anonymous=1 socket=$pulse_dir/native" --load="module-null-sink sink_name=virt_mic"
  export PULSE_SERVER="unix:$pulse_dir/native"
  pactl set-default-source virt_mic.monitor
  pactl info | grep -E "Default Source|Server Name"
  espeak-ng -s 130 -w "$out/speech.wav" "hello world this is a test"
  ls -l "$out/speech.wav"
}

boot() {
  use_sdk
  mkdir -p "$out"
  adb start-server
  # With the virtual mic set up, the emulator gets host audio. Without it, none.
  local sound=(-no-audio)
  if [ -S "$pulse_dir/native" ]; then
    export XDG_RUNTIME_DIR="$pulse_dir" PULSE_SERVER="unix:$pulse_dir/native"
    sound=(-allow-host-audio)
  fi
  nohup emulator -avd "$AVD" -no-window "${sound[@]}" -no-boot-anim -no-snapshot -gpu swiftshader_indirect -accel on >"$out/emulator.log" 2>&1 &
  local emu_pid=$! waited=0
  # Every wait is bounded and checks the emulator is alive, so a dead one fails fast.
  until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
    kill -0 "$emu_pid" 2>/dev/null || { tail -30 "$out/emulator.log" >&2; fail "the emulator process exited after ${waited}s"; }
    [ "$waited" -lt 420 ] || { tail -30 "$out/emulator.log" >&2; adb devices >&2; fail "the emulator did not finish booting in 420 s"; }
    sleep 3
    waited=$((waited + 3))
  done
  for scale in window_animation_scale transition_animation_scale animator_duration_scale; do
    adb shell settings put global "$scale" 0
  done
  # A slow CI emulator raises ANR dialogs for system apps, which take focus from the app.
  adb shell settings put global hide_error_dialogs 1
  adb shell input keyevent 82
  echo "emulator booted after ~${waited}s: $(adb shell getprop ro.product.cpu.abi | tr -d '\r')"
}

# daemon: a real aterm daemon on 7420 with the tailnet off, the COI-3494 relay on 7419
# (the app's origin is refused by the loopback listener), and the stub seat.
daemon() {
  mkdir -p "$out" "$state"
  chmod 700 "$state"
  (cd "$repo_root/aterm" && go build -o "$out/aterm" .)
  nohup "$out/aterm" daemon --socket "$state/d.sock" --websocket 127.0.0.1:7420 --tailnet-port "" --idle 0 >"$out/daemon.log" 2>&1 &
  nohup python3 -I "$repo_root/scripts/aterm_test_rig_proxy.py" --listen 127.0.0.1:7419 --upstream 127.0.0.1:7420 --refuse-uploads-file "$state/refuse-upload" >"$out/relay.log" 2>&1 &
  local waited=0
  until [ -S "$state/d.sock" ]; do
    [ "$waited" -lt 20 ] || { cat "$out/daemon.log" >&2; fail "the daemon never opened its socket"; }
    sleep 1
    waited=$((waited + 1))
  done
  python3 -I "$repo_root/scripts/ci/aterm_android_seat.py" start "$state/d.sock" "$SEAT" "$state/seat.log"
}

journey() {
  use_sdk
  local apk="${1:?usage: journey APK}"
  [ -S "$pulse_dir/native" ] && export XDG_RUNTIME_DIR="$pulse_dir" PULSE_SERVER="unix:$pulse_dir/native" ATERM_SPEECH_WAV="$out/speech.wav"
  python3 -I "$repo_root/scripts/ci/aterm_android_journey.py" --apk "$apk" --out "$out/screens" \
    --socket "$state/d.sock" --seat "$SEAT" --seat-log "$state/seat.log" --refuse-file "$state/refuse-upload"
}

# collect: the seat's log and the final logcat, for the artifact.
collect() {
  use_sdk
  mkdir -p "$out/screens"
  cp "$state/seat.log" "$out/screens/seat.log" 2>/dev/null || true
  # logcat waits forever with no device, which hung a run to its job limit.
  timeout 30 adb logcat -d -t 6000 >"$out/screens/logcat-final.txt" 2>/dev/null || true
}

cmd="${1:-}"
[ $# -gt 0 ] && shift
case "$cmd" in
  kvm | avd | audio | boot | daemon | journey | collect) "$cmd" "$@" ;;
  *) fail "usage: aterm-android-emulator.sh kvm | avd | audio | boot | daemon | journey APK | collect" ;;
esac
