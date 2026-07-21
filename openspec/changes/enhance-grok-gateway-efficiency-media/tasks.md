## 1. Grok 请求体优化

- [x] 1.1 增加 `gateway.grok_payload` 配置、默认值和校验
- [x] 1.2 实现 Responses/Chat 图片检测、精确去重及 `store=false`
- [x] 1.3 实现软预算触发的历史工具输出安全截断
- [x] 1.4 实现硬预算 413 错误并接入全部 Grok HTTP 转发路径
- [x] 1.5 增加优化行为、边界和非 Grok 隔离测试

## 2. Grok 额度软调度

- [x] 2.1 从新鲜 billing/API quota 快照解析 Grok headroom
- [x] 2.2 接入现有 `quota_headroom` scheduler 权重并保持 sticky/硬排除优先级
- [x] 2.3 在 scheduler metadata 中保留必要 Grok quota 字段
- [x] 2.4 将 Grok 额度观测更新接入 scheduler-neutral 单账号刷新
- [x] 2.5 增加新鲜、陈旧、缺失、耗尽及 sticky 行为测试

## 3. Grok 视频编辑

- [x] 3.1 增加 `/v1/videos/edits` 与 `/videos/edits` 路由及 endpoint 归一化
- [x] 3.2 增加 Grok media endpoint、上游 URL 和 handler 分发
- [x] 3.3 复用审核、计费、failover 与异步 request ID 账号粘连
- [x] 3.4 增加路由、URL、透传、计费和粘连测试

## 4. 验证

- [x] 4.1 运行 gofmt 和 OpenSpec strict validation
- [x] 4.2 运行 config、xai、repository、handler、routes、service 聚焦测试
- [x] 4.3 运行受影响包完整测试并记录既存失败
- [x] 4.4 运行 `git diff --check`
