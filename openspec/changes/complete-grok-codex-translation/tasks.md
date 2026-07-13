## 1. Tests First

- [x] 1.1 增加 Grok 请求侧 custom/apply_patch 工具、tool_choice 和历史调用转换测试
- [x] 1.2 增加 Grok 非流式 function_call → custom_tool_call 回程测试
- [x] 1.3 增加 Grok SSE custom 工具完整生命周期回程测试
- [x] 1.4 增加普通 function 工具和非 Grok 路径不受影响的回归测试

## 2. Implementation

- [x] 2.1 建立 request-scoped Grok custom 工具翻译上下文
- [x] 2.2 实现 custom 工具定义、tool_choice、调用历史与输出的请求侧转换
- [x] 2.3 实现非流式 Grok 响应的 custom 工具逆向还原
- [x] 2.4 在现有 SSE 帧处理链中实现 custom 工具事件逆向还原
- [x] 2.5 清理 `apply_patch` 特殊丢弃逻辑并保持 unsupported tool 过滤行为

## 3. Verification

- [x] 3.1 运行 Grok 定向 unit tests
- [x] 3.2 运行 `internal/pkg/apicompat` 测试
- [x] 3.3 运行受影响的 `internal/service` 回归测试并记录结果
