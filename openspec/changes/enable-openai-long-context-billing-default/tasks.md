## 1. 默认语义与回归

- [x] 1.1 更新账号实体测试：OpenAI 缺失/无效值默认 true，显式 false/true 原样生效，非 OpenAI false。
- [x] 1.2 修改 `IsOpenAILongContextBillingEnabled` 的缺省语义和注释。
- [x] 1.3 更新 OpenAI RecordUsage 测试，覆盖 GPT-5.6 缺省启用和显式关闭。

## 2. 计费边界验证

- [x] 2.1 验证 272000 不触发、272001 触发整单输入 2x/输出 1.5x。
- [x] 2.2 验证 input/cache-read/cache-write 共同参与阈值且均应用输入倍率。
- [x] 2.3 验证无长上下文元数据、非 OpenAI provider、显式 false 和区间价格路径不变。

## 3. 门禁与发布

- [x] 3.1 运行 domain、billing、OpenAI RecordUsage、HTTP/SSE/WS 定向测试和 race。
- [x] 3.2 运行 backend 编译、`git diff --check` 和 OpenSpec strict validation。
- [ ] 3.3 以明确文件集合提交，不混入 Vertex 或其他 dirty WIP。
- [x] 3.4 backend-only staging 验证普通/长上下文费用与 rollback。
- [ ] 3.5 蓝绿发布生产并观察长上下文 usage 对账。

## Implementation notes

- 2026-08-17：API `0.1.326` 已以 backend-only 蓝绿方式发布到美国生产 `18251`，`0.1.325`/`18250` 作为 rollback。staging 候选的健康、版本、未登录 401 和日志门禁通过，候选移除后 staging 原服务健康。生产切换后 Nginx、public health/version、容器 `healthy`/restart=0 和错误日志门禁通过。截止短观察窗结束，切换后尚无 >272K 的 GPT-5.6 实际 usage 样本，因此 3.5 保持未勾选，不把切换前的请求当作新版对账证据。
