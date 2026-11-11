#!/usr/bin/env bash
# Throwaway COI-3591 probe: can a CI emulator hear a PulseAudio null source.
set -uo pipefail

echo 'KERNEL=="kvm", GROUP="kvm", MODE="0666", OPTIONS+="static_node=kvm"' | sudo tee /etc/udev/rules.d/99-kvm4all.rules >/dev/null
sudo udevadm control --reload-rules
sudo udevadm trigger --name-match=kvm
sudo udevadm settle || true
sudo apt-get update -qq
sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends libpulse0 libgl1 pulseaudio pulseaudio-utils sox espeak-ng
export ANDROID_HOME=/usr/local/lib/android/sdk
export ANDROID_AVD_HOME="$HOME/.android/avd"
mkdir -p "$ANDROID_AVD_HOME"
export PATH="$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH"
IMG="system-images;android-34;google_apis;x86_64"
yes | sdkmanager --licenses >/dev/null || true
sdkmanager "emulator" "$IMG"
echo no | avdmanager create avd --force --name ci --package "$IMG" --device pixel_6
emulator -help-all 2>&1 | grep -i -E "audio|mic|grpc" | head -20
espeak-ng -s 130 -w /tmp/speech.wav "hello world this is a test"
sox /tmp/speech.wav -n stat 2>&1 | grep -E "Maximum amplitude|Length"

start_pulse() {
  pulseaudio --kill 2>/dev/null || true
  sleep 1
  mkdir -p /tmp/pa
  chmod 700 /tmp/pa
  export XDG_RUNTIME_DIR=/tmp/pa
  pulseaudio -D --exit-idle-time=-1 --load="module-native-protocol-unix auth-anonymous=1 socket=/tmp/pa/native" --load="module-null-sink sink_name=virt_mic"
  export PULSE_SERVER=unix:/tmp/pa/native
  pactl set-default-source virt_mic.monitor
  pactl info | grep -E "Server String|Default Source"
}

try() {
  local label="$1"; shift
  echo "######## variant: $label"
  start_pulse
  env "$@" nohup emulator -avd ci -no-window -no-boot-anim -no-snapshot -gpu swiftshader_indirect -accel on -allow-host-audio >emulator.log 2>&1 &
  local pid=$! waited=0
  until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
    kill -0 "$pid" 2>/dev/null || { echo "emulator exited"; tail -15 emulator.log; return 1; }
    [ "$waited" -lt 240 ] || { echo "no boot"; kill "$pid"; return 1; }
    sleep 3
    waited=$((waited + 3))
  done
  echo "booted after ${waited}s"
  grep -n -i -E "audio|pulse" emulator.log | head
  adb shell 'which tinycap tinymix tinyplay arecord' 2>&1 | head
  adb shell 'ls /dev/snd 2>&1' | head
  ( sleep 2; paplay --device=virt_mic /tmp/speech.wav ) &
  adb shell 'tinycap /data/local/tmp/cap.wav -D 0 -d 0 -r 16000 -c 1 -b 16 -T 6' 2>&1 | tail -3
  adb pull /data/local/tmp/cap.wav /tmp/cap.wav 2>&1 | tail -1
  sox /tmp/cap.wav -n stat 2>&1 | grep -E "Maximum amplitude|Length|RMS"
  adb emu kill || kill "$pid"
  sleep 8
}

try pa-env-vars QEMU_AUDIO_DRV=pa QEMU_PA_SERVER=unix:/tmp/pa/native QEMU_PA_SOURCE=virt_mic.monitor QEMU_PA_SINK=virt_mic PULSE_SERVER=unix:/tmp/pa/native XDG_RUNTIME_DIR=/tmp/pa || true
try plain PULSE_SERVER=unix:/tmp/pa/native XDG_RUNTIME_DIR=/tmp/pa || true
try default-socket XDG_RUNTIME_DIR=/tmp/pa || true
