#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ $(id -u) -eq 0 ]] || { echo 'Run as root'; exit 1; }
[[ -f bin/yuanyang && -f web/dist/index.html && -f /usr/local/bin/yuanyang ]] || exit 1
backup="/var/backups/yuanyang/$(date +%Y%m%d-%H%M%S)-$$"
install -d -m 0700 "$backup"
systemctl stop yuanyang
trap 'echo "Snapshot preparation failed; original data retained"; systemctl start yuanyang || true' ERR
cp -a /var/lib/yuanyang "$backup/data"
cp -a /usr/local/bin/yuanyang "$backup/yuanyang"
cp -a /opt/yuanyang/web "$backup/web"
for unit in yuanyang.service yuanyang.slice yuanyang-recover.service; do
  if [[ -f /etc/systemd/system/$unit ]]; then cp -a "/etc/systemd/system/$unit" "$backup/$unit"; fi
done
rollback(){
  trap - ERR
  echo "Upgrade failed; restoring data and code. Snapshot: $backup"
  systemctl stop yuanyang || true
  install -m 0755 "$backup/yuanyang" /usr/local/bin/yuanyang.rollback
  mv -f /usr/local/bin/yuanyang.rollback /usr/local/bin/yuanyang
  if [[ -d /var/lib/yuanyang ]]; then mv /var/lib/yuanyang "$backup/failed-data"; fi
  cp -a "$backup/data" /var/lib/yuanyang
  cp -a "$backup/web/." /opt/yuanyang/web/
  for unit in yuanyang.service yuanyang.slice yuanyang-recover.service; do
    if [[ -f "$backup/$unit" ]]; then cp -a "$backup/$unit" "/etc/systemd/system/$unit"; fi
  done
  systemctl daemon-reload
  systemctl start yuanyang || true
}
trap rollback ERR
install -m 0755 bin/yuanyang /usr/local/bin/yuanyang.next
mv -f /usr/local/bin/yuanyang.next /usr/local/bin/yuanyang
cp -R web/dist/. /opt/yuanyang/web/
find /opt/yuanyang/web -type d -exec chmod 0755 {} +
find /opt/yuanyang/web -type f -exec chmod 0644 {} +
for unit in yuanyang.service yuanyang.slice yuanyang-recover.service; do
  install -m 0644 "deploy/$unit" "/etc/systemd/system/$unit"
done
systemctl daemon-reload
systemctl start yuanyang
sleep 3
systemctl is-active --quiet yuanyang
curl --fail --silent --show-error --max-time 5 http://127.0.0.1:9090/healthz >/dev/null
trap - ERR
echo "Upgrade complete; previous data, key, executable and assets preserved at $backup"
