#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Builds core/libcore into app/libs/libcore.aar with gomobile.
# Requires: Go (see core/go.mod), Android SDK with an NDK installed, JDK 17.
#   ANDROID_HOME      (or ANDROID_SDK_ROOT)
#   ANDROID_NDK_HOME  (optional; otherwise the newest $ANDROID_HOME/ndk/* is used)
# Optional:
#   TARGETS  default "android/arm64,android/arm,android/amd64"
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CORE="$ROOT/core"
OUT="$ROOT/app/libs/libcore.aar"
TARGETS="${TARGETS:-android/arm64,android/arm,android/amd64}"
API=26

export ANDROID_HOME="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
if [[ -z "$ANDROID_HOME" ]]; then echo "ANDROID_HOME is not set" >&2; exit 1; fi
if [[ -z "${ANDROID_NDK_HOME:-}" ]]; then
  ANDROID_NDK_HOME="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1 || true)"
  export ANDROID_NDK_HOME
fi
if [[ -z "${ANDROID_NDK_HOME:-}" || ! -d "$ANDROID_NDK_HOME" ]]; then echo "No Android NDK found" >&2; exit 1; fi
echo "Using NDK: $ANDROID_NDK_HOME"

cd "$CORE"
# gomobile/gobind pinned by core/go.mod (golang.org/x/mobile)
go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
export PATH="$(go env GOPATH)/bin:$PATH"
gomobile init

MIHOMO_VERSION="$(go list -m -f '{{.Version}}' github.com/metacubex/mihomo)"
LDFLAGS="-s -w -buildid= -X github.com/metacubex/mihomo/constant.Version=${MIHOMO_VERSION} -X github.com/metacubex/mihomo/constant.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"

mkdir -p "$(dirname "$OUT")"
gomobile bind -v \
  -target="$TARGETS" \
  -androidapi "$API" \
  -javapkg=io.github.sqmydor.surfmi.core \
  -tags "cmfa,with_gvisor" \
  -trimpath \
  -ldflags "$LDFLAGS" \
  -o "$OUT" \
  ./libcore

echo "Built $OUT (mihomo $MIHOMO_VERSION)"
