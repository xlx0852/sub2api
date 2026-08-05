## 1. Replay assembly

- [x] 1.1 让 completed/done terminal output 覆盖 output_item.done 候选集合
- [x] 1.2 为乱序、重复和半成品事件补状态机测试

## 2. Replay adaptation

- [x] 2.1 实现只处理 collector 输出的标准 Responses replay adapter
- [x] 2.2 保留 call_id、移除非法私有 id，并校验 call/output 完整性
- [x] 2.3 接入 WS 与 HTTP bridge replay 合并路径，移除客户端 input 全量改写

## 3. Indexed repair

- [x] 3.1 扩展普通 Responses HTTP rejected-field retry，支持 arguments/id 精确索引
- [x] 3.2 限制每个请求每种修复最多一次，并补长 input 回归测试

## 4. Verification

- [x] 4.1 补历史故障 fixtures、重复 ID 与客户端 payload 不变测试
- [x] 4.2 执行 service 定向测试、race 测试、全量后端测试和 diff check
- [x] 4.3 整理灰度指标、开关和回滚步骤

## 5. Call/output pairing hardening

- [x] 5.1 在 replay 合并边界对齐同一 call_id 的混合 call/output 方言
- [x] 5.2 为明确的 No tool call found 错误增加单次定点修复，拒绝伪造孤儿调用
- [x] 5.3 补线上 call_id fixture、HTTP bridge 端到端重试和误修防护测试

## 6. Codex CLI contract verification

- [x] 6.1 对照官方 Codex CLI 源码确认 custom tool output 原样回传 call_id
- [x] 6.2 保留客户端首发类型与 call_id，移除按工具名猜测映射
- [x] 6.3 支持上游显式拒绝时唯一 call_/fc_ 前缀别名配对重试
- [ ] 6.4 部署后观测真实 Codex 多轮 HTTP bridge 会话
