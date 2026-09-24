## Context

mirasim relay 上有两条只读元数据接口，语义完全不同：

| 接口 | 维度 | 内容 | 实测结论 |
| --- | --- | --- | --- |
| `GET /v1/model-roster` | **全局**（不含账号） | `{version, agents:{claude,codex,dsh,kimi}, models, cli}`，17 个模型 id + ctx/label/pricing/effort + 各 CLI 最低版本 | 内容与「本账号能用什么」无关；默认选中的 `claude` 组 6 个模型在当前账号上全部 404 |
| `GET /v1/models` | **账号维度** | `{object:"list", data:[{id, object, owned_by:"mirasim-router"}]}` | 列出 5 个 id，逐个真实调用**全部 200 + 返回内容** |

`GET /v1/models` 也不需要 `x-mirasim-probe` 头（带与不带结果一致）。

因此「只接入可以用的模型」的正确实现是：**白名单来源改用账号维度的 `/v1/models`**，而不是在
全局 roster 上做启发式过滤（roster 里压根没有 `deepseek-v4-flash` /
`deepseek-v4-flash-vision-exp` 这两个可用 id，任何过滤都不可能补出来）。

## Goals / Non-Goals

- Goals:
  - 「同步上游模型」返回的集合 == 该账号当前真正可调用的模型集合。
  - 保持既有错误映射、脱敏与「显式同步绕过缓存」语义。
  - 用一条 E2E 把「列出的每个 id 都能真实调用」钉成回归断言。
- Non-Goals:
  - 不改变 `/v1/limits` 用量链路与 `mirasim_windows` 展示。
  - 不改变模型转发（`/v1/messages` 等）与调度、计费逻辑。
  - 不新增管理页面；不改前端组件。
  - 不做「未列入白名单的模型在转发时强制拒绝」——白名单是配置，不是网关 ACL。

## Decisions

### D1: 白名单来源改用 `GET /v1/models`（采纳）

- 理由：它是唯一**账号维度**的可用性信号，实测精确率 5/5；roster 是全局目录，实测 0/6
  （默认 agent）可调用，且缺失 2 个可用 id。
- 复用的既有机制：`mirasimRelayClient.routeAuthHeaders`（明文签名头 + 不密封）、缓存
  device ticket/access token 优先（不触发 refresh 轮换）、独立超时。
- 解析：OpenAI 风格 `{object:"list", data:[{id}]}`。只取 `id`；`id` 必须是非空 JSON 字符串，
  `trim` 后去重，稳定排序（`sort.Strings` 已是确定序）。
- 空列表 / 全部条目非法 → 错误（`upstream` 类），避免把白名单静默清空。

### D2: roster 代码去留 —— 推荐 A1（删除白名单用途），备选 A2

- **A1（推荐）**：删除 `mirasim_roster.go` 及其单测、删除 `MirasimModelRoster` 相关的
  metadata 方法与 spec requirement。理由：删除后 `mirasim_roster.go` 无任何消费者，属死代码；
  仓库规范不接受长期悬空代码。`/v1/model-roster` 通道本身可同时移除（只读元数据只剩
  `/v1/models` 与 `/v1/limits`）。
- **A2（备选）**：保留 roster 解析与 `/v1/model-roster` 通道，仅把白名单来源改成
  `/v1/models`，roster 留作未来「模型目录富化」（ctx/label/pricing/effort）的输入。
  代价：一段没有调用方的代码 + 一段没有消费者的 spec requirement。
- 本变更默认按 **A1** 执行；若批准时倾向 A2，tasks 2.x 改为「保留但标注无消费者」。

### D3: 缓存与 force 语义

- `AvailableModels` 提供进程内按账号缓存（TTL 与现有 roster 一致：10 分钟），缓存键含配置
  指纹（relay/auth URL、client version、seal 公钥、设备私钥、代理任一变化即失效）。
- 显式「同步上游模型」传 `force=true` 绕过缓存（上一轮已修）；用量链路不受影响。

### D4: 不采用「同步时逐个探测」方案

- 备选方案是同步时对每个候选模型发一次极小真实请求，只保留 200 的。**不采纳**：
  - 每次点击同步都产生 N 次真实推理（计费 + 延迟 + 上游压力）；
  - 探测结果受瞬时抖动影响（实测 `kimi-k3` 曾出现一次 120s 挂起，重测 3/3 正常），
    用探测结果裁剪白名单会出现「模型时有时无」的抖动；
  - `/v1/models` 已经是权威账号维度信号，无需再探测。
- 探测能力保留在 **E2E 测试**（`//go:build e2e`）里，用于验证 `/v1/models` 与实际可调用集合
  一致，不进入运行时路径。

### D5: 生产账号 140 的可用集合必须实测核对

- 本次实测用的是桌面客户端那套凭据；账号 140 是另一套会话族，授权可能不同。
- 本变更**不**假设二者一致：发布后必须用 140 的 `POST /admin/accounts/140/models/sync-upstream`
  核对其返回集合，并与 140 的 `/v1/models` 对齐（见 tasks 6.x）。
- 若 140 的可用集合与客户端不同（例如 140 含 claude 授权），白名单会自然反映 140 的集合——
  这正是「账号维度」设计要的效果。

## Risks / Trade-offs

- `/v1/models` 与实际可调用集合存在漂移（上游声明了但调不通，或反之）→ 缓解：E2E 断言
  「列出的每个 id 都真实 200」；若未来出现漂移，由 E2E 暴露而不是靠人工发现。
- 删除 roster 会移除上一变更的部分实现与单测 → 缓解：`/v1/limits` 与用量链路完全不动；
  roster 解析逻辑在 git 历史与 `add-mirasim-model-roster-and-limits` 变更包中可追溯。
- `deepseek-v4-flash-vision-exp` 这类名字带 `exp` 的模型可能随时下线 → 无需特殊处理，
  它只是 `/v1/models` 的当前输出；下次同步自动跟随。
- 同步结果变小（5 个 id 而非 14/17）：这是期望行为，但需要在发布记录里写清，
  避免被误读为「功能退化」。

## Migration Plan

1. 代码：新增 `/v1/models` 通道与解析；`upstream_models.go` mirasim 分支改源；按 D2(A1) 移除
   roster 白名单用途。
2. 单测：解析/去重/排序/空列表/错误映射/脱敏/`force` 语义。
3. E2E（`-tags e2e`，需真实凭据）：断言 `/v1/models` 每个 id 真实可调用。
4. 前端：无改动，跑既有回归确认白名单组件不受影响。
5. 发布：前端无改动 → 只发后端（同容器热替换 + 二进制备份）。
6. 发布后核验：账号 140 的 sync 接口返回集合；确认不再出现 claude/gpt 那批 404 模型。

## Open Questions

- 是否需要把一个**服务端可选白名单**（例如 `extra.mirasim_models`）叠加在 `/v1/models` 之上，
  以便运营侧主动收窄（例如暂时屏蔽 `*-exp`）？当前范围不做，若需要可另开变更。
