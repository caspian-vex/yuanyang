#!/usr/bin/env bash
# Standalone installer; supports release packages and source builds.
set -euo pipefail
umask 077
repo=${YUANYANG_REPO:-caspian-vex/yuanyang}
version=${YUANYANG_VERSION:-latest}
mode=auto
ref=main
ref_set=0
version_set=0
usage(){
  echo 'Usage: bash deploy.sh [--auto | --source [--ref main|vTAG|COMMIT] | --release [--version latest|vTAG]]'
  echo 'Default: release first, source fallback only when latest release returns HTTP 404.'
  echo 'Legacy: bash deploy.sh vTAG'
  echo 'Environment: YUANYANG_REPO=owner/repo INSTALL_DEPS=1 (default); use INSTALL_DEPS=0 to skip apt.'
}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --source) mode=source; shift ;;
    --auto) mode=auto; shift ;;
    --release) mode=release; shift ;;
    --ref) [[ $# -ge 2 ]] || { usage; exit 1; }; ref=$2; ref_set=1; shift 2 ;;
    --version) [[ $# -ge 2 ]] || { usage; exit 1; }; version=$2; version_set=1; shift 2 ;;
    latest|v[0-9]*) version=$1; version_set=1; shift ;;
    *) echo "Unknown argument: $1"; usage; exit 1 ;;
  esac
done
[[ $repo =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo 'Invalid repository'; exit 1; }
[[ $version == latest || $version =~ ^v[0-9][A-Za-z0-9._-]*$ ]] || { echo 'Invalid release version'; exit 1; }
[[ $ref =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ && $ref != *..* ]] || { echo 'Invalid source ref'; exit 1; }
[[ $mode == source || $ref_set == 0 ]] || { echo '--ref requires --source'; exit 1; }
[[ $mode != source || $version_set == 0 ]] || { echo 'Use --ref to choose a source version'; exit 1; }
[[ $(id -u) -eq 0 ]] || { echo 'Run as root or with sudo bash'; exit 1; }
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo 'Linux x86_64 required'; exit 1; }
[[ -f /etc/os-release ]] || exit 1
. /etc/os-release
case "${ID}:${VERSION_ID}" in ubuntu:22.04|ubuntu:24.04|debian:12|debian:13) ;; *) echo 'Supported: Ubuntu 22.04/24.04, Debian 12/13'; exit 1;; esac
[[ $(stat -fc %T /sys/fs/cgroup) == cgroup2fs ]] || { echo 'cgroup v2 required'; exit 1; }
export INSTALL_DEPS=${INSTALL_DEPS:-1}
[[ $INSTALL_DEPS == 0 || $INSTALL_DEPS == 1 ]] || { echo 'INSTALL_DEPS must be 0 or 1'; exit 1; }
exec 9>/run/lock/yuanyang-deploy.lock
flock -n 9 || { echo 'Another deployment is running'; exit 1; }
if [[ $INSTALL_DEPS == 1 ]]; then
  apt-get update
  packages=(curl ca-certificates tar)
  if [[ $mode == source ]]; then packages+=(git jq xz-utils); fi
  DEBIAN_FRONTEND=noninteractive apt-get install -y "${packages[@]}"
fi
for tool in curl tar sha256sum; do command -v "$tool" >/dev/null || { echo "Missing $tool"; exit 1; }; done
work=$(mktemp -d /tmp/yuanyang-deploy.XXXXXXXX)
trap 'rm -rf -- "$work"' EXIT
fetch(){ curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 900 --retry 3 "$1" -o "$2"; }
verify(){
  local expected=$1 actual
  [[ $expected =~ ^[a-fA-F0-9]{64}$ ]] || { echo 'Invalid or missing checksum'; exit 1; }
  actual=$(sha256sum "$2")
  [[ ${actual%% *} == "$expected" ]] || { echo "Checksum mismatch: $2"; exit 1; }
}
if [[ $mode == auto ]]; then
  base="https://github.com/$repo/releases"
  if [[ $version == latest ]]; then base="$base/latest/download"; else base="$base/download/$version"; fi
  status=0
  http_code=$(curl --location --silent --show-error --fail --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 120 --retry 2 --write-out '%{http_code}' "$base/SHA256SUMS" -o "$work/SHA256SUMS") || status=$?
  if [[ $status == 0 ]]; then
    mode=release
  elif [[ $status == 22 && $http_code == 404 && $version == latest ]]; then
    echo 'No latest release is available (HTTP 404); switching to source build on main.'
    echo 'Compilation needs more memory than the panel runtime; no swap is configured automatically.'
    mode=source
    if [[ $INSTALL_DEPS == 1 ]]; then
      DEBIAN_FRONTEND=noninteractive apt-get install -y git jq xz-utils
    fi
  else
    echo "Release lookup failed (curl=$status HTTP=$http_code); not falling back to a different version."
    exit 1
  fi
