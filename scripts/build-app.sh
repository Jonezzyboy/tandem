#!/usr/bin/env sh
# Builds desktop/build/bin/Tandem.app as a universal bundle with a universal
# td inside it (Contents/MacOS/td), which the Homebrew cask links onto PATH.
# Release CI runs this, so a local run produces the same bundle.
set -e

VERSION="${1:-dev}"
cd "$(dirname "$0")/.."
ldflags="-s -w -X github.com/jonezzyboy/tandem/internal/cli.Version=$VERSION"

(cd desktop && wails build -clean -platform darwin/universal -trimpath)

app=desktop/build/bin/Tandem.app
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for arch in arm64 amd64; do
  GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$tmp/td-$arch" ./cmd/td
done
lipo -create -output "$app/Contents/MacOS/td" "$tmp/td-arm64" "$tmp/td-amd64"

# Adding td breaks the seal wails build applied; re-sign the whole bundle.
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict "$app"
echo "Built $app with td $VERSION"
