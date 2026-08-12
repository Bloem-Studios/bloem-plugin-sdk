#!/bin/sh
set -eu
mkdir -p dist
GOWORK=off CGO_ENABLED=0 go build -trimpath -o dist/compat-probe ./cmd/compat-probe
shasum -a 256 dist/compat-probe > dist/compat-probe.sha256
dist/compat-probe manifest > dist/compat-probe.manifest.json
