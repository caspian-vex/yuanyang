# HTTP 接口

统一 `/api` 前缀，JSON 请求必须使用 `Content-Type: application/json`；JSON 返回。失败为 `{ "error": "message" }`，HTTP 400/401/403/404/409/429/500/503。列表默认 50，`limit` 最大 200，`offset` 为非负数。变更接口需 Cookie 和 `X-CSRF-Token`。不允许跨域。

登录字段使用 PascalCase，资源字段按下表所示。后端拒绝未识别的 JSON 字段。`GET /healthz` 是不含敏感信息的部署健康检查，不受面板 API 白名单影响。服务列表返回 `{items:Service[]}`，支持 offset/limit/q；Service 包含 name/state/sub/startup/description。

| 接口 | 方法 | 权限与内容 |
|---|---|---|
| `/status` | GET | 公开；initialized/version |
| `/login` | POST | Name、Password、Code；返回用户与 CSRF，设置 HttpOnly Cookie |
| `/me` | GET | 当前用户 id/name/role/csrf |
| `/logout` | POST | 撤销当前会话 |
| `/password` | POST | Password、Code、NewPassword；改密并撤销会话 |
| `/sessions` | GET/POST | 当前用户会话时间列表；POST `{All:true}` 撤销自己的全部会话 |
| `/host` | GET | 本机系统信息 |
| `/metrics` | GET | 最近一次共享采样，available=false 时显示不可用 |
| `/overview` | GET | 网站、证书临期、异常服务与告警计数，10 秒共享缓存 |
| `/ports` | GET | 最多 200 个监听端口及关联进程，缓存 60 秒；不能读取的进程显示未知 |
| `/domains/check?domain=example.com` | GET | DNS 解析结果及可达性检查说明 |
| `/diagnostics` | GET | 管理员；脱敏 JSON 诊断文件，不含配置秘密 |
| `/login-blocks` | GET/POST | 管理员；登录封禁列表，POST `{IP:"..."}` 解封 |
| `/history?range=1h` | GET | 1h/24h/7d/30d，数组 |
| `/processes?q=&offset=0` | GET | 分页进程 |
| `/processes` | POST | 管理员；PID；SIGTERM，保护关键进程 |
| `/reboot` | POST | 管理员；Password、Code、Confirm；5 秒后重启 |
| `/files?path=relative` | GET | 管理员/运维；目录 JSON 或文件流 |
| `/files?path=relative&overwrite=true` | PUT | 原始文件流，最多 100 MB |
| `/files?path=relative` | POST | Action=mkdir/delete/move/copy，Target、Confirm；永久删除必须确认 |
| `/terminal` | WebSocket | 管理员或单独授权运维；同源 Origin；首帧 Username/Password/PrivateKey/Passphrase/CSRF/Cols/Rows；之后二进制输入、JSON 尺寸调整，二进制输出 |
| `/sites` | GET/POST/DELETE | 网站列表/保存/删除；变更需管理员或运维 |
| `/certificates` | GET/POST | 管理员；Name 对应网站；Action=issue/import；Email、Agree 或 Certificate、PrivateKey |
| `/services` | GET/POST | 列表；Name、Action=start/stop/restart/enable/disable/logs |
| `/logs?unit=nginx.service` | GET/SSE | 管理员/运维；data 为 JSON 字符串，每流最长 30 分钟 |
| `/logs?site=example&kind=error&q=` | GET/SSE | 网站 access/error 日志，可关键词过滤 |
| `/tasks` | GET/POST/DELETE | 管理员；POST 保存资源或 `{id:1,action:"run"}` 入队；返回 202 |
| `/runs` | GET | 管理员；最近任务执行记录，含截断后的输出 |
| `/alerts` | GET | 告警列表 |
| `/audit` | GET | 管理员；审计列表 |
| `/users` | GET/POST | 管理员；ID、Name、Password、Role、Disabled、Action=save/delete |
| `/security` | GET | 管理员；基础安全状态 |
| `/security/totp` | POST | Password、Action=prepare/enable/disable、Code；成功启停撤销全部会话 |
| `/firewall` | GET/POST | 管理员；Action=status/enable/disable/allow/deny/delete/confirm；Port、Protocol、Source、Policy、Confirm |
| `/settings` | GET/POST | 管理员；受限的键值映射；密码读取为占位符 |
| `/gateway` | GET/POST | 管理员；HTTPS 入口状态；Action=apply/confirm，Domain、Port、CertificateName 或 Certificate/PrivateKey；apply 要求 Password、Code、Confirm |
| `/mail/test` | POST | 管理员；测试当前邮件配置 |

网站资源：`id,name,domains:string[],kind:static|proxy,root,upstream,enabled,https,redirect,notFound`。站点名称不可更改，静态目录必须位于 `/var/www/`。域名不可被多个受管网站重复占用。notFound 为可选自定义 404 URI。

任务资源：`id,name,schedule,command,account,enabled,timeout,workdir`。timeout=1–900，account 不得 root/yuanyang。workdir 为空时使用账号主目录。GET 返回 `{task,next:string[]}` 列表，next 为未来五次执行时间。执行记录含 exit_code；列表输出仅返回前 4 KB，避免大批输出突破内存预算。

账号初始化由本机命令 `sudo -u yuanyang /usr/local/bin/yuanyang init-admin /var/lib/yuanyang` 在服务停止时执行；仅数据库为空时创建 admin 并在终端显示随机密码，已存在账号则保持不变，不提供网页注册接口。

设置键：`panel.name`、`smtp.host/port/user/password/from/to`、`alerts.enabled/cpu/memory/disk/load/services`、`alerts.quiet.start/end`、`security.allowIPs`、`session.idleMinutes/maxHours`、`files.root`、`uploads.maxMB`、`retention.auditDays/taskDays`。SMTP 必须支持 STARTTLS，默认端口 587。白名单不得排除当前来源 IP；部署服务信任回环代理的 X-Real-IP，该值必须由反向代理覆盖。

`POST /alerts {ID:1}` 确认告警，不改变恢复状态；GET 可加 `state=active|recovered`。审计支持 `user/action/result` 模糊筛选及 `since/until` Unix 秒时间筛选。证书支持 `DELETE /certificates {Name:"site-name"}`，仍被受管站点或面板入口引用时拒绝删除。用户创建/编辑支持 Terminal 布尔值，只允许对运维角色授权。`GET /run?id=1` 获取单次执行完整输出，最多约 64 KB。

受控 root helper 通过 stdin 接收固定操作的 JSON，不提供任意 root Shell。任务命令只能在非 root 系统账号下运行。API 变更审计不记录登录凭据、证书私钥及任务命令内容。
