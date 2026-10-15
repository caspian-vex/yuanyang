# 测试与验收

## 自动检查

```bash
go mod tidy
gofmt -w cmd internal
go test ./...
go vet ./...
# 在有足够内存的开发机执行，race 的开销不能用于生产性能统计：
go test -race ./internal/panel
cd web
npm install
npm run build
```

已有单元测试覆盖：路径及符号链接越界、Nginx 注入校验、HTTPS 配置生成、有界输出、AES-GCM、TOTP 标准向量、队列容量和同任务重叠、匿名访问/CSRF/只读权限。

## Linux 集成验收

在每个支持系统的干净虚拟机执行，配置 1 vCPU / 512 MB；测试机可重装，有独立 SSH 恢复通道。不要在待保留的业务服务器上进行故障注入。

1. 安装、初始化、管理员登录、注销，确认初始化令牌失效。验证错误密码 5 次限流，空闲/最长会话到期。
2. 创建运维与只读账号；直接请求变更 API 验证权限，禁用账号后旧 Cookie 失效。
3. TOTP 生成、确认启用、错误验证码拒绝、停用和本机恢复；检查数据库不含明文 TOTP/SMTP 密码。
4. 创建静态站、上传 index.html，实际访问；创建代理站，验证请求头和 WebSocket。构造非法配置，确认旧配置仍可访问。
5. 真域名 HTTP-01 签发、导入 PEM、开启 HTTPS 与跳转；模拟证书保存失败及 Nginx reload 失败，确认旧证书恢复。在隔离测试机的服务环境中设置 `YUANYANG_ACME_DIRECTORY=https://acme-staging-v02.api.letsencrypt.org/directory` 使用 ACME staging，避免消耗生产签发配额；测试证书不受浏览器信任，不得用于正式入口。
6. 上传 100 MB 文件、101 MB 文件、同名冲突、符号链接、`..`、中途断开、大目录翻页；确认临时文件清理、下载内容匹配。
7. SSH 密码/私钥登录、窗口缩放、断网与第三个连接拒绝；两路日志流、第三路拒绝、超长日志和慢客户端。
8. 非关键服务启停/自启和日志；SSH/网络/面板等关键服务拒绝停止。终止测试进程，关键进程拒绝。
9. 定时任务五段表达式、未来五次时间、立即执行、停用、超时、输出截断、重叠跳过、队列满；确认 transient unit 超时后无残留子进程。
10. 修改阈值触发 CPU/内存/磁盘告警，验证五分钟持续条件、三十分钟重复限制、恢复邮件及 SMTP 失败最多三次尝试。
11. UFW 允许/拒绝/删除规则，确认生效；不确认等待恢复；重启面板后待恢复任务仍能运行。存在其他防火墙管理器时拒绝接管。
12. 启动时磁盘不可写、运行中磁盘不足、数据库错误、`/proc` 不可读、SSH/SMTP 不可达，界面不能显示成功或正常指标。
13. 升级保留网站、证书和数据；升级失败还原旧程序；卸载保留所有业务数据。
14. 在 SSH 隧道内配置面板 HTTPS 入口，验证域名/端口、导入与受管证书绑定、新入口确认、超时恢复和本机 gateway-rollback。验证错误域名证书、占用端口、Nginx 配置冲突时旧入口继续可用。
15. 在尚未确认防火墙或入口变更时重启测试机，验证启动恢复单元能恢复原配置。确认受管证书续期同步作用于入口，引用中的证书不能删除。

## 性能

运行 `scripts/measure.sh` 并执行上述混合负载；用登录后的浏览器请求 `/api/me` 获取 CSRF，通过 Cookie 调用查询接口测延迟，不将认证 bcrypt、文件传输、命令执行或外部证书签发耗时纳入普通查询 P95。

记录机器/内核版本、CPU 限额、cgroup 数据、外部命令单位数据、请求数量/失败率、P50/P95/P99，以及 24 小时内 OOM 和重启次数。完整标准见 PERFORMANCE.md，不能用浏览器显示的系统总内存替代面板资源组测量。

普通查询压测工具（建议在独立测试客户端执行）：

```bash
# 使用已有浏览器会话，避免密码写进命令行参数；填入 Cookie 中的 session 值：
export PANEL_COOKIE='session=YOUR_SESSION_TOKEN'
go run ./cmd/bench -url https://panel.example.com -requests 2000 -concurrency 2
unset PANEL_COOKIE
```

也可使用 PANEL_USER、PANEL_PASSWORD 和 PANEL_TOTP 环境变量进行一次登录。工具混合查询 me、metrics、history、host、overview、processes，输出成功率及 P50/P95/P99；失败或 P95 超过 300 ms 时非零退出。SSH、日志和上传混合负载仍按前述步骤另外打开。
