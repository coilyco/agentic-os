#!/usr/bin/env bash
# The emulator job's steps (COI-3485), one verb per workflow step. What each does and
# what it cannot prove: tooling-aterm-client references/android.md.

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
export ANDROID_HOME="${ANDROID_HOME:-$HOME/android-sdk}"
IMAGE="${ATERM_EMULATOR_IMAGE:-system-images;android-34;google_apis;x86_64}"
AVD=ci
SEAT=emulator-seat
out="${ATERM_EMULATOR_OUT:-$repo_root/aterm-emulator-out}"
# A unix socket path caps near 100 bytes, which a runner's workspace path can exceed.
state="${ATERM_EMULATOR_STATE:-/tmp/aterm-emulator}"
# The virtual mic: the emulator's gRPC injectAudio, on this port (COI-3591).
grpc_port=8554

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
  # The 2 GB default let lmkd kill system apps once the Vosk model loaded (COI-3591).
  local config="${ANDROID_AVD_HOME:-$HOME/.android/avd}/$AVD.avd/config.ini"
  sed -i '/^hw.ramSize/d' "$config"
  echo "hw.ramSize=4096" >>"$config"
  command -v emulator || fail "no emulator binary on PATH after sdkmanager"
  emulator -version | head -n3
  emulator -list-avds | grep -qx "$AVD" || fail "emulator does not list $AVD, AVD home is $ANDROID_AVD_HOME"
}

# audio: a spoken phrase, as a stream the emulator's gRPC injectAudio feeds its mic.
# PulseAudio never started under the emulator (`Could not init pa`). gRPC avoids it.
audio() {
  mkdir -p "$out/bin"
  sudo apt-get update -qq
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends espeak-ng sox
  GOBIN="$out/bin" go install github.com/fullstorydev/grpcurl/cmd/grpcurl@v1.9.1
  espeak-ng -s 130 -w "$out/speech-raw.wav" "${ATERM_LANE_PHRASE:-hello world this is a test}"
  sox "$out/speech-raw.wav" -r 16000 -c 1 -b 16 "$out/speech.wav"
  python3 -I "$repo_root/scripts/ci/aterm_audio_stream.py" "$out/speech.wav" "$out/speech.jsonl"
  vosk_model
}

# vosk_model: the debug build's offline recognizer reads this model. Pinned by hash, so a
# changed download fails the job. Apache-2.0, from the vosk-api authors (alphacephei).
vosk_model() {
  local zip="$out/vosk-model.zip" sha=30f26242c4eb449f948e42cb302dd7a686cb29a3423a8367f99ff41780942498
  curl -fsSL --retry 3 -o "$zip" https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip
  [ "$(sha256sum "$zip" | cut -d' ' -f1)" = "$sha" ] || fail "the Vosk model download does not match its pinned hash"
  rm -rf "$out/vosk-model" "$out/vosk-unpacked"
  unzip -q "$zip" -d "$out/vosk-unpacked"
  mv "$out/vosk-unpacked"/* "$out/vosk-model"
  rm -rf "$out/vosk-unpacked" "$zip"
}

boot() {
  use_sdk
  mkdir -p "$out"
  adb start-server
  # With a phrase to inject, keep audio and open the gRPC port. Otherwise none.
  local sound=(-no-audio)
  [ -s "$out/speech.jsonl" ] && sound=(-grpc "$grpc_port")
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

# speech_env: where the phrase and grpcurl are, for the journey scripts, once `audio` ran.
speech_env() {
  [ -s "$out/speech.jsonl" ] || return 0
  export ATERM_SPEECH_JSONL="$out/speech.jsonl" ATERM_GRPCURL="$out/bin/grpcurl" ATERM_GRPC_PORT="$grpc_port"
  if [ -d "$out/vosk-model/am" ]; then export ATERM_VOSK_MODEL="$out/vosk-model"; fi
}

journey() {
  use_sdk
  local apk="${1:?usage: journey APK}"
  speech_env
  python3 -I "$repo_root/scripts/ci/aterm_android_journey.py" --apk "$apk" --out "$out/screens" \
    --socket "$state/d.sock" --seat "$SEAT" --seat-log "$state/seat.log" --refuse-file "$state/refuse-upload"
}

# apk: the debug APK to prove, from a run's artifact or built from the app checkout.
# ATERM_LANE_APP_DIR is the ref under test, ATERM_LANE_APK_RUN a run id or empty.
apk() {
  local dest="${1:?usage: apk DEST}" app="${ATERM_LANE_APP_DIR:-$repo_root}" run="${ATERM_LANE_APK_RUN:-}"
  mkdir -p "$(dirname "$dest")"
  # The emulator image step needs sdkmanager, so the toolchain goes in either way.
  bash "$repo_root/scripts/ci/aterm-android.sh" toolchain
  if [ -n "$run" ]; then
    local tmp
    tmp=$(mktemp -d)
    gh run download "$run" --repo "$GITHUB_REPOSITORY" --name "aterm-android-debug-x86_64-$run" --dir "$tmp"
    cp "$(find "$tmp" -name '*.apk' | head -n1)" "$dest"
  else
    bash "$app/scripts/ci/aterm-android.sh" build-debug x86_64 "$dest"
  fi
  ls -l "$dest"
}

# lane: a seat's step file, run by aterm_android_lane.py. It comes from the app checkout.
lane() {
  use_sdk
  local apk="${1:?usage: lane APK STEPS}" steps="${2:?usage: lane APK STEPS}" app="${ATERM_LANE_APP_DIR:-$repo_root}"
  # An older ref has no step file of its own, so the lane's own copy is the fallback.
  if [ ! -f "$steps" ] && [ -f "$repo_root/${steps#"$app"/}" ]; then steps="$repo_root/${steps#"$app"/}"; fi
  [ -f "$steps" ] || fail "no step file at $steps, in the ref or in the lane's own checkout"
  speech_env
  python3 -I "$repo_root/scripts/ci/aterm_android_lane.py" --apk "$apk" --out "$out/screens" --steps "$steps" \
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
  kvm | avd | audio | boot | daemon | journey | apk | lane | collect) "$cmd" "$@" ;;
  *) fail "usage: aterm-android-emulator.sh kvm | avd | audio | boot | daemon | journey APK | apk DEST | lane APK STEPS | collect" ;;
esac
