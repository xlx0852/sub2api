# 全平台简化选号生产发布（2026-09-20）

## 版本与范围

用户明确要求“所有平台都走简化后的选号逻辑”并连续授权继续。此次删除旧执行逻辑后，对所有已支持平台默认生效，包括 compact 的选号；真实续链、工具配对、平台硬门槛和计费边界保留。

- 代码提交：`96653045de78ed3769870d42396774a2ad29c83c`。
- 生产 Go 代码净减少约 4,111 行（不含测试/前端/文档）。
- API 二进制 SHA-256：`c7ef71968bc6e67bf5c7ad0dfd7ee454ea420e4f77909d9e7500a67a0d514093`。
- 前端 archive SHA-256：`38deb8f6061ea82ab597d13f109e35813490aabec6051eb6e07a20f72b1cf009`。
- 二进制由 detached clean worktree 构建；`main.Commit` 显式写入上述提交，Linux amd64、CGO=0、embed。工作区原有 item-ID 长度修复和历史 OpenSpec 归档均未混入。
- 前端在同一 clean worktree 先执行 vue-tsc -b、Vite build。pnpm 11 对临时 node_modules symlink 的自动 install 检查拒绝无 TTY 清理后，改为直接执行现有 pnpm 安装的 `.bin/vue-tsc` 和 `.bin/vite`；未允许其删除共享依赖。构建成功，只有现有 chunk-size 警告。

## 当前部署

- 新容器：`sub2api-api-candidate-0.1.391-allplatforms-96653045d`。
- API 端口：**18344**。
- bind-mounted binary：`/opt/sub2api/api/releases/0.1.391-allplatforms-96653045d/sub2api`。
- 外部前端：`/opt/sub2api/frontend/releases/0.1.391-allplatforms-96653045d`，`frontend/current` 已原子指向该目录。
- 移除容器中的 `GATEWAY_OPENAI_SCHEDULER_SIMPLE_SELECTION_GROUP_IDS`：不再靠列表启用，所有平台与后续新分组自动使用共享 priority/load/equal-shuffle。
- 其余 env、资源、网络、mount、restart policy、权限、entrypoint 与 healthcheck 对照旧容器保持不变。凭证 env 只在内存中复制，没有写入文件或打印。
- Nginx upstream：18343 → 18344；`nginx -t` 与 reload 通过。
- 成对切换时刻：`2026-09-20T15:01:50.437935+00:00`（北京时间 **23:01:50**）。
- origin `/health` 返回 JSON status=ok；新前端 index 和前三个静态资产与 release 文件 SHA 比对一致。

## 实测

仅使用管理员 user_id=1 所有的现有 key，密钥不回显、不落盘；不改账号映射/启停、分组供给或真实用户 session。

切流前候选：
- OpenAI sol JSON、astra SSE：200 且 completed，用量账号 69。
- Grok grok-4.6 Chat：200，正常终态，用量账号 131。
- Kimi kimi-for-coding Chat：200，正常终态，用量账号 107。
- Anthropic 兼容分组 kimi-k3 Messages：200/end_turn，用量账号 81。
- Antigravity gemini-3.8-flash：正确 `/antigravity/v1beta/...:generateContent` 入口 200/finishReason，用量账号 122。
- 首次 Antigravity 探测误用了已下线的 `/v1beta` 入口；新旧实例均 404，确认不是回归。已更正入口后再验证成功，未将最初 404 隐藏为成功。

切流后经正式 TLS origin 再次发送上述五个平台请求，全部 200 且有对应完成终态，耗时约 1.9–4.0 秒；Antigravity 本次由账号 118 承接。

截至 **23:05:15**：
- 切流后 ops_error_logs 新增 **0** 条。
- 用量入账：OpenAI **70**，Grok **2**，Kimi **1**，Antigravity **1**，Anthropic **1**，合计 **75**。
- 新容器日志确认五个平台均执行 `account_simple_selection`，无空候选 reject trace、panic/fatal；Docker healthy、restart count=0。
- 用量窗口含冒烟和自然流量，也可能含排空旧连接结算；用量表不标实例，不能把窗口全部记录强归为新实例处理。

## 覆盖边界

生产当前没有原生 Gemini 分组（原生供给已退役）；Gemini 兼容排序通过本地回归，Gemini 协议的线上实测由 Antigravity 承接。
compact 的 capability/unknown/全局模型并发预算/续链与工具语义通过本地回归，本次没有发送昂贵的线上 compact 请求。
这些限制不影响“所有平台默认同一排序”的实现结论，但不能宣称全模型、全协议、全负载已线上长窗口验证。

## 回滚与排空

- API 回滚：`sub2api-api-candidate-0.1.391-simple-all-1817d3627`，端口 **18343**。
- SPA 回滚：`/opt/sub2api/frontend/releases/0.1.390-f3bc03625860`，`frontend/rollback` 已指向该目录。
- Nginx 备份：`/opt/sub2api/api/releases/0.1.391-allplatforms-96653045d/nginx.before.conf`。
- 新版不再提供旧算法热开关；回滚需要 API 与 SPA 成对回切。先确认旧容器健康，再修改唯一 upstream/repoint 前端，nginx -t 后 reload。
- 23:05:15，旧 18341/18342 已确认 socket=0 后停止保留；18343 尚有 2 条 socket，留运行自然排空，不强杀 WS/SSE。

## 后续

继续按真实业务窗口监控 routing-503、credential-class 误拒、各平台模型门槛、工具续链和 busy wait。不要以短窗口零错误代替长期容量验证，也不要为了分流数据人为恢复停用账号。

## 延长观察更新（23:12 左右）

延长窗口后不再声称“没有任何错误记录”：近五分钟出现 40 条账号 69 的 `model_mismatch` warning（sent=gpt-6-astra、responded=gpt-5.6-luna，HTTP status 字段为空）。这是独立的上游模型标识不一致观测，不能误归为 503，也不能单凭该条 warning 断言每次业务请求的最终结果。
截至该次查询，自 23:01:50 切流以来 `ops_error_logs.status_code>=500` 为 0，routing-503 为 0。API 仍 healthy、restart=0，nginx/current SPA 指向本次发布。后续模型标识排查与本次选号退役分开处理。
