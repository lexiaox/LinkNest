#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-go}"
FYNE_BIN="${FYNE_BIN:-fyne}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist/gui-artifacts}"
WINDOWS_CC="${WINDOWS_CC:-x86_64-w64-mingw32-gcc-posix}"
WINDOWS_CXX="${WINDOWS_CXX:-x86_64-w64-mingw32-g++-posix}"
ANDROID_SDK="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-$HOME/.local/android-sdk}}"
ANDROID_NDK="${ANDROID_NDK_HOME:-}"

mkdir -p "$OUT_DIR" "$ROOT_DIR/.cache/go-build"

if ! command -v "$GO_BIN" >/dev/null 2>&1; then
  echo "go not found. Set GO_BIN=/path/to/go or add go to PATH." >&2
  exit 1
fi

if ! command -v "$FYNE_BIN" >/dev/null 2>&1; then
  echo "fyne not found. Set FYNE_BIN=/path/to/fyne or install fyne CLI." >&2
  exit 1
fi

GO_PATH="$(command -v "$GO_BIN")"
GO_DIR="$(dirname "$GO_PATH")"

if ! command -v "$WINDOWS_CC" >/dev/null 2>&1; then
  echo "$WINDOWS_CC not found. Install mingw-w64 or set WINDOWS_CC/WINDOWS_CXX." >&2
  exit 1
fi

if [ ! -d "$ANDROID_SDK" ]; then
  echo "Android SDK not found at $ANDROID_SDK. Set ANDROID_HOME or ANDROID_SDK_ROOT." >&2
  exit 1
fi

if [ -z "$ANDROID_NDK" ] && [ -d "$ANDROID_SDK/ndk" ]; then
  ANDROID_NDK="$(find "$ANDROID_SDK/ndk" -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n 1)"
fi

if [ -z "$ANDROID_NDK" ] || [ ! -d "$ANDROID_NDK" ]; then
  echo "Android NDK not found. Set ANDROID_NDK_HOME or install an NDK under $ANDROID_SDK/ndk." >&2
  exit 1
fi

if ! command -v zip >/dev/null 2>&1; then
  echo "zip not found. Install zip (e.g. apt-get install zip)." >&2
  exit 1
fi

if ! command -v sha256sum >/dev/null 2>&1; then
  echo "sha256sum not found. Install coreutils (e.g. apt-get install coreutils)." >&2
  exit 1
fi

echo "Building Windows GUI..."
(
  cd "$ROOT_DIR"
  PATH="$(dirname "$(command -v "$WINDOWS_CC")"):$PATH" \
    GOCACHE="$ROOT_DIR/.cache/go-build" \
    TMPDIR="${TMPDIR:-/tmp}" \
    TEMP="${TEMP:-/tmp}" \
    TMP="${TMP:-/tmp}" \
    GOOS=windows \
    GOARCH=amd64 \
    CGO_ENABLED=1 \
    CC="$WINDOWS_CC" \
    CXX="$WINDOWS_CXX" \
    "$GO_BIN" build -trimpath -ldflags "-s -w -H=windowsgui" \
      -o "$OUT_DIR/linknest-desktop.exe" \
      ./client/desktop/cmd/linknest-desktop
  (
    cd "$OUT_DIR"
    rm -f LinkNest-Windows-GUI.zip
    zip -q LinkNest-Windows-GUI.zip linknest-desktop.exe
  )
)

echo "Building Android APK..."
(
  cd "$ROOT_DIR/client/mobile/cmd/linknest-mobile"
  ANDROID_HOME="$ANDROID_SDK" \
    ANDROID_SDK_ROOT="$ANDROID_SDK" \
    ANDROID_NDK_HOME="$ANDROID_NDK" \
    PATH="$GO_DIR:$PATH" \
    GOCACHE="$ROOT_DIR/.cache/go-build" \
    TMPDIR="${TMPDIR:-/tmp}" \
    "$FYNE_BIN" package -os android \
      -app-id top.ledouya.linknest.mobile \
      -name LinkNestMobile \
      -icon Icon.png
  cp LinkNestMobile.apk "$OUT_DIR/LinkNestMobile.apk"
)

echo "Artifacts:"
ls -lh "$OUT_DIR"/LinkNest-Windows-GUI.zip "$OUT_DIR"/LinkNestMobile.apk
sha256sum "$OUT_DIR"/LinkNest-Windows-GUI.zip "$OUT_DIR"/LinkNestMobile.apk
