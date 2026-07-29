# Change: 增加远程模型目录安全刷新

## Why
当前第一方模型目录只能从本地文件或二进制内嵌副本加载，新增模型、调整默认映射或更新展示元数据都需要重新构建或重启服务。项目已经具备账号级上游模型探测和远程价格数据更新，但缺少一个可校验、可降级、可热更新的公共模型目录分发链路。

## What Changes
- 为 `modelcatalog` 增加可配置的远程目录、校验和、本地持久化与内嵌回退加载链
- 服务启动后异步检查远端目录，并按配置周期刷新，不阻塞启动和网关请求
- 对远端内容执行大小、JSON 结构、业务约束、版本和 SHA-256 校验，失败时保留最后可用快照
- 原子替换内存目录，并按变化类型失效模型目录、模型列表、映射和定价派生缓存
- 清理包初始化阶段固化的模型目录快照，使模型、默认值和映射读取能够响应热更新
- 提供管理员状态查询和手动刷新接口，展示来源、版本、哈希、检查时间、成功时间和安全错误摘要
- 保持账号级 `/v1/models` / `fetchAvailableModels` 同步、租户配置、渠道定价和应用版本升级链路不变

## Impact
- Affected specs: `remote-model-catalog`
- Affected code: `backend/internal/pkg/modelcatalog`, 配置加载、服务生命周期、模型目录消费者、模型/定价缓存、管理员路由与前端管理入口
- External data source: `https://github.com/xlx0852/model-catalog`（默认 Raw URL，可由配置替换为任意受信 HTTPS/CDN 源）
- Database: 无 schema 变更
