## 1. 统一与清理
- [x] 1.1 锁定跨平台 priority/load/tie、硬门槛、busy affinity、compact、continuation 回归。
- [x] 1.2 OpenAI-compatible 路径移除 opt-in 和旧评分主体，全部使用简化排序。
- [x] 1.3 Gateway 与 Gemini 兼容路径接入共享排序并保留平台副作用边界。
- [x] 1.4 移除旧 scoring/TopK/shadow 执行代码，更新失效的旧行为测试。
- [x] 1.5 移除调参 UI/伪总分，兼容历史设置但不允许恢复旧选号路径。
## 2. 验证与发布
- [x] 2.1 相关平台/handler/工具续链/compact 回归、race、build/vet、前端验证、archtest、strict。
- [x] 2.2 隔离构建候选，保留回滚，蓝绿上线；真实可用供给与未实测平台明确分开记录。
