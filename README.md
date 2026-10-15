# 鸳鸯 · Yuanyang

面向个人站长的本机运维面板。Go 后端 + Vue 3/TypeScript 前端 + SQLite，部署时不需要 Node.js 服务。

**状态：首版源码，尚未在目标 Linux 上编译或验收。100 MB 是资源预算及验收条件，不是已验证的性能结论。请先在可恢复的测试机验证。**

## 环境

- 运行：Ubuntu 22.04/24.04 LTS 或 Debian 12/13，x86_64，systemd、cgroup v2。目标机器 1 核 / 512 MB。
- 构建：Go ≥1.26，Node.js ≥20.19 和 npm。请在开发机或 CI 构建；构建工具不受运行时 100 MB 预算约束。Go 版本与已固定的 [x/crypto v0.58.0 模块声明](https://raw.githubusercontent.com/golang/crypto/v0.58.0/go.mod)保持一致。
- 系统依赖：Nginx、OpenSSH Server、sudo、UFW、CA 证书、iproute2。ACL 可选。

## 构建与安装

### 一行安装

先把本次源码修改推送到 GitHub。以下命令在云服务器的 Bash 中执行，支持 Ubuntu 22.04/24.04、Debian 12/13 的 x86_64 系统，需要 root 或 sudo。服务器需能访问 GitHub、系统软件源；源码模式还需访问 Go、Node.js 和依赖源。

**一键安装（优先安装包；最新 Release 返回 404 时自动从 main 源码构建）：**

```bash
set -o pipefail; curl -fsSL https://raw.githubusercontent.com/caspian-vex/yuanyang/main/scripts/deploy.sh | sudo bash -s --
```

**直接拉取 main 源码、自动准备工具链、构建并安装（不依赖 Release）：**

```bash
set -o pipefail; curl -fsSL https://raw.githubusercontent.com/caspian-vex/yuanyang/main/scripts/deploy.sh | sudo bash -s -- --source
```

指定版本安装：在第一条命令末尾加 `--version v0.1.1`。指定源码分支、标签或提交：在第二条命令末尾加 `--ref v0.1.1`。同一命令可重复执行，已安装时自动进入带备份及失败回退的升级流程。默认通过 apt 安装所需系统依赖；将 `sudo bash` 改成 `sudo env INSTALL_DEPS=0 bash` 可跳过 apt，但仍需提前准备必需工具。源码模式会下载临时构建工具链，不替换系统 Go/Node.js，结束后清理工具链和构建缓存。

默认模式仅在最新 Release 查询返回 HTTP 404 时回退源码；指定版本不存在、网络失败或校验失败均停止。末尾添加 `--release` 可强制只使用安装包，适合 512 MB 机器。源码编译与依赖安装不受面板运行时 100 MB 预算约束，512 MB 机器可能内存不足；脚本限制 Go 编译并行度，但不自动配置 Swap，也不保证低内存构建成功。构建失败不会停止或替换现有面板。

安装后通过下文的 SSH 隧道读取令牌并完成初始化。脚本不会将 9090 直接开放公网。

### 从 GitHub 拉取部署（推荐）

仓库地址：`https://github.com/caspian-vex/yuanyang`。推送 main、推送 `v*` 标签或在 main 手动运行工作流后，GitHub Actions 自动构建、检查并发布带 SHA-256 校验文件的 Linux x86_64 安装包。main 构建使用唯一版本名 `v0.1.0-build.<运行编号>.<尝试编号>`，关联实际构建提交，并更新 latest 下载入口。云服务器使用安装包时无需 Go、Node.js 或 npm。

先提交并推送本次修改，再在开发机发布版本：

```bash
git push origin main
git tag v0.1.0
git push origin v0.1.0
```

等待 Actions 的 `Build and release` 中 build 和 release 两个任务都成功，并确认 Releases 已出现安装包。只看到 build 成功并不代表安装包已经发布。main 的一次普通推送也会自动发布，不必额外创建标签；上面的标签命令用于发布指定版本。

在云服务器执行（需 root 或 sudo，且能访问 GitHub）：

```bash
curl -fL --proto '=https' --proto-redir '=https' \
  https://raw.githubusercontent.com/caspian-vex/yuanyang/main/scripts/deploy.sh \
  -o yuanyang-deploy.sh
# 可先查看脚本；默认通过 apt 安装系统依赖
sudo bash yuanyang-deploy.sh v0.1.0
```

也可以先拉取源码：

```bash
git clone https://github.com/caspian-vex/yuanyang.git
cd yuanyang
sudo bash scripts/deploy.sh v0.1.0
```

省略版本参数会安装最新正式 Release；已有安装自动走升级备份与失败回退。下载或校验失败不会替换现有程序。跳过系统依赖安装：`sudo env INSTALL_DEPS=0 bash scripts/deploy.sh v0.1.0`。当前下载入口面向公开仓库；私有仓库需自行下载有权限访问的安装包，执行包内安装脚本。SHA-256 用于校验传输完整性，发布权限由仓库控制。

初次安装保持 `127.0.0.1:9090` 监听，按下文建立 SSH 隧道初始化，再配置 HTTPS 公网入口。云平台安全组需放行 SSH 和实际使用的 HTTP/HTTPS 端口。

### 自行构建

```bash
bash scripts/build.sh
# 在测试服务器，以 root 执行；不自动安装依赖：
sudo bash scripts/install.sh
# 或明确允许安装所需系统软件：
sudo env INSTALL_DEPS=1 bash scripts/install.sh
```

构建脚本会安装前端依赖、生成 go.sum/package-lock.json、运行 Go 测试及 vet，并输出 Linux 二进制及前端静态资源。依赖初次解析需要网络。

初次访问建议在个人电脑建立隧道：

```bash
ssh -L 9090:127.0.0.1:9090 your-user@your-server
# 服务器上读取一次性令牌：
sudo cat /var/lib/yuanyang/setup-token
```

浏览器打开 `http://127.0.0.1:9090`，输入令牌并创建管理员。令牌在初始化后删除。公网访问在“系统设置 → 面板访问入口”中配置域名、HTTPS 端口及证书；可绑定已申请的受管证书，也可提供 PEM 证书及私钥。先放行新端口，应用后在新地址登录并于 60 秒内确认，否则自动恢复。受管证书自动续期会同步应用到面板入口。也提供 `deploy/nginx-panel.conf.example` 供手动部署参考；同一域名的入口配置应选择手动或面板管理其中一种，不能重复定义。

面板本身监听回环地址。反向代理必须覆盖 `X-Forwarded-Proto`，并保留正确 Host；不要将后端端口直接暴露公网。

## 使用

- **网站**：先在文件管理器创建 `/var/www` 下的目录，再创建静态网站或反向代理。保存前校验 Nginx，失败回退。仅管理 `yuanyang-*.conf`，不导入已有网站。
- **证书**：创建并启用 HTTP 网站，确认域名解析及公网 80 端口，申请证书或导入 PEM，再编辑网站启用 HTTPS。ACME 每日检查，提前 30 天续期。
- **文件**：只能访问 `-files` 配置的根目录；使用 Go `os.Root` 限制路径和符号链接。上传流式处理，最多一个，单文件 100 MB。目录分页，不全量排序。支持普通文件与目录复制、移动和删除；目录复制最多 64 层，遇到符号链接或特殊文件明确失败并清理新副本。
- **终端**：管理员及单独授权的运维成员可连接本机 `127.0.0.1:22`，支持两个标签页、最多两个会话。使用系统账号密码或私钥认证，支持加密私钥口令，不持久保存凭据；校验本机 Ed25519/ECDSA/RSA 主机公钥。root 能否登录取决于系统 SSH 配置。撤销权限后，已连接会话最多 30 秒内关闭。
- **服务**：systemd 查询及启停、重启、自启。关键服务受保护。日志流最多两个，每个浏览器最多保留 1,000 行。
- **定时任务**：标准五段 Cron，不补跑停机任务。最多排队 32 个；同任务不重叠。非 root 执行账号；外部命令作为独立 transient systemd 服务，128 MB 内存、50% CPU、最多 32 个进程，1–900 秒超时。任务输出最多 64 KB 并明确标记截断。
- **告警**：CPU/内存默认 90%、磁盘 85%、负载默认 CPU 核数 ×2，持续 5 分钟触发。支持指定服务停止告警、告警确认和跨午夜的通知静默时段。持续异常 30 分钟再次提醒，恢复发送通知。邮件要求 STARTTLS，最多尝试三次。
- **防火墙**：UFW 变更后 60 秒内确认连接，超时由独立定时服务恢复原规则。启用时保留 SSH、80、443、9090 端口；自定义入口务必先配置放行。检测到 firewalld/nftables 服务正在运行时拒绝接管。
- **权限**：管理员全部权限；运维成员管理网站、服务和文件；只读成员查看状态。所有接口服务端鉴权，变更验证 CSRF。禁用/改权限会撤销 HTTP 会话。

文件默认属主为面板用户。安装脚本不递归修改现有网站属主；请为计划管理的网站目录授予面板用户读写 ACL，并为 Nginx 保留读取权限，例如：

```bash
sudo apt-get install acl
sudo setfacl -m u:yuanyang:rwx /var/www
sudo setfacl -R -m u:yuanyang:rwX /var/www/your-site
```

## 资源与数据

`yuanyang.slice` 为面板及临时恢复辅助进程设置合计 `MemoryMax=100000000`（十进制 100 MB）、禁止 Swap、CPU 配额 100%。Go 使用 `GOMAXPROCS=1` 和 64 MiB 软内存目标。硬限制触发 OOM 意味着验收失败，不能视为通过。

外部任务运行在独立 systemd 单元，需另行统计，不能从面板之外运行额外采集或后台常驻进程来隐瞒内存。SQLite 使用容量 64 的单写入队列、事务批量提交，缓存 2 MB；实时监控最多 360 个点；历史按分钟/小时聚合；普通 API 最多同时十六个，认证最多同时两个；流式文件和日志不整体缓存。

数据默认 `/var/lib/yuanyang`：SQLite、初始化令牌、`secret.key`、加密 ACME 账号、证书元信息。SMTP 密码和 TOTP 密钥 AES-GCM 加密。根证书私钥安装到 root 独占的 `/etc/yuanyang/certificates`，接口不提供下载。

保留策略：实时内存缓存 1 小时（重启后重新采样）、分钟 7 天、小时 30 天、审计 180 天、任务 30 天。详细内存/性能验收见 [docs/TESTING.md](docs/TESTING.md)。

## 升级、恢复、卸载

```bash
sudo bash scripts/upgrade.sh
sudo systemctl status yuanyang
sudo journalctl -u yuanyang -n 100
# 停止面板后从标准输入重置最早的管理员密码，并清除其 TOTP / 所有会话：
sudo systemctl stop yuanyang
read -rs -p 'New password: ' PANEL_PASSWORD; echo
printf '%s' "$PANEL_PASSWORD" | sudo /usr/local/bin/yuanyang recover /var/lib/yuanyang
unset PANEL_PASSWORD
sudo systemctl start yuanyang
# 必要时人工触发待执行的防火墙恢复：
sudo /usr/local/bin/yuanyang firewall-rollback
# 人工恢复尚未确认的 HTTPS 入口修改：
sudo /usr/local/bin/yuanyang gateway-rollback
# 卸载程序，保留业务数据、证书、网站配置：
sudo bash scripts/uninstall.sh
```

升级前保存停止状态下的数据、加密密钥、服务配置及旧程序副本；启动或健康检查失败时自动恢复该快照，同时保留失败版本的数据副本。跨版本数据库回退需要从相同时间的完整快照恢复，不能只替换旧二进制。`secret.key` 丢失后，加密信息不可恢复。

## 交付边界

本次提供首版源码、核心操作界面和资源约束，不声称已经通过编译或生产验收。系统设置支持界面内修改对外访问域名、HTTPS 端口及证书，并自动回退未确认配置；内部后端默认固定 127.0.0.1:9090。定时任务命令和认证凭据不写入审计，执行输出仅管理员可读。终端首版使用本机 22 端口。文件与目录复制采用流式处理，目录限制 64 层且不跟随符号链接。

文件根目录、上传上限、会话时间、审计/任务保留天数、IP 白名单及邮件/告警规则可在系统设置修改；监控保留分层固定，以控制内存。审计和任务列表最多返回 200 条，执行输出列表仅展示前 4 KB，展开时按需获取完整的受限输出。站点上限 200，任务上限 1,000，用户上限 200；这些上限用于明确拒绝超出当前内存预算的规模。

Docker、数据库、业务备份恢复和运行环境管理属于第二阶段，当前没有占位 API 或模拟数据。接口说明见 [docs/API.md](docs/API.md)，未执行的验收项目见 [docs/PERFORMANCE.md](docs/PERFORMANCE.md)。
