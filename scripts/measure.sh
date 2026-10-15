#!/usr/bin/env bash
set -euo pipefail
duration="${1:-3600}"
[[ $duration =~ ^[0-9]+$ ]] || exit 1
[[ $(stat -fc %T /sys/fs/cgroup) == cgroup2fs ]] || { echo 'cgroup v2 required'; exit 1; }
group="$(systemctl show yuanyang.slice -p ControlGroup --value)"
[[ $group == /yuanyang.slice ]] || { echo 'Unexpected panel cgroup'; exit 1; }
base="/sys/fs/cgroup${group}"
echo 'timestamp,memory_current,memory_peak,cpu_usage_usec,oom_kill'
end=$((SECONDS + duration))
while ((SECONDS < end)); do
  current=$(cat "$base/memory.current")
  peak=$(cat "$base/memory.peak" 2>/dev/null || printf 'unavailable')
  cpu=$(awk '$1=="usage_usec" {print $2}' "$base/cpu.stat")
  oom=$(awk '$1=="oom_kill" {print $2}' "$base/memory.events")
  printf '%s,%s,%s,%s,%s\n' "$(date +%s)" "$current" "$peak" "$cpu" "$oom"
  sleep 10
done
