#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

VERSION=$(git describe --tags --dirty --always)

if [ "$#" -gt 0 ]; then
    VERSION=${VERSION%%\.*}
    VERSION="$VERSION-$1"
fi

echo "$VERSION"