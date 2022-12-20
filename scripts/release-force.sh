#!/usr/bin/env bash

set -eu
set -o pipefail

echo -n "Build timestamp: "
TIMESTAMP=$(date +%s)
echo $TIMESTAMP

echo -n "Git commit: "

COMMIT=$(git rev-parse HEAD)
echo $COMMIT
RELEASE=true
VERSION=v1.69.1-unvetted-1

echo Running "go $@"
exec go "$1" -ldflags \
  "-X storj.io/private/version.buildTimestamp=$TIMESTAMP
   -X storj.io/private/version.buildCommitHash=$COMMIT
   -X storj.io/private/version.buildVersion=$VERSION
   -X storj.io/private/version.buildRelease=$RELEASE" "${@:2}"

