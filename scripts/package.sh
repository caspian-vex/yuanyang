#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f bin/yuanyang && -f web/dist/index.html ]] || { echo 'Run scripts/build.sh first'; exit 1; }
mkdir -p dist
tar -czf dist/yuanyang-linux-amd64.tar.gz bin/yuanyang web/dist deploy scripts/install.sh scripts/upgrade.sh scripts/uninstall.sh scripts/measure.sh README.md docs
(cd dist && sha256sum yuanyang-linux-amd64.tar.gz > SHA256SUMS)
