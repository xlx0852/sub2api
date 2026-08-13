# Change: 重构为模块化单体与前后端契约分离架构

## Why

`backend/internal/service` 已膨胀为约 420 个非测试文件 / 17.6 万行的上帝包，且 repository 反向 import service 实现其内定义的接口，形成"拆包死锁"；六个上游平台族各自复制「选账号 → 改写 → 转发 → 解析流 → 计费」垂直链路，存在两套调度器、多份同构流式实现与 1000+ 行巨型函数，新平台接入只能继续复制粘贴。前端存在万行级上帝组件与账号表单双份维护。需要一份不缩减功能的目标架构蓝图与可分阶段落地的迁移路线。

## What Changes

- 后端依赖方向重建：新增 `domain` 与 `ports` 层，Repository/Cache 接口下沉，`repository` 不再 import `service`，打破拆包死锁。
- `service` 按领域拆分为 `app` 模块（gateway、scheduling、billing、quota、account、moderation、ops、payment、notification 等）。
- 网关收敛为统一生命周期管道（parse → admission → select → rewrite → transport → relay → settle）+ 显式 provider 适配器；**全部六个平台迁入统一管道，旧转发链路删除**。
- 合并两套账号调度器为单一 `AccountSelector` + 策略插件；合并两套 `RecordUsage` 为单一结算路径。
- 装配治理：模块级 Wire ProviderSet、Lifecycle 注册表替代手工 cleanup 列表、全局 setter 收归注入、合并双中间件目录、Config 按模块拆分（YAML/env 兼容不变）。
- 前后端契约分离：前端构建为环境无关产物、公开配置端点取代 HTML 注入、控制面 API 走 OpenAPI 契约并生成 TS 客户端；`-tags embed` 退化为可选静态打包，默认交付形态（一键安装、单容器 compose、后台在线升级）不变，新增分离部署选项（compose 变体 + nginx 参考配置）。
- 前端 `features/` 模块化、账号表单引擎（平台字段 registry 供 Create/Edit/Bulk 共用）、`SettingsView` 拆为懒加载子路由。
- CI 边界防回退：depguard 依赖规则 + 架构测试。
- 本 change 为架构蓝图与 P0–P8 迁移路线；各阶段实现按独立提交推进，全程行为保持。

## Impact

- Affected specs: `architecture-governance`（新增）、`frontend-backend-separation`（新增）
- Affected code: `backend/internal/*` 全域（分阶段结构性搬移）、`frontend/src/*`、`deploy/`（仅新增分离变体，默认部署不变）
- Compatibility: 所有对外协议行为、调度/计费语义、配置格式、默认部署与升级形态保持不变；无部署 breaking
