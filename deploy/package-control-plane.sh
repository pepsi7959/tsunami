#!/usr/bin/env bash
# Build the control-plane package on your workstation (cross-compiles the ocean
# binary for linux/amd64, bundles the web UI + installer). Produces
# tsunami-control-plane.tgz to scp to the droplet. Re-run for each update.
set -euo pipefail
cd "$(dirname "$0")/.."

rm -rf dist && mkdir -p dist
echo ">> cross-compiling ocean (linux/amd64)"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/ocean ./master
cp -r webcontrol dist/webcontrol
cp deploy/install-native.sh dist/install-native.sh
chmod +x dist/install-native.sh

tar czf tsunami-control-plane.tgz -C dist .
echo ">> built tsunami-control-plane.tgz ($(du -h tsunami-control-plane.tgz | cut -f1))"
echo "   scp it to the droplet and run install-native.sh (see below)."
