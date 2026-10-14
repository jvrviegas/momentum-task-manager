#!/bin/sh
# Builds cmd/momentum-bridge for every architecture Xcode is building and
# places it next to the app executable, where ProcessBridge launches it.
set -eu

export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/local/go/bin:$PATH"
repo="${SRCROOT}/.."
out="${TARGET_BUILD_DIR}/${EXECUTABLE_FOLDER_PATH}/momentum-bridge"
work="${DERIVED_FILE_DIR}/momentum-bridge"
version=$(git -C "$repo" describe --tags --always --dirty 2>/dev/null || echo dev)

mkdir -p "$work" "$(dirname "$out")"
set --
for arch in ${ARCHS}; do
	case "$arch" in
	arm64) goarch=arm64 ;;
	x86_64) goarch=amd64 ;;
	*) echo "error: unsupported architecture $arch" >&2; exit 1 ;;
	esac
	(cd "$repo" && CGO_ENABLED=0 GOOS=darwin GOARCH="$goarch" go build -trimpath \
		-ldflags "-s -w -X main.Version=$version" \
		-o "$work/momentum-bridge-$arch" ./cmd/momentum-bridge)
	set -- "$@" "$work/momentum-bridge-$arch"
done
lipo -create "$@" -output "$out"
codesign --force --sign "${EXPANDED_CODE_SIGN_IDENTITY:--}" --timestamp=none "$out"
