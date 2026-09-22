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

# 十七、票形态/落池的可测量化与首个基线（2026-09-22）

## 17.1 为什么需要它

"探针长得像真实客户端、还带 cookie"到底有没有让路由变好，此前**无法回答**：
`seeded` 日志只有 `requested_model` 与 `pool`，缺**形态**。而形态与池并非一一对应
（实测 292 ↔ {gpt-5.5, gpt-6-astra}，312 ↔ {luna, terra}，332 也出现过在 luna），
所以三个字段必须在同一行。

已加：`seeded account=… requested_model=… pool=… len=… egress=… cookies=…`

## 17.2 首个生产样本（19:55，探针已带 cookie）

```
requested_model=gpt-6-astra  pool=gpt-5.6-luna   len=312  cookies=3
requested_model=gpt-5.5      pool=gpt-5.5        len=292  cookies=3
requested_model=gpt-5.6-sol  pool=gpt-5.6-sol    len=312  cookies=3
requested_model=gpt-5.6-terra pool=gpt-5.6-terra len=312  cookies=3
```

`cookies=3` 说明探针确实带着会话亲和；**astra 请求同样被降级到 luna**。

## 17.3 基线：真实客户端（归档抓包 12 小时 / 123 条 /responses）

```
请求模型 → 实际落池                    票长分布
gpt-6-astra → gpt-5.6-luna  101 (82.1%)   len=312  122 (99.2%)
gpt-5.6-terra → gpt-5.6-terra 16 (13.0%)  len=292    1 ( 0.8%)
(空)        → gpt-5.6-luna    3 ( 2.4%)
gpt-5.6-luna → gpt-5.6-luna   2 ( 1.6%)
gpt-6-astra → gpt-6-astra     1 ( 0.8%)

**请求 astra 且真的落在 astra：1/102 (1.0%)**
```

### 这个基线的意义（推翻了流传的前提）

**连带全套 cookie、真实 UA、合法 session 的官方客户端，请求 astra 102 次也只拿到 1 次。**
82.1% 被降级到 luna。也就是说：

- 社区疯抢的 292/astra **不是"钉住就能拿到"的资源**，它本身只有约 **1%** 自然到达率；
- 那个 fork 宣称的目标"钉住 292/332"**在 1% 的自然概率下无法用少量样本验证**，
  这也解释了它为何明确写"本分支尚未复现"；
- 我方探针的表现（astra→luna）与真实客户端**一致**，即当前改动没有让路由变差，
  但也没有魔法可让 astra 变得可得。

## 17.4 可复现工具

`scripts/codex_ticket_shape_report.py`（本机脚本，`scripts/` 已 gitignore）：

```
scripts/codex_ticket_shape_report.py                 # 探针(生产) vs 真实客户端(归档基线)
scripts/codex_ticket_shape_report.py --since 2h      # 指定窗口
scripts/codex_ticket_shape_report.py --baseline-only  # 只出基线
```

它把两侧归一成同一口径（请求模型 / 落池 / 票长）并直接给出
**"请求 astra 且真的落在 astra"** 的命中率——这正是判定"像不像客户端值不值得"的那个数。

## 17.5 累积样本对比（探针 9 条 / 3 小时 vs 客户端 123 条 / 12 小时）

| 维度 | 探针（已带 cookie） | 真实客户端基线 |
|---|---|---|
| 请求 astra 的落池 | **luna 3/3** | luna 82.1%（astra 命中 1.0%） |
| 请求 5.5 的落池 | **gpt-5.5 2/2** | — |
| 请求 terra 的落池 | terra 2/2 | terra 13.0% |
| 票长 312 | 77.8% | 99.2% |
| 票长 292 | **22.2%** | 0.8% |

探针样本仅 9 条，**不足以定论**；但方向清楚：
**探针在 astra 上的被降级行为与真实客户端一致**（都落 luna），说明本次身份对齐没有让路由变差。

## 17.6 顺带发现的更重要事实：292 不来自 astra

把三处证据并起来看：

| 来源 | 观察 |
|---|---|
| 探针（17.2） | `requested_model=gpt-5.5 → pool=gpt-5.5 → len=292` |
| 本次 DB 快照 | 账号 120 全部池槽位 312；账号 132 多为 332、一条 356；账号 133 为 332/356 |
| 更早的生产记录 | 账号 69/111 的 `gpt-5.5` 槽位都是 **292** |

⇒ **292 形态稳定地出现在 `gpt-5.5` 池**；`gpt-6-astra` 反而基本拿不到（82~100% 降级到 luna）。
社区把 292 与 astra 绑定，是因为二者都属于**同一档（高优先）池**，而不是同一个池。

**可行动的结论**：若目标是 292/332 这类形态，**该盯的是 `gpt-5.5`**（探针请求即稳定拿到 292），
不是几乎拿不到的 `gpt-6-astra`。这一点此前无人明确指出，是用我们自己的数据得到的。
