#!/usr/bin/env bash
# Decode APP_KEY_B64 into a multi-line step output named pem. The secret store
# holds the key as one base64 line, and the decoded form is not a registered
# secret, so each line is masked before it reaches an output.

set -euo pipefail

if [ -z "${APP_KEY_B64:-}" ]; then
  echo "::error::the App private key secret is empty or not set" >&2
  exit 1
fi
if ! pem=$(printf '%s' "$APP_KEY_B64" | base64 -d 2>/dev/null) || [ -z "$pem" ]; then
  echo "::error::the App private key secret is not valid base64" >&2
  exit 1
fi
case "$pem" in
  "-----BEGIN "*) ;;
  *)
    echo "::error::the decoded App private key is not a PEM block" >&2
    exit 1
    ;;
esac

while IFS= read -r line; do
  [ -n "$line" ] && echo "::add-mask::$line"
done <<< "$pem"
{
  echo "pem<<APP_KEY_PEM_EOF"
  printf '%s\n' "$pem"
  echo "APP_KEY_PEM_EOF"
} >> "${GITHUB_OUTPUT:?GITHUB_OUTPUT is not set}"
