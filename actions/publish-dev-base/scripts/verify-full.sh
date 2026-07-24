#!/usr/bin/env bash
# Verify the complete language and operator surface on every published architecture.

set -euo pipefail

image="${IMAGE_BASE}:${TAG}"
for platform in linux/amd64 linux/arm64; do
  echo "verifying ${image} on ${platform}"
  docker run --rm --platform "$platform" --entrypoint bash "$image" \
    -euo pipefail -c '
      dpkg-query -W \
        libasound2-dev \
        libudev-dev \
        libwayland-dev \
        libxkbcommon-dev \
        pkg-config
      cargo --version
      trunk --version
      rustup target list --installed | grep -qx wasm32-unknown-unknown
      node --version
      go version
      dotnet --list-sdks
      python --version
      pipenv --version
      aguard --version
      pkg-config --exists wayland-client xkbcommon
      printf "%s\\n" \
        "#include <wayland-client.h>" \
        "#include <xkbcommon/xkbcommon.h>" \
        "int main(void) { return 0; }" \
        > /tmp/bevy-native-smoke.c
      cc /tmp/bevy-native-smoke.c \
        $(pkg-config --cflags --libs wayland-client xkbcommon) \
        -o /tmp/bevy-native-smoke
      /tmp/bevy-native-smoke
    '
done
