#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ $(id -u) -eq 0 ]] || { echo 'Run as root'; exit 1; }
[[ -f /etc/os-release ]] || exit 1
. /etc/os-release
case "${ID}:${VERSION_ID}" in ubuntu:22.04|ubuntu:24.04|debian:12|debian:13) ;; *) echo 'Supported: Ubuntu 22.04/24.04, Debian 12/13'; exit 1;; esac
[[ $(stat -fc %T /sys/fs/cgroup) == cgroup2fs ]] || { echo 'cgroup v2 is required for the aggregate memory budget'; exit 1; }
grep -qw memory /sys/fs/cgroup/cgroup.controllers || { echo 'memory controller unavailable'; exit 1; }
grep -qw cpu /sys/fs/cgroup/cgroup.controllers || { echo 'cpu controller unavailable'; exit 1; }
[[ $(uname -m) == x86_64 ]] || { echo 'This build targets x86_64'; exit 1; }
[[ -f bin/yuanyang && -f web/dist/index.html ]] || { echo 'Run scripts/build.sh first'; exit 1; }
if [[ ${INSTALL_DEPS:-0} == 1 ]]; then
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y nginx openssh-server sudo ufw ca-certificates iproute2 acl curl
fi
for tool in nginx sshd sudo ufw systemctl visudo ss curl; do command -v "$tool" >/dev/null || { echo "Missing $tool; install dependencies or set INSTALL_DEPS=1"; exit 1; }; done
nginx -t
nginx -T 2>&1 | grep -F '/etc/nginx/conf.d/*.conf' >/dev/null || { echo 'Nginx must include /etc/nginx/conf.d/*.conf inside http{}'; exit 1; }
[[ ! -f /etc/systemd/system/yuanyang.service ]] || { echo 'Already installed; use scripts/upgrade.sh'; exit 1; }
if ss -H -ltn 'sport = :9090' | grep -q .; then echo 'Port 9090 is in use'; exit 1; fi
getent group yuanyang >/dev/null || groupadd --system yuanyang
id yuanyang >/dev/null 2>&1 || useradd --system --gid yuanyang --home-dir /var/lib/yuanyang --shell /usr/sbin/nologin yuanyang
usermod -a -G adm,systemd-journal,www-data yuanyang
install -d -m 0755 /opt/yuanyang /opt/yuanyang/web /var/www
install -d -o yuanyang -g www-data -m 0710 /var/lib/yuanyang
install -d -o yuanyang -g www-data -m 0755 /var/lib/yuanyang/acme /var/lib/yuanyang/acme/.well-known /var/lib/yuanyang/acme/.well-known/acme-challenge
install -d -o root -g root -m 0700 /etc/yuanyang /etc/yuanyang/certificates
# /var/www remains owned by its existing owner. Grant only the panel account access.
if command -v setfacl >/dev/null; then setfacl -m u:yuanyang:rwx /var/www; else echo 'Grant yuanyang write access to your site directories with ACLs before uploading.'; fi
install -o root -g root -m 0755 bin/yuanyang /usr/local/bin/yuanyang
cp -R web/dist/. /opt/yuanyang/web/
find /opt/yuanyang/web -type d -exec chmod 0755 {} +
find /opt/yuanyang/web -type f -exec chmod 0644 {} +
chown -R root:root /opt/yuanyang
printf '%s\n' 'yuanyang ALL=(root) NOPASSWD: /usr/local/bin/yuanyang helper' > /etc/sudoers.d/yuanyang
chmod 0440 /etc/sudoers.d/yuanyang
visudo -cf /etc/sudoers.d/yuanyang
install -m 0644 deploy/yuanyang.service /etc/systemd/system/yuanyang.service
install -m 0644 deploy/yuanyang.slice /etc/systemd/system/yuanyang.slice
install -m 0644 deploy/yuanyang-recover.service /etc/systemd/system/yuanyang-recover.service
systemctl daemon-reload
initial_credentials=$(sudo -u yuanyang /usr/local/bin/yuanyang init-admin /var/lib/yuanyang)
systemctl enable yuanyang-recover.service
systemctl enable --now yuanyang
ready=0
for attempt in {1..30}; do
  if curl --fail --silent --max-time 2 http://127.0.0.1:9090/healthz >/dev/null; then ready=1; break; fi
  sleep 1
done
[[ $ready == 1 ]] || { echo 'Startup health check failed. Inspect: journalctl -u yuanyang -n 100'; exit 1; }
if command -v ufw >/dev/null && LC_ALL=C ufw status | grep -q '^Status: active'; then
  ufw allow 9090/tcp
fi
echo 'Installed. Open http://<server-public-ip>:9090 in your browser.'
echo 'Allow inbound TCP 9090 in your cloud security group or provider firewall.'
echo '========== 鸳鸯面板部署完成 =========='
panel_host=${YUANYANG_PUBLIC_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}' || true)}
[[ $panel_host != *:* ]] || panel_host="[$panel_host]"
printf '访问地址：http://%s:9090\n' "${panel_host:-服务器公网IP}"
echo '若服务器使用 NAT，请将地址替换为公网 IP，并放行云平台安全组 TCP 9090。'
echo '部署端口：9090 / TCP'
printf '%s\n' "$initial_credentials"
unset initial_credentials
echo 'Existing sites, firewall rules, SSH configuration and cron entries have not been replaced.'
