#!/bin/sh
# Render Homebrew and Scoop metadata from the version-stamped native AOS binaries.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
dist=${AOS_RELEASE_DIST:-"$repo_root/dist"}
version=${AOS_RELEASE_VERSION:-$(
    git -C "$repo_root" describe --tags --exact-match --match 'aos-v*'
)}
bare=${version#aos-v}
# GitHub, because Forgejo is tailnet-only and a brew or scoop install runs anywhere.
base="https://github.com/coilyco/agentic-os/releases/download/${version}"

if ! printf '%s\n' "$version" | grep -Eq '^aos-v[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "aos release version must match aos-vMAJOR.MINOR.PATCH: $version" >&2
    exit 1
fi

sha() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum < "$1" | cut -d' ' -f1
    else
        shasum -a 256 < "$1" | cut -d' ' -f1
    fi
}

darwin_arm64=$(sha "$dist/aos-darwin-arm64")
linux_amd64=$(sha "$dist/aos-linux-amd64")
linux_arm64=$(sha "$dist/aos-linux-arm64")
windows_amd64=$(sha "$dist/aos-windows-amd64.exe")
aoscompose_darwin_arm64=$(sha "$dist/aoscompose-darwin-arm64")
aoscompose_linux_amd64=$(sha "$dist/aoscompose-linux-amd64")
aoscompose_linux_arm64=$(sha "$dist/aoscompose-linux-arm64")
aoscompose_windows_amd64=$(sha "$dist/aoscompose-windows-amd64.exe")
aosward_darwin_arm64=$(sha "$dist/aosward-darwin-arm64")
aosward_linux_amd64=$(sha "$dist/aosward-linux-amd64")
aosward_linux_arm64=$(sha "$dist/aosward-linux-arm64")
aosward_windows_amd64=$(sha "$dist/aosward-windows-amd64.exe")
aosguard_darwin_arm64=$(sha "$dist/aosguard-darwin-arm64")
aosguard_linux_amd64=$(sha "$dist/aosguard-linux-amd64")
aosguard_linux_arm64=$(sha "$dist/aosguard-linux-arm64")
aosguard_windows_amd64=$(sha "$dist/aosguard-windows-amd64.exe")
aterm_darwin_arm64=$(sha "$dist/aterm-darwin-arm64")
aterm_linux_amd64=$(sha "$dist/aterm-linux-amd64")
aterm_linux_arm64=$(sha "$dist/aterm-linux-arm64")

cat > "$dist/aos.rb" <<EOF
class Aos < Formula
  desc "Agent runtime composition root for Agentic OS"
  homepage "https://github.com/coilyco/agentic-os"
  version "${bare}"
  license "MIT"

  on_macos do
    on_arm do
      url "${base}/aos-darwin-arm64"
      sha256 "${darwin_arm64}"
      resource "aoscompose" do
        url "${base}/aoscompose-darwin-arm64"
        sha256 "${aoscompose_darwin_arm64}"
      end
      resource "aosward" do
        url "${base}/aosward-darwin-arm64"
        sha256 "${aosward_darwin_arm64}"
      end
      resource "aosguard" do
        url "${base}/aosguard-darwin-arm64"
        sha256 "${aosguard_darwin_arm64}"
      end
      resource "aterm" do
        url "${base}/aterm-darwin-arm64"
        sha256 "${aterm_darwin_arm64}"
      end
    end
  end
  on_linux do
    on_intel do
      url "${base}/aos-linux-amd64"
      sha256 "${linux_amd64}"
      resource "aoscompose" do
        url "${base}/aoscompose-linux-amd64"
        sha256 "${aoscompose_linux_amd64}"
      end
      resource "aosward" do
        url "${base}/aosward-linux-amd64"
        sha256 "${aosward_linux_amd64}"
      end
      resource "aosguard" do
        url "${base}/aosguard-linux-amd64"
        sha256 "${aosguard_linux_amd64}"
      end
      resource "aterm" do
        url "${base}/aterm-linux-amd64"
        sha256 "${aterm_linux_amd64}"
      end
    end
    on_arm do
      url "${base}/aos-linux-arm64"
      sha256 "${linux_arm64}"
      resource "aoscompose" do
        url "${base}/aoscompose-linux-arm64"
        sha256 "${aoscompose_linux_arm64}"
      end
      resource "aosward" do
        url "${base}/aosward-linux-arm64"
        sha256 "${aosward_linux_arm64}"
      end
      resource "aosguard" do
        url "${base}/aosguard-linux-arm64"
        sha256 "${aosguard_linux_arm64}"
      end
      resource "aterm" do
        url "${base}/aterm-linux-arm64"
        sha256 "${aterm_linux_arm64}"
      end
    end
  end

  def install
    bin.install Dir["aos-*"].first => "aos"
    resource("aoscompose").stage { bin.install Dir["aoscompose-*"].first => "aoscompose" }
    bin.install_symlink bin/"aoscompose" => "aoscomposed"
    resource("aosward").stage { bin.install Dir["aosward-*"].first => "aosward" }
    resource("aosguard").stage { bin.install Dir["aosguard-*"].first => "aosguard" }
    resource("aterm").stage { bin.install Dir["aterm-*"].first => "aterm" }
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/aos version")
    assert_match version.to_s, shell_output("#{bin}/aoscompose version")
    assert_match version.to_s, shell_output("#{bin}/aoscomposed version")
    assert_match version.to_s, shell_output("#{bin}/aosward version")
    assert_match version.to_s, shell_output("#{bin}/aosguard --version")
    assert_match version.to_s, shell_output("#{bin}/aterm --version")
  end
end
EOF

cat > "$dist/aos.json" <<EOF
{
    "version": "${bare}",
    "description": "Agent runtime composition root for Agentic OS",
    "homepage": "https://github.com/coilyco/agentic-os",
    "license": "MIT",
    "architecture": {
        "64bit": {
            "url": "${base}/aos-windows-amd64.exe",
            "hash": "${windows_amd64}",
            "bin": [
                ["aos-windows-amd64.exe", "aos"],
                ["aoscompose-windows-amd64.exe", "aoscompose"],
                ["aoscompose-windows-amd64.exe", "aoscomposed"],
                ["aosward-windows-amd64.exe", "aosward"],
                ["aosguard-windows-amd64.exe", "aosguard"]
            ]
        }
    },
    "pre_install": [
        "Invoke-WebRequest -Uri '${base}/aoscompose-windows-amd64.exe' -OutFile \"\$dir/aoscompose-windows-amd64.exe\"; if ((Get-FileHash \"\$dir/aoscompose-windows-amd64.exe\" -Algorithm SHA256).Hash -ne '${aoscompose_windows_amd64}') { throw 'aoscompose checksum mismatch' }",
        "Invoke-WebRequest -Uri '${base}/aosward-windows-amd64.exe' -OutFile \"\$dir/aosward-windows-amd64.exe\"; if ((Get-FileHash \"\$dir/aosward-windows-amd64.exe\" -Algorithm SHA256).Hash -ne '${aosward_windows_amd64}') { throw 'aosward checksum mismatch' }",
        "Invoke-WebRequest -Uri '${base}/aosguard-windows-amd64.exe' -OutFile \"\$dir/aosguard-windows-amd64.exe\"; if ((Get-FileHash \"\$dir/aosguard-windows-amd64.exe\" -Algorithm SHA256).Hash -ne '${aosguard_windows_amd64}') { throw 'aosguard checksum mismatch' }"
    ]
}
EOF

echo "$dist/aos.rb"
echo "$dist/aos.json"
