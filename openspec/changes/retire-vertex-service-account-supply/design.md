## Context

`service_account` 同时用于 Anthropic Vertex、Gemini Vertex 和 Vertex 批图。完整退役必须同时处理控制面、存量数据、调度候选、token 交换、协议适配和批图，否则仍存在可达的生产执行路径。当前 active change `remove-gemini-api-batch-provider` 明确保留 Vertex，必须同步修正规格和任务。

## Goals / Non-Goals

- Goals:
  - 阻止 Vertex Service Account 的新增、恢复、激活、测试、调度和运行。
  - 幂等停用存量账号并保留历史财务与审计数据。
  - 删除普通网关和批图中的全部 Vertex 专用生产执行路径。
- Non-Goals:
  - 不转换存量凭据。
  - 不删除历史账号或财务记录。

## Decisions

### Decision 1: 统一封锁所有重新上线写路径

共享服务对 `type=service_account` 返回稳定 retired 错误。创建、类型转换、状态更新、恢复、清错、测试、批量启用和 repository 绕过路径都必须 fail-closed。

### Decision 2: 迁移只停用，不删除历史行

幂等迁移把所有 Service Account 账号置为 inactive/non-schedulable。账号、凭据密文和历史引用继续保留，管理员只能识别和删除历史行，不能恢复运行。

### Decision 3: 删除运行时和批图实现

删除 Vertex token provider、URL/body 转换、Anthropic/Gemini 分支、探测、调度和批图 Provider。历史类型的最小解析/展示兼容只在读取旧行需要时保留。

### Decision 4: 对齐 Gemini 批图退役 change

`remove-gemini-api-batch-provider` 中所有“Vertex remains supported”条款、fixtures 和待部署门禁必须更新为无 Google 批图 Provider 的最终状态。

## Risks / Trade-offs

- 存量活跃账号被停用会减少供应容量。缓解：部署前审计账号、近期流量和替代供应。
- 遗漏恢复路径会让退役账号重新上线。缓解：service/repository 双层守卫和写路径矩阵测试。
- 删除 Vertex 批图会影响非终态任务。缓解：迁移前审计任务表，先阻止新任务并完成终结。
- 删除共享 Google 代码可能影响其他 OAuth。缓解：只删除 Service Account/Vertex 独占引用并跑 Gemini/Antigravity 回归。

## Migration Plan

1. 只读审计生产存量账号、近期 usage、分组绑定和批图非终态任务。
2. 先封锁全部写入/恢复路径，再执行幂等停用迁移。
3. 删除运行时、批图和前端实现并完成全链路回归。
4. staging 验证迁移、拒绝路径、非 Vertex 网关和补偿回滚。
5. 以 `both + db_change` 发布生产；应用可回滚，inactive 数据只通过显式补偿 SQL 恢复。

## Open Questions

- 无。当前边界固定为停止新增、保留存量。
