#!/usr/bin/env sh
# Sets the app version in every manifest that carries one. The release
# workflow refuses to build when these disagree with the tag.
set -e

VERSION="$1"
if [ -z "$VERSION" ]; then
  echo "usage: scripts/set-version.sh <version>   e.g. 0.3.0" >&2
  exit 1
fi
case "$VERSION" in
  v*) echo "Pass the bare version, without the leading 'v'." >&2; exit 1 ;;
esac

cd "$(dirname "$0")/.."

(cd desktop/frontend && npm version "$VERSION" --no-git-tag-version --allow-same-version >/dev/null)

node -e '
  const fs = require("fs");
  const [file, version] = [process.argv[1], process.argv[2]];
  const conf = JSON.parse(fs.readFileSync(file, "utf8"));
  conf.info.productVersion = version;
  fs.writeFileSync(file, JSON.stringify(conf, null, 2) + "\n");
' desktop/wails.json "$VERSION"

echo "Set version $VERSION. Commit, then cut the release: gh release create v$VERSION --generate-notes"
