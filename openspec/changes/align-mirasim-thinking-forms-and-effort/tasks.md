# Tasks

- [x] 1.1 `mirasim_thinking.go`：形态枚举 + effort 阶梯 + 归一（ultra→max、预算↔档位、400 边界）
- [x] 1.2 `mirasim_roster.go`：解析 signed roster 的 `adaptive` 显式标记（agents 覆盖 models、家族前缀校验、kimi-code/k3 映射）
- [x] 1.3 运行期客户端：`fetchModelRoster` + 缓存 + `modelThinkingShape`（只读缓存、在途去重）
- [x] 1.4 元数据路径触发 roster 刷新（失败只记 debug，不影响调用方）
- [x] 1.5 messages 管线挂点（映射后用最终模型名；无控制则原样放行）+ handler 400 通道
- [x] 2.1 单测：effort 阶梯矩阵、DeepSeek off、GLM/Kimi off 拒绝、预算型预算夹紧、自适应型折档、无控制放行、形态未知默认自适应
- [x] 2.2 单测：roster 解析（显式标记、agents 覆盖、前缀丢弃、字符串 adaptive 不算显式）
- [x] 2.3 既有元数据契约测试同步（新增 `/v1/model-roster` 只读调用）
- [x] 3.1 `go build ./...`；隔离环境 `-tags unit` vet/test；基线失败集合一致
- [x] 3.2 `openspec validate align-mirasim-thinking-forms-and-effort --strict`
- [ ] 3.3 生产发布 + 真实核验（relay 请求 200；无新增 400/forward_failed）
