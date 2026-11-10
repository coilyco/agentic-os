#!/usr/bin/env bash
# Throwaway COI-3460 probe of a CI emulator's speech service and mic.
set -uo pipefail

echo 'KERNEL=="kvm", GROUP="kvm", MODE="0666", OPTIONS+="static_node=kvm"' | sudo tee /etc/udev/rules.d/99-kvm4all.rules >/dev/null
sudo udevadm control --reload-rules
sudo udevadm trigger --name-match=kvm
sudo udevadm settle || true
ls -l /dev/kvm
sudo apt-get update -qq
sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends libpulse0 libgl1 pulseaudio pulseaudio-utils sox
export ANDROID_HOME=/usr/local/lib/android/sdk
export PATH="$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH"
IMG="system-images;android-34;google_apis;x86_64"
yes | sdkmanager --licenses >/dev/null || true
sdkmanager "emulator" "$IMG"
echo no | avdmanager create avd --force --name ci --package "$IMG" --device pixel_6
pulseaudio --start --exit-idle-time=-1 --log-target=stderr || true
pactl load-module module-null-sink sink_name=virt_mic sink_properties=device.description=virt_mic
pactl set-default-source virt_mic.monitor
pactl list short sources
adb start-server
queries() {
  echo "== booted"
  adb shell getprop ro.build.fingerprint
  echo "== google packages"
  adb shell pm list packages | grep -i -E "google|speech|voice|tts" | sort
  echo "== recognition services"
  adb shell cmd package query-services --brief -a android.speech.RecognitionService
  echo "== secure settings"
  adb shell settings get secure voice_recognition_service
  echo "== googlequicksearchbox"
  adb shell dumpsys package com.google.android.googlequicksearchbox | grep -i -E "versionName|enabled" | head -5
  echo "== audio input devices"
  adb shell dumpsys media.audio_flinger | grep -i -E "input|source" | head -20
  echo "== net"
  adb shell ping -c1 -W3 8.8.8.8 | tail -2
}

try() {
  local label="$1"; shift
  echo "######## variant: $label"
  nohup emulator -avd ci -no-window -no-boot-anim -no-snapshot -gpu swiftshader_indirect -accel on "$@" >emulator.log 2>&1 &
  local pid=$! waited=0
  until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
    kill -0 "$pid" 2>/dev/null || { echo "emulator exited after ${waited}s"; tail -25 emulator.log; return 1; }
    [ "$waited" -lt 300 ] || { echo "no boot in 300s"; tail -25 emulator.log; kill "$pid"; return 1; }
    sleep 3
    waited=$((waited + 3))
  done
  echo "booted after ${waited}s"
  queries
  grep -i -E "audio|pulse|mic" emulator.log | head -10
  adb emu kill || kill "$pid"
  sleep 8
}

try audio-pulse -allow-host-audio || true
try no-audio -no-audio || true
