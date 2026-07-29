## Context

sub2api 当前通过 `internal/pkg/modelcatalog` 在进程初始化时读取工作目录文件或内嵌 `catalog.json`。该目录已覆盖平台模型、默认映射、别名、退役 ID、fallback pricing 和 UI preset，但没有远程刷新与热更新。另有两条相邻但独立的链路：`PricingService` 下载 LiteLLM 价格数据；账号管理按凭据请求实际的上游模型列表。

新建的公共仓库 `xlx0852/model-catalog` 提供 `catalog.json`、`catalog.sha256`、版本快照和 CI 校验。运行时仍必须把远端输入视为不可信数据，不能依赖远端始终可用或正确。

## Goals / Non-Goals

- Goals:
  - 无需重启即可安全更新公共模型元数据、默认映射和 fallback pricing
  - 远端不可用、内容损坏或版本异常时继续使用最后可用目录
  - 多副本部署时避免同步风暴，并提供足够的刷新状态和错误可观测性
  - 保持模型目录、账号真实可用模型和数据库运营策略三个职责边界
- Non-Goals:
  - 不自动修改账号模型白名单、渠道定价、用户售价或租户路由策略
  - 不把远端 catalog 中出现的模型自动视为任意账号实际可调用
  - 不恢复或替代 GitHub Release 驱动的应用在线升级功能
  - 不在本变更中建立 catalog 编辑后台；目录仍通过独立仓库 PR 发布

## Decisions

- Decision: 使用“内嵌 → 本地最后可用 → 远端候选”的分层模型
  - 启动同步加载内嵌或本地快照，远端检查异步执行，避免网络阻塞启动。
  - 远端候选只有在所有校验通过后才进入内存并持久化。

- Decision: 远端源通过配置注入，GitHub Raw 只是默认值
  - 允许部署方切换到 CDN、对象存储或内部镜像。
  - 与 `remove-github-update-checks` 不冲突：后者针对应用版本和在线升级，本能力针对公共运行时数据。

- Decision: 校验和与目录使用独立请求，并在目录解析前后都限制资源消耗
  - HTTPS URL 必须经过现有 URL 安全策略或等价的严格验证。
  - 响应体设置硬上限、请求超时和有限重试；SHA-256、JSON 和业务校验全部通过才接受。
  - 错误响应体、令牌和内部网络信息不得进入管理员错误摘要。

- Decision: 使用不可变快照原子替换，不原地修改目录对象
  - `modelcatalog.Get()` 应返回只读快照或安全副本。
  - 新目录完成解析和派生索引构建后，通过短临界区替换指针。

- Decision: 先消除静态消费者，再承诺热更新
  - `openai.DefaultTestModel`、`xai.DefaultChatModel`、`domain.DefaultAntigravityModelMapping` 等启动期副本必须改为动态访问或由刷新回调更新。
  - 测试必须证明刷新后新请求看到新值，旧请求不会观察到半更新状态。

- Decision: 差异按语义分类
  - 区分模型可用性/映射、展示元数据、fallback pricing 和 UI preset 变化。
  - 仅展示字段变化不重建账号调度；模型 ID、退役状态或映射变化才失效模型可用性相关缓存。

- Decision: 自动刷新不直接覆盖数据库运营配置
  - 数据库里的账号映射、渠道定价、分组 `/v1/models` 列表继续拥有更高的运营语义优先级。

## Runtime Flow

```text
process start
  -> load local last-known-good snapshot when valid
  -> otherwise load embedded snapshot
  -> serve traffic
  -> asynchronously fetch checksum and catalog
  -> enforce URL, timeout and size policy
  -> verify SHA-256 and validate catalog
  -> reject non-increasing or disallowed version transitions
  -> compute semantic diff
  -> atomically persist and swap snapshot
  -> invalidate only affected derived caches
  -> record status and metrics
```

## Configuration

建议配置形态：

```yaml
model_catalog:
  remote_enabled: true
  remote_url: https://raw.githubusercontent.com/xlx0852/model-catalog/main/catalog.json
  hash_url: https://raw.githubusercontent.com/xlx0852/model-catalog/main/catalog.sha256
  data_dir: ./data/model-catalog
  fallback_file: ./resources/model-catalog/catalog.json
  update_interval_hours: 3
  hash_check_interval_minutes: 10
  request_timeout_seconds: 30
  max_body_bytes: 8388608
```

禁用 `remote_enabled` 时保持当前本地/内嵌行为，便于隔离部署与紧急回滚。

## Status Model

管理员状态至少包含：

- 当前版本、更新时间和 SHA-256
- 当前来源：`embedded`、`local` 或 `remote`
- 最近检查时间、最近成功时间和是否正在刷新
- 最近安全错误摘要
- 下一次计划检查时间

手动刷新与定时刷新共用 singleflight/互斥控制，避免重复下载。

## Risks / Trade-offs

- 风险: 远端错误映射影响新请求路由
  - Mitigation: 严格校验、版本快照、最后可用副本、禁用开关和快速回滚远端版本。
- 风险: 热更新后部分包继续使用启动期变量
  - Mitigation: 全量审计目录消费者，并用跨包回归测试验证动态读取。
- 风险: 多副本同时请求 GitHub Raw
  - Mitigation: 定时器加入随机抖动，支持 CDN/镜像；后续可选 Redis 协调，不作为首版依赖。
- 风险: catalog 和账号真实能力不一致
  - Mitigation: catalog 仅表达公共元数据；账号调度继续受实际账号配置与探测结果约束。
- 风险: GitHub 不可用
  - Mitigation: 不阻塞启动，保留内嵌和磁盘最后可用目录，并允许切换数据源。

## Migration Plan

1. 增加配置和严格验证器，保持远端刷新默认可安全关闭。
2. 引入快照存储、状态对象和本地最后可用目录。
3. 审计并替换启动期静态目录消费者。
4. 接入后台更新器、差异回调和精确缓存失效。
5. 增加管理员状态与手动刷新入口。
6. 在测试环境启用远端刷新并验证失败降级、回滚和多副本行为。
7. 生产启用；出现异常时关闭 `remote_enabled` 并继续使用磁盘/内嵌快照。

## Open Questions

- 首版是否在管理端页面提供手动刷新按钮，还是只提供 API 和状态展示。
- 是否要求远端版本只能严格递增，或允许通过显式 `allow_downgrade` 配置执行紧急回滚。
