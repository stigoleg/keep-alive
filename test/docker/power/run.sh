#!/bin/sh
# Builds the image and runs the Linux D-Bus inhibitor test against
# python-dbusmock. Needs a running Docker daemon.
set -eu
root=$(cd "$(dirname "$0")/../../.." && pwd)
docker build -f "$root/test/docker/power/Dockerfile" -t keepalive-power-test "$root"
exec docker run --rm keepalive-power-test "$@"
