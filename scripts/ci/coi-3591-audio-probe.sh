#!/usr/bin/env bash
# Throwaway COI-3591 probe: inject audio into a CI emulator's virtual mic over gRPC.
set -uo pipefail

echo 'KERNEL=="kvm", GROUP="kvm", MODE="0666", OPTIONS+="static_node=kvm"' | sudo tee /etc/udev/rules.d/99-kvm4all.rules >/dev/null
sudo udevadm control --reload-rules
sudo udevadm trigger --name-match=kvm
sudo udevadm settle || true
sudo apt-get update -qq
sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends libpulse0 libgl1 sox espeak-ng
export ANDROID_HOME=/usr/local/lib/android/sdk
export ANDROID_AVD_HOME="$HOME/.android/avd"
mkdir -p "$ANDROID_AVD_HOME"
export PATH="$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$HOME/go/bin:/usr/local/go/bin:$PATH"
IMG="system-images;android-34;google_apis;x86_64"
yes | sdkmanager --licenses >/dev/null || true
sdkmanager "emulator" "$IMG"
echo no | avdmanager create avd --force --name ci --package "$IMG" --device pixel_6
espeak-ng -s 130 -w /tmp/speech.wav "hello world this is a test"
sox /tmp/speech.wav -r 16000 -c 1 -b 16 /tmp/speech16.wav
go version
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@v1.9.1 2>&1 | tail -2
ls "$HOME/go/bin"

nohup emulator -avd ci -no-window -no-boot-anim -no-snapshot -gpu swiftshader_indirect -accel on -grpc 8554 >emulator.log 2>&1 &
pid=$!
waited=0
until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
  kill -0 "$pid" 2>/dev/null || { echo "emulator exited"; tail -15 emulator.log; exit 1; }
  [ "$waited" -lt 240 ] || { echo "no boot"; exit 1; }
  sleep 3
  waited=$((waited + 3))
done
echo "booted after ${waited}s"
grep -n -i -E "audio|grpc" emulator.log | head
echo "== reflection"
timeout 20 grpcurl -plaintext 127.0.0.1:8554 list 2>&1 | head -20
timeout 20 grpcurl -plaintext 127.0.0.1:8554 describe android.emulation.control.AudioPacket 2>&1 | head -20
timeout 20 grpcurl -plaintext 127.0.0.1:8554 describe android.emulation.control.AudioFormat 2>&1 | head -30
timeout 20 grpcurl -plaintext 127.0.0.1:8554 describe android.emulation.control.EmulatorController 2>&1 | grep -i audio

python3 - <<'PY'
import base64, json, wave
w = wave.open("/tmp/speech16.wav")
frames = w.readframes(w.getnframes())
step = 3200
with open("/tmp/inject.jsonl", "w") as out:
    for i in range(0, len(frames), step):
        msg = {"audio": base64.b64encode(frames[i:i + step]).decode()}
        if i == 0:
            msg["format"] = {"samplingRate": 16000, "channels": "Mono", "format": "AUD_FMT_S16", "mode": "MODE_REAL_TIME"}
        out.write(json.dumps(msg) + "\n")
print("chunks", (len(frames) + step - 1) // step)
PY
adb root >/dev/null 2>&1
sleep 3
adb shell setenforce 0
( sleep 2; timeout 25 grpcurl -plaintext -d @ 127.0.0.1:8554 android.emulation.control.EmulatorController/injectAudio < /tmp/inject.jsonl 2>&1 | tail -5 ) &
adb shell 'tinycap /data/local/tmp/cap.wav -D 0 -d 0 -r 16000 -c 1 -b 16 -T 8' 2>&1 | tail -3
sleep 3
adb pull /data/local/tmp/cap.wav /tmp/cap.wav 2>&1 | tail -1
sox /tmp/cap.wav -n stat 2>&1 | grep -E "Maximum amplitude|Length|RMS"
adb emu kill || kill "$pid"