fi
if [[ $mode == source ]]; then
  for tool in git jq xz; do command -v "$tool" >/dev/null || { echo "Missing $tool; enable INSTALL_DEPS=1"; exit 1; }; done
  echo "Building $repo at $ref. Compilation needs more memory than the 100 MB runtime budget."
  mkdir "$work/package"
  git -C "$work/package" init -q
  git -C "$work/package" remote add origin "https://github.com/$repo.git"
  GIT_TERMINAL_PROMPT=0 git -C "$work/package" fetch --depth 1 origin "$ref"
  git -C "$work/package" checkout -q --detach FETCH_HEAD
  git -C "$work/package" rev-parse HEAD
  # Install verified toolchains into this temporary directory, never replace system tools.
  goversion=$(awk '$1 == "go" {print $2; exit}' "$work/package/go.mod")
  [[ $goversion =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid Go version in go.mod'; exit 1; }
  fetch 'https://go.dev/dl/?mode=json&include=all' "$work/go.json"
  gofile="go${goversion}.linux-amd64.tar.gz"
  gosum=$(jq -r --arg v "go$goversion" --arg f "$gofile" '.[] | select(.version == $v) | .files[] | select(.filename == $f) | .sha256' "$work/go.json")
  fetch "https://go.dev/dl/$gofile" "$work/go.tar.gz"
  verify "$gosum" "$work/go.tar.gz"
  fetch 'https://nodejs.org/dist/latest-v22.x/SHASUMS256.txt' "$work/node-sums"
  nodefile=$(awk '$2 ~ /^node-v22\.[0-9]+\.[0-9]+-linux-x64\.tar\.xz$/ {print $2}' "$work/node-sums")
  [[ $nodefile =~ ^node-v22\.[0-9]+\.[0-9]+-linux-x64\.tar\.xz$ ]] || { echo 'Cannot resolve Node.js 22 archive'; exit 1; }
  nodesum=$(awk -v f="$nodefile" '$2 == f {print $1}' "$work/node-sums")
  fetch "https://nodejs.org/dist/latest-v22.x/$nodefile" "$work/node.tar.xz"
  verify "$nodesum" "$work/node.tar.xz"
  mkdir "$work/tools" "$work/tools/node"
  tar -xzf "$work/go.tar.gz" -C "$work/tools" --no-same-owner
  tar -xJf "$work/node.tar.xz" -C "$work/tools/node" --strip-components=1 --no-same-owner
  export PATH="$work/tools/go/bin:$work/tools/node/bin:$PATH"
  export GOMAXPROCS=1 GOFLAGS=-p=1 GOCACHE="$work/go-cache" GOMODCACHE="$work/go-modules" npm_config_cache="$work/npm-cache"
  bash "$work/package/scripts/build.sh"
else
  asset=yuanyang-linux-amd64.tar.gz
  base="https://github.com/$repo/releases"
  if [[ $version == latest ]]; then base="$base/latest/download"; else base="$base/download/$version"; fi
  echo "Downloading $repo ($version)..."
  fetch "$base/SHA256SUMS" "$work/SHA256SUMS" || { echo 'Release unavailable. Wait for the publish job to finish, or run without --release to enable automatic source fallback.'; exit 1; }
  fetch "$base/$asset" "$work/$asset"
  expected=$(awk -v asset="$asset" '$2 == asset {print $1}' "$work/SHA256SUMS")
  verify "$expected" "$work/$asset"
  tar -tzf "$work/$asset" > "$work/members"
  while IFS= read -r entry; do
    [[ $entry != /* && /$entry/ != *'/../'* ]] || { echo 'Unsafe archive path'; exit 1; }
  done < "$work/members"
  tar -tvzf "$work/$asset" > "$work/listing"
  if awk 'substr($0,1,1) != "-" && substr($0,1,1) != "d" {bad=1} END {exit !bad}' "$work/listing"; then
    echo 'Archive contains links or special files'; exit 1
  fi
  mkdir "$work/package"
  tar -xzf "$work/$asset" -C "$work/package" --no-same-owner --no-same-permissions
fi
cd "$work/package"
if [[ -f /etc/systemd/system/yuanyang.service ]]; then
  bash scripts/upgrade.sh
else
  bash scripts/install.sh
fi
echo 'Deployment complete. Inspect: systemctl status yuanyang'
