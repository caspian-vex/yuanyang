#!/usr/bin/env bash
# Standalone deployment from GitHub releases; no compiler needed on the server.
set -euo pipefail
umask 077
repo=${YUANYANG_REPO:-caspian-vex/yuanyang}
version=${YUANYANG_VERSION:-latest}
if [[ ${1:-} == --help ]]; then
  echo 'sudo bash scripts/deploy.sh [latest|vVERSION]'
  echo 'Environment: YUANYANG_REPO=owner/repo INSTALL_DEPS=1 (default); INSTALL_DEPS=0 skips apt installation.'
  exit 0
fi
[[ $# -le 1 ]] || { echo 'Expected zero or one version argument'; exit 1; }
version=${1:-$version}
[[ $repo =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo 'Invalid GitHub repository'; exit 1; }
[[ $version == latest || $version =~ ^v[0-9][A-Za-z0-9._-]*$ ]] || { echo 'Expected latest or a version tag such as v0.1.0'; exit 1; }
[[ $(id -u) -eq 0 ]] || { echo 'Run as root'; exit 1; }
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo 'Linux x86_64 required'; exit 1; }
[[ -f /etc/os-release ]] || exit 1
. /etc/os-release
case "${ID}:${VERSION_ID}" in ubuntu:22.04|ubuntu:24.04|debian:12|debian:13) ;; *) echo 'Supported: Ubuntu 22.04/24.04, Debian 12/13'; exit 1;; esac
export INSTALL_DEPS=${INSTALL_DEPS:-1}
[[ $INSTALL_DEPS == 0 || $INSTALL_DEPS == 1 ]] || { echo 'INSTALL_DEPS must be 0 or 1'; exit 1; }
if [[ $INSTALL_DEPS == 1 ]]; then
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y curl ca-certificates tar
fi
for tool in curl tar sha256sum flock; do command -v "$tool" >/dev/null || { echo "Missing $tool"; exit 1; }; done
exec 9>/run/lock/yuanyang-deploy.lock
flock -n 9 || { echo 'Another deployment is running'; exit 1; }
work=$(mktemp -d /tmp/yuanyang-deploy.XXXXXXXX)
trap 'rm -rf -- "$work"' EXIT
asset=yuanyang-linux-amd64.tar.gz
base="https://github.com/$repo/releases"
if [[ $version == latest ]]; then base="$base/latest/download"; else base="$base/download/$version"; fi
fetch(){ curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 600 --retry 3 "$1" -o "$2"; }
echo "Downloading $repo ($version)..."
fetch "$base/SHA256SUMS" "$work/SHA256SUMS" || { echo 'Release unavailable. Publish a v* tag and wait for the GitHub release workflow first.'; exit 1; }
fetch "$base/$asset" "$work/$asset"
expected=$(awk -v asset="$asset" '$2 == asset {print $1}' "$work/SHA256SUMS")
[[ $expected =~ ^[a-fA-F0-9]{64}$ ]] || { echo 'Invalid or missing checksum'; exit 1; }
actual=$(sha256sum "$work/$asset")
[[ ${actual%% *} == "$expected" ]] || { echo 'Checksum mismatch; installation cancelled'; exit 1; }
tar -tzf "$work/$asset" > "$work/members"
while IFS= read -r entry; do
  [[ $entry != /* && /$entry/ != *'/../'* ]] || { echo 'Unsafe archive path'; exit 1; }
done < "$work/members"
if tar -tvzf "$work/$asset" | awk 'substr($0,1,1) != "-" && substr($0,1,1) != "d" {bad=1} END {exit !bad}'; then
  echo 'Archive contains links or special files'; exit 1
fi
mkdir "$work/package"
tar -xzf "$work/$asset" -C "$work/package" --no-same-owner --no-same-permissions
cd "$work/package"
if [[ -f /etc/systemd/system/yuanyang.service ]]; then
  bash scripts/upgrade.sh
else
  bash scripts/install.sh
fi
echo 'Deployment complete. Inspect: systemctl status yuanyang'
