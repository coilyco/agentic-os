#!/usr/bin/env bash
# Publish guard: a release APK needs the real signing key or it cannot update in place.
set -euo pipefail
if [ -z "${ATERM_ANDROID_KEYSTORE_B64:-}" ] || [ -z "${ATERM_ANDROID_KEYSTORE_PASSWORD:-}" ]; then
  echo "::error::The ATERM_ANDROID_KEYSTORE_B64 and ATERM_ANDROID_KEYSTORE_PASSWORD repo secrets are unset, so the build would sign with a throwaway key. Create them from SSM, or dispatch with allow_throwaway for a one-off install."
  exit 1
fi
