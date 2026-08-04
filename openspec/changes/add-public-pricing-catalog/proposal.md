# Change: 增加匿名公开价格清单与抗请求放大缓存

## Why

当前模型广场仅允许登录用户访问，潜在用户在注册前无法了解模型覆盖和价格。直接把现有登录态接口改为公开会泄露专属分组、渠道名称和用户倍率，并让匿名请求反复查询数据库，形成 CC 攻击下的请求放大点。

## What Changes

- 在无需登录的公开首页嵌入价格清单区段，提供模型卡片、搜索和平台筛选体验。
- 新增公开价格快照接口，仅返回模型、平台、展示元数据和非专属启用分组中的最低公开有效价格。
- 不公开渠道名称、分组名称与 ID、专属分组、用户专属倍率、账号及上游信息。
- 公开接口只读取长效快照，不在匿名请求路径中逐次查询渠道、分组或上游。
- 快照采用进程内热缓存 + Redis 长效缓存；过期后单飞后台重建，重建失败时继续返回旧快照。
- 返回稳定 `ETag` 和面向 CDN 的 `Cache-Control`，支持 `304 Not Modified`，降低源站带宽与连接压力。
- 对公开接口增加独立的匿名 IP 限流；缓存未就绪且无法生成快照时快速失败，不允许匿名并发穿透数据库。
- 管理端渠道、分组、模型价格或公开开关变更后失效快照；管理员可主动重建，匿名用户不能触发强制刷新。
- 保留现有登录态模型广场语义，登录用户仍可看到自己有权限的专属分组和实际倍率。

## Impact

- Affected specs: `public-pricing-catalog`（新增能力规格）
- Affected code:
  - `backend/internal/handler/available_channel_handler.go`
  - `backend/internal/service/channel_service.go` 及公开快照服务
  - `backend/internal/server/routes/`
  - Redis 缓存与匿名限流中间件
  - `frontend/src/router/index.ts`
  - `frontend/src/views/`、`frontend/src/components/channels/`
  - 中英文 i18n 与前后端回归测试
- No database migration and no new external dependency.
