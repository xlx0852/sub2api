## 1. Core convergence

- [x] 1.1 定义四档策略及兼容历史账号的 extra 语义
- [x] 1.2 实现稳定 ID 派生和头/体/内嵌 metadata 统一改写
- [x] 1.3 覆盖 root/parent/fork/thread/turn/window 关系并保留非身份字段

## 2. Transport integration

- [x] 2.1 接入标准 Responses HTTP 路径
- [x] 2.2 接入 OAuth passthrough HTTP 路径
- [x] 2.3 接入 Responses WebSocket v2 首帧和重放路径
- [x] 2.4 确保 compact 和非 OAuth/API Key 路径不被误改写

## 3. Admin UI

- [x] 3.1 新建 OpenAI OAuth 账号默认 session
- [x] 3.2 编辑和批量编辑支持四档策略
- [x] 3.3 补齐中英文风险和迁移说明

## 4. Verification

- [x] 4.1 单测覆盖四档、载体一致、父子线程及旧账号默认 off
- [x] 4.2 运行相关 Go 测试与全包编译
- [ ] 4.3 运行前端单测、类型检查与生产构建
- [x] 4.4 运行 `openspec validate add-codex-fingerprint-convergence --strict`
