ARG BASE_IMAGE
FROM ${BASE_IMAGE} AS dev-base-lang-rust
ARG TRUNK_VERSION=0.21.14

# Rust toolchain in the self-contained /usr/local/cargo + /usr/local/rustup
# prefixes core's ENV already points at. full inherits this complete tier.
RUN set -eux; \
    curl -fsSL https://sh.rustup.rs -o /tmp/rustup-init.sh; \
    chmod 0755 /tmp/rustup-init.sh; \
    /tmp/rustup-init.sh -y --no-modify-path --profile minimal --default-toolchain stable; \
    rm /tmp/rustup-init.sh; \
    rustup target add wasm32-unknown-unknown; \
    rustup --version; \
    cargo --version; \
    rustc --version

# trunk (static-site Wasm packager) lands in cargo/bin so the graft carries
# it. Release triple spelling matches KDL_ARCH (x86_64/aarch64).
RUN set -eux; \
    . /tmp/arch.env; \
    curl -fsSL "https://github.com/trunk-rs/trunk/releases/download/v${TRUNK_VERSION}/trunk-${KDL_ARCH}-unknown-linux-gnu.tar.gz" \
      -o /tmp/trunk.tar.gz; \
    tar -xzf /tmp/trunk.tar.gz -C /usr/local/cargo/bin trunk; \
    chmod 0755 /usr/local/cargo/bin/trunk; \
    rm /tmp/trunk.tar.gz; \
    trunk --version; \
    chmod -R a+rwX /usr/local/cargo /usr/local/rustup

# Bevy-class native build libs (alsa/udev/wayland/xkbcommon via pkg-config)
# belong with the Rust consumer contract. Validate the headers and linker in
# the image so the published specialist can build native Winit workspaces.
RUN set -eux; \
    apt-get update; \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      libasound2-dev \
      libudev-dev \
      libwayland-dev \
      libxkbcommon-dev \
      pkg-config; \
    printf '%s\n' \
      '#include <wayland-client.h>' \
      '#include <xkbcommon/xkbcommon.h>' \
      'int main(void) { return 0; }' \
      > /tmp/bevy-native-smoke.c; \
    cc /tmp/bevy-native-smoke.c \
      $(pkg-config --cflags --libs wayland-client xkbcommon) \
      -o /tmp/bevy-native-smoke; \
    /tmp/bevy-native-smoke; \
    rm /tmp/bevy-native-smoke /tmp/bevy-native-smoke.c; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
CMD ["bash"]
