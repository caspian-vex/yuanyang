#!/usr/bin/env bash
set -euo pipefail
[[ $(id -u) -eq 0 ]] || { echo 'Run as root'; exit 1; }
systemctl disable --now yuanyang || true
# Preserve data, websites, certificates, managed Nginx configs, service user and web assets.
rm -f /etc/systemd/system/yuanyang.service /etc/sudoers.d/yuanyang
if [[ ! -f /var/lib/yuanyang-firewall/snapshot.json && ! -f /var/lib/yuanyang-gateway/backup.json ]]; then systemctl disable --now yuanyang-recover.service || true; rm -f /etc/systemd/system/yuanyang-recover.service /usr/local/bin/yuanyang; else echo 'Pending firewall rollback: binary retained until recovery completes.'; fi
systemctl daemon-reload
echo 'Panel disabled. All business files, certificates, configuration and data retained.'
