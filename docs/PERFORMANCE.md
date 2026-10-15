# 性能验收报告（待执行）

截至本次交付，开发环境为 Windows，未安装 Go/npm 工具链，未下载项目依赖，未执行 Go 编译/单元测试、Vue 完整构建、Linux 集成测试或压力测试。已使用本机 Node.js 对 10 个 TypeScript 脚本块进行语法解析、8 个 Vue 文件进行标签配对检查、39 个 Go 文件进行括号/导入基础扫描，并检查 JSON 清单与脚本 LF 换行；这些检查不能替代编译、类型检查或运行测试。以下性能项目全部为待测项，不是实测结果。

| 项目 | 通过标准 | 当前结果 |
|---|---|---|
| 空闲内存 | yuanyang.slice 稳态 ≤50 MB | 未测 |
| 混合负载内存 | 全部面板进程合计峰值 ≤100 MB，无 OOM | 未测 |
| 空闲 CPU | 单核平均 ≤5% | 未测 |
| 普通查询 P95 | 本地网络 ≤300 ms | 未测 |
| 混合负载 | 仪表盘 + SSH + 日志 + 100 MB 上传时 API 达标 | 未测 |
| 业务规模 | 两个浏览器、20 网站、100 任务、30 天指标 | 未测 |
| 稳定性 | 连续 24 小时，无持续增长/任务失效 | 未测 |
| 系统兼容 | Ubuntu 22.04/24.04、Debian 12/13 | 未测 |

测量 `yuanyang.slice` 的 memory.current/memory.peak，包括服务、sudo helper 和防火墙及访问入口恢复进程。外部任务按独立 `yuanyang-task-*` 单元另行测量，默认受 128 MB/50% CPU 限制。Nginx、SSH 服务、操作系统与浏览器不计入面板预算。

```bash
bash scripts/measure.sh 86400 > panel-resources.csv
systemctl show yuanyang.slice -p MemoryCurrent -p MemoryPeak -p CPUUsageNSec
systemctl show 'yuanyang-task-*' -p MemoryCurrent -p MemoryPeak -p CPUUsageNSec
```

内存达到硬上限后被杀、频繁重启、漏采、丢失告警或拒绝正常验收负载都判定失败。Go 软内存目标及 systemd 配额只能限制开销，不能证明目标已实现。
