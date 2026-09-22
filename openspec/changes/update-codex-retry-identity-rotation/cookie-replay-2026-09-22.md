# 十六、上游会话 Cookie 的采集与回放（2026-09-22 实施）

## 16.1 为什么做

出站相对真实客户端只剩一个稳定缺失的头：**cookie**。真实客户端 **123/123** 都发它，
网关此前一个都不发。

抓包实证的机制（2026-09-22 全量）：

| 证据 | 内容 |
|---|---|
| 上游在 codex 端点下发 | `__oailb`（**Max-Age=3600**，OpenAI 侧负载均衡亲和）、`__cf_bm`（CF bot mgmt，Expires ≈30 分钟） |
| 真实客户端携带 | `__cf_bm` + `__cflb` + `__oailb` + `_cfuvid`（120/123 全带） |
| **亲和性丢失当场重签** | 没带 `__oailb` 的 2 次请求，响应立刻下发 `Set-Cookie: __oailb`；带着它的 116 次响应都不再下发 |

⇒ cookie 是**出口亲和令牌**，不是可选项。

## 16.2 实现

| 环节 | 做法 |
|---|---|
| 采集点 | 与票同址（`captureOpenAICodexTurnStateForPool`），**且只在拿到可复用票时采集**——票有效 = 该轮被上游接受 |
| 存储 | **账号级**（会话是账号的，不是池的）：`openai_codex_turn_state_cookies` + `openai_codex_turn_state_cookie_egress` |
| 出口绑定 | 存**出口指纹**（`sha256(协议://主机:端口)` 前 8 字节）。出口变了就不回放——拿旧出口的亲和去新出口用只会制造新的不一致。**只落指纹，不落代理地址**（URL 可能含凭据） |
| 注入点 | 与票同一处（`applyOpenAICodexTurnStateReuseForModel`）+ 采票探针（`applyCodexTurnStateProbeIdentity`），两处共用一份状态 |
| 寿命 | **按上游给的**：有 Max-Age/Expires 就用；没有则 30 分钟兜底；上限 60 分钟（与票 TTL 同量级——票过期后单独回放更老的 cookie 没有依据） |
| 删除 | Max-Age≤0、空值、Expires 已过 三种删除写法都要认 |
| 失效 | `invalidateOpenAICodexTurnState` **连带清 cookie**：失效的语义是"这个会话不要了"，只换票不换后端等于继续钉在已饱和的后端上 |
| 日志 | 只有计数（`seeded ... cookies=3`），**绝不记值** |

**安全边界**：只回放**上游签发给同一账号**的 cookie。绝不转发用户端 cookie——
那属于用户自己的 ChatGPT 登录态，既跨账号也泄露凭据。

**已知边界**：cookie 不单独决定票形态。抓包里同一个 `__oailb` 值既产出过 54 次 312，
也产出过唯一那条 292。所以回放 cookie 是"保持一致"，不是"保证拿到目标池"。

## 16.3 生产验证（0.1.391-cookie-c5c049263 @18354）

```
采票：  seeded account=69 ... pool=gpt-5.6-luna egress=direct cookies=3    ← 采到 3 个
       probe_no_ticket 0 次、HTTP 400 0 次
落库：  69  → 715 字节 cookie JSON，出口指纹 5ee7a93798f061c8
       120 → 718 字节 cookie JSON，出口指纹 5ee7a93798f061c8（两号同为直连，指纹一致）
流量：  /responses {200: 146, 101: 1}    ← 零 4xx/5xx
安全：  日志中含 cookie 值的行数 = 0
健康：  running/healthy，重启 0
```

**测试覆盖**（线上线级，不只看代码）：探针路径断言第二条探针真的带上 `Cookie`；
业务路径（`Forward`）断言票与两个 cookie 同时出现在出站请求上，且**过期后不再回放**。

## 16.4 过程中修掉的两个自身错误（留档）

1. **删除型 Set-Cookie 被空值守卫拦掉**：删除写法 `__cf_bm=; Max-Age=0` 的值本来就是空的，
   我先判空值再判删除，于是条目留在列表里、网关会继续回放一个上游已作废的令牌。
   已改为「非正 Max-Age / 空值 / Expires 已过」三种都当删除，且在任何取值过滤之前判定。
2. **测试用 map 字面量构造响应头**：`http.Header` 字面量不规范化键名，
   小写 `x-codex-turn-state` 用 `Get` 查不到，测试误判"上游没发票"。
   真实响应由 Go 的 HTTP 客户端规范化写回，生产无此问题。改用 `Set` 构造。

## 16.5 容器收敛

`ageprobe-493602d0c` 已停止，当前保留 **2 个**：线上 `cookie-c5c049263` @18354
+ 上一版回滚目标 `ticketalign2-54a87a938` @18353（nginx 备份 `bak-cookie-*`）。
仍存在"回滚容器同样在跑采票器"的竞争（同一 DB），只是从 9 个收敛到 2 个。
