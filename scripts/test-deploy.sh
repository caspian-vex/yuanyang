#!/usr/bin/env bash
# Offline regression checks for release selection; never install or download anything.
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
awk '/^if \[\[ \$mode == auto/ {copy=1} /^if \[\[ \$mode == source/ {exit} copy {print}' scripts/deploy.sh > "$work/select.sh"
[[ -s $work/select.sh ]]
cat > "$work/run.sh" <<'RUNNER'
set -euo pipefail
mode=auto
repo=example/panel
version=${TEST_VERSION:-latest}
INSTALL_DEPS=0
work=$TEST_WORK
curl(){ printf '%s' "$TEST_HTTP"; return "$TEST_STATUS"; }
source "$TEST_WORK/select.sh"
echo "RESULT=$mode"
RUNNER
check(){
  local status=$1 http=$2 version=$3 expected=$4 output rc=0
  output=$(TEST_WORK="$work" TEST_STATUS="$status" TEST_HTTP="$http" TEST_VERSION="$version" bash "$work/run.sh") || rc=$?
  if [[ $expected == error ]]; then
    [[ $rc != 0 ]] || { echo "Expected failure: $status/$http/$version"; exit 1; }
  else
    [[ $rc == 0 && $output == *"RESULT=$expected"* ]] || { echo "Unexpected result: $output"; exit 1; }
  fi
}
check 0 200 latest release
check 22 404 latest source
check 22 404 v0.1.1 error
check 22 403 latest error
check 22 500 latest error
check 6 000 latest error
check 28 000 latest error
echo 'Deployment selection: 7 offline checks passed'
