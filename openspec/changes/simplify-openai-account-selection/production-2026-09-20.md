# 生产上线记录：OpenAI group 4 简化调度（2026-09-20）

## 授权与范围

用户明确要求“直接上线”。仅启用 OpenAI group 4 的非 compact 简化 profile；其他 group/平台/compact 路径及独立前端均不切换策略。

本记录取代 `verification.md` 中候选交付阶段的“未上传、未启用”状态。历史本地验证记录保留，不倒改为当时已经上线。

## 版本与配置

- 源码：`1817d36270384cc6f49c9ef89e9bac034dd238c3`。
- 二进制 SHA-256：`c8706411f738964adb7e7c2ccc88207df92684bbb63080c40cc258fa4b2fa45e`，上传后校验一致。
- 二进制：`/opt/sub2api/api/releases/0.1.391-simple-1817d3627/sub2api`。
- 新容器：`sub2api-api-candidate-0.1.391-simple-1817d3627`，`127.0.0.1:18342`。
- 环境变量：`GATEWAY_OPENAI_SCHEDULER_SIMPLE_SELECTION_GROUP_IDS=4`。
- 通过内存中的 Docker inspect/create JSON 复用旧实例配置，不把继承的凭证 env 写到文件或输出。
- 已核对网络、其他 bind mounts、restart policy、资源限制、日志设置、权限、entrypoint、command、healthcheck 等保持不变。
- Nginx：`/etc/nginx/conf.d/code-sicts-shop.conf`，upstream `18341 → 18342`。
- 切流时刻：`2026-09-20T12:47:21.144664+00:00`（北京时间 20:47:21）。
- 切流前备份：`/opt/sub2api/api/releases/0.1.391-simple-1817d3627/nginx.before.conf`。
- 无前端发布、无数据库 migration、无账号开关改写、无真实会话批量清理。

## 切流前故障对照

使用管理员所有的测试 Key ID 47，密钥只在进程内存中读取和传递。
创建独立 synthetic session，将其临时绑定到当前不可调度的 apikey 账号 89；不修改真实用户的已有 session。

- 旧 18341，同样的 `gpt-5.6-sol` 请求：HTTP 503，synthetic binding 仍为 89。
- 新 18342，JSON：HTTP 200、completed=true，binding 更新为 132。
- 新 18342，SSE：HTTP 200、response.completed，binding 更新为 132。
- 两条成功用量均记在账号 132；测试 session 及 epoch 元数据随后删除。

这验证的是“失效绑定不能把健康 OAuth 候选排除”，不是靠提高 132 优先级、恢复 89 或修改生产账号来制造成功。

## 切流后验证

- 新容器 `/health` 返回 JSON `{"status":"ok"}`，Docker healthy；不是用 SPA 的 200 冒充健康检查。
- Nginx 配置测试通过，直连 TLS origin `/health` 连续 3 次通过。
- 经 Nginx TLS origin 发出 sol/astra × JSON/SSE 共 4 次请求：全部 HTTP 200 且 completed，耗时 2.26–3.05 秒，4 条用量全部落 132。
- 20:51:21 查询切流后窗口：group 4 新增成功用量 43 条（132：sol 15、astra 28），全站 ops_error_logs 新增 0 条。
- 新容器同期日志确认 simple profile 已执行：group 4、132 的 sol 16 次、astra 14 次选号；选号数、连接 turn 数和 usage 数口径不同，不强行等同。
- 新容器无 no-candidates reject trace、panic/fatal，restart count=0。
- 132 保持 active+schedulable、ignore_upstream_401=true。

## 保留边界与后续观测

- 上线时 69/89/112 已是 schedulable=false，132/126 为 true；原样保留。因此本次生产窗口不构成多个健康 OAuth 账号公平分流的实测证明。
- 观察到部分 132 的 session affinity 被重新校验失效并重绑，但未造成请求错误；未把该日志直接归因为账号故障，继续观察 epoch/绑定稳定性。
- 43 条成功用量含冒烟请求和自然请求；存量旧连接仍在排空，usage 表本身不区分容器版本，不能把全部入账数强归为新容器完成。
- 早期未路由实例 `sub2api-api-candidate-0.1.391-availfix-c291389c2` 在确认 18337 无连接后已停止保留。
- 旧 18341 容器截至 20:51:21 尚有存量连接，保留运行以避免断流，同时作为快速回滚点。后续应在连接排空后停止，而非强杀 WS/SSE。

## 回滚

原容器：`sub2api-api-candidate-0.1.391-ignore401-63f365071`，端口 18341。
确认旧实例健康后，将 Nginx 唯一 upstream 恢复到 18341，先 `nginx -t` 再 reload。优先做精确 upstream 回改；备份仅作核对，避免覆盖期间其他运维配置。
旧容器未启用新 group scope；回切无需删除会话、重置账号或改前端。

## 用户授权后的全量扩围（当前状态）

用户随后明确要求“全量切”。查询 `groups WHERE deleted_at IS NULL AND platform='openai'` 的完整集合为 `[4,42]`；本次将范围从 `[4]` 扩大到 `[4,42]`，仍不改变其他平台和 compact 策略。

- 当前容器：`sub2api-api-candidate-0.1.391-simple-all-1817d3627`，端口 **18343**。
- 当前二进制：`/opt/sub2api/api/releases/0.1.391-simple-all-1817d3627/sub2api`；同一已验证源码与 SHA-256，未重新混入工作区改动。
- 运行 env 已核对：`GATEWAY_OPENAI_SCHEDULER_SIMPLE_SELECTION_GROUP_IDS=4,42`。
- 切流时刻：`2026-09-20T12:58:44.682705+00:00`（北京时间 20:58:44）。
- Nginx upstream：18342 → 18343；`nginx -t`、origin `/health` 三次检查通过。
- 切流前候选 sol 请求 completed；切流后经 TLS origin 的 sol JSON、astra SSE 两次请求均 HTTP 200 且 completed。
- 截至 21:00:40，切流后 ops_error 新增 0 条；group 4 共 25 条成功用量（132：astra 18、sol 7），group 23 Grok 共 16 条成功用量（131：grok-4.6 11、grok-4.5 5）。
- 新容器日志确认 group 4 简化 profile 已执行 14 次选号，无空候选 reject trace；Docker healthy，restart count=0。
- group 42 当前无账号绑定、无管理员所有的可用测试 key、无自然业务流量。本次只验证其配置覆盖，未伪造调用成功证据，也未为测试修改供给或使用其他用户的 key。
- 新建 OpenAI 分组不会自动加入此显式列表；后续增加分组时需同步 scope。

扩围回滚点为 **18342** 的 group-4-only 容器；Nginx 备份为 `/opt/sub2api/api/releases/0.1.391-simple-all-1817d3627/nginx.before.conf`。旧 18341/18342 截至 21:00:40 仍有存量 socket，保留运行自然排空，不强行断开 WS/SSE。新请求/新连接均由当前 nginx upstream 承接；存量连接不能被当作已经强制迁移到新容器。
