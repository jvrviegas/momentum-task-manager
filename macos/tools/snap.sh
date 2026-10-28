#!/bin/sh
# Builds the Debug app and walks it through every page in light and dark, on a
# fake server, saving one PNG of the app's own window per page and look.
#
#   tools/snap.sh                      # every page, into /tmp/Momentum-shots
#   MOMENTUM_SNAPSHOT_PAGES=today,inbox tools/snap.sh
#   SNAP_OUT=/tmp/shots tools/snap.sh
#
# The run is isolated: it uses its own bundle identifier (so its preferences
# and saved window state are not the real app's), a fake bridge (so no server,
# no config.toml and no child process), and an in-memory sign-in store (so no
# Keychain). Needs Screen Recording permission for the terminal that runs it.
set -eu

cd "$(dirname "$0")/.."
out="${SNAP_OUT:-/tmp/Momentum-shots}"
derived="${SNAP_DERIVED:-$HOME/Library/Developer/Momentum-snap}"
bundle=dev.jvrviegas.momentum.snapshot

xcodegen generate --quiet
xcodebuild -scheme Momentum -configuration Debug -destination 'platform=macOS' \
	-derivedDataPath "$derived" PRODUCT_BUNDLE_IDENTIFIER="$bundle" build -quiet

app="$derived/Build/Products/Debug/Momentum Task Manager.app"
binary="$app/Contents/MacOS/Momentum Task Manager"
[ -x "$binary" ] || { echo "error: no app at $app" >&2; exit 1; }

# A crashed run leaves a "reopen windows?" alert that hangs the next one, and
# the window's saved frame would change the size of every image.
rm -rf "$HOME/Library/Saved Application State/$bundle.savedState"
defaults delete "$bundle" >/dev/null 2>&1 || true

rm -rf "$out"
mkdir -p "$out"
caffeinate -u -t 2 || true
MOMENTUM_SNAPSHOT="$out" "$binary"
