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
nohup emulator -avd ci -no-window -allow-host-audio -no-boot-anim -no-snapshot -gpu swiftshader_indirect -accel on >emulator.log 2>&1 &
until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do sleep 3; done
echo "== booted"
adb shell getprop ro.build.fingerprint
echo "== google packages"
adb shell pm list packages | grep -i -E "google|speech|voice|tts" | sort
echo "== recognition services"
adb shell cmd package query-services --brief -a android.speech.RecognitionService
echo "== secure settings"
adb shell settings get secure voice_recognition_service
adb shell settings get secure assistant
echo "== googlequicksearchbox"
adb shell dumpsys package com.google.android.googlequicksearchbox | grep -i -E "versionName|enabled" | head -5
echo "== audio input devices"
adb shell dumpsys media.audio_flinger | grep -i -E "input|source" | head -20
adb shell dumpsys audio | grep -i -E "input|mic" | head -20
echo "== emulator audio log"
grep -i -E "audio|pulse|mic" emulator.log | head -20
echo "== net"
adb shell ping -c1 -W3 8.8.8.8 | tail -2
