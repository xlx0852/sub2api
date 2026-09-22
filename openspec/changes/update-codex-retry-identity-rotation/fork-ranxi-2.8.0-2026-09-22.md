# 十四、对照 ranxi2001/sub2api v2.8.0 的实现（2026-09-22）

上游发布：`v2.8.0`（2026-09-22T07:59:24Z），97 个文件改动，核心是把「票」重构为
**绑定采票出口 + 会话身份**的一等对象，并**把上游下发的 Cookie 冻结 240 秒注入业务请求**。

## 14.1 它的设计（读源码与自带测试所得）

**票对象** `openAICodexTicket`：`State`(turn-state)、`Length`(形态)、`CapturedAt`/`ExpiresAt`、
`HarvestCookies []string` + `HarvestCookiesAt`、`HarvestSessionID`、
`HarvestProxyURL`/`HarvestNodeID`/`HarvestNodeName`（出口绑定）、`Model`。

**Cookie 窗口**（`openai_codex_ticket_cookie_test.go` 断言）：`codexTicketCookiesFresh` 在
**239 秒为真、240 秒为假**；注入点为 `restoreBoundCodexTicketHarvestIdentity` →
`h.Set("Cookie", strings.Join(ticket.HarvestCookies, "; "))`，过期则 `h.Del("Cookie")`。
采集来源是**响应头**：`responseCookiePairs(resp)` 取 `resp.Cookies()`。

**Cookie 刷新规则**：只有**被接受**的响应能刷新——同 state（被接受）→ 把新 `Set-Cookie`
**合并**进冻结列表；312/形态不符（被拒）→ **保留原冻结 cookie 不动**。

**出口绑定**：`pinCodexTicketEgress` / `stickBoundCodexTicketRequest` /
`loadCodexTicketDirectedSidecar`（Mihomo 定向节点 + 节点指纹校验），业务请求必须复用采票出口。

**身份钉住**：`session_id` 头设为采票会话；删除「外来身份头」与「探针从不发的头」、
删除请求体三字段。

| 清单 | 内容 |
|---|---|
| 外来身份头（删） | `session-id` `conversation_id` `installation_id` `x-codex-installation-id` `thread_id` `thread-id` `turn_id` `turn-id` `window_id` `x-codex-window-id` `x-client-request-id` `x-codex-turn-metadata` |
| 探针从不发（删） | `x-codex-beta-features` **`x-codex-routing-hint`** `accept-language` responses-lite |
| 请求体（删） | `prompt_cache_key` `client_metadata` `device_id` |

**它自己的免责声明**（重要）：
- 「原作者的 30 分钟实测属于其报告，**本分支尚未复现**」
- 「292/312 只是 `x-codex-turn-state` 的形态，**不能单独证明模型质量**」
- 「**240 秒指本版 Cookie 注入窗口，并非对上游票据寿命或模型质量的保证**」

## 14.2 我用自己抓包独立验证它的机制

**成立的部分**：

1. **上游确实在 codex 端点下发 Cookie**：`__oailb`（**Max-Age=3600**）16 次、
   `__cf_bm` 35 次（`/models` 46 次、`/responses` 5 次）。→ 采票就能拿到。
2. **真实客户端携带 LB 亲和 cookie**：`__cflb`+`__oailb`+`__cf_bm`+`_cfuvid` 共 120/123 次。
   其中 **`__cflb` 正是它测试里用的名字**。
3. **重钉可观测**：缺 `__oailb` 的 2 次请求，响应立即下发 `Set-Cookie: __oailb`；
   带着它的 116 次响应都不再下发。→ **cookie 是亲和令牌，丢失即被重新分配**。

**不成立/需修正的部分**：

4. **同一个亲和值同时产出两种形态**：`__oailb=1749066b` 既产出 **54 次 312**，
   又产出**唯一那条 292**。→ **LB cookie 不单独决定池/票形态**，
   它的模型（钉住即钉住目标形态）在我的数据上不成立。
5. **「240 秒」是它的实现选择**，不是上游给的寿命：`__oailb` 的 `Max-Age=3600`（1 小时）。
   社区流传的「原本 1 小时」正好对上这个 Max-Age。

## 14.3 与真实客户端抓包的冲突点

它的业务流量会**删掉真实客户端 123/123 全都在发的头**：
`session-id`、`thread-id`、`x-client-request-id`、`x-codex-turn-metadata`、`x-codex-window-id`、
`x-codex-beta-features`、`x-codex-routing-hint`、responses-lite、`accept-language`；
同时它的探针**自造了真实客户端从不发的下划线 `session_id`**（实测 **0/123**，连字符
`session-id` 才是 123/123）。

⇒ **它是拿「客户端像不像」换「会话自洽」**，我方策略相反（镜像真实客户端）。
两条路都没被任何一方实测证明更优，且它自己声明核心收益未复现。

## 14.4 我方（sub2api 0.1.391）对照出的真实缺陷

| # | 缺陷 | 证据 | 影响 |
|---|---|---|---|
| 1 | **采票出口 ≠ 业务出口** | 采票用裸 `&http.Client{}`（`openai_codex_turn_state.go:709`，**直连服务器 74.82.195.84**）；业务用 `s.httpUpstream.Do(req, proxyURL, ...)`，`proxyURL = account.Proxy.URL()` | 有代理的号（98/102/111）**采票与回放不同 IP**；`__cf_bm` 这类绑定 IP 的 cookie 必然不一致 |
| 2 | **完全不采集/不注入 Cookie** | 探针拿到 `result.headers` 却只取 turn-state，`Set-Cookie` 被丢弃；业务出站唯一缺失的头就是 `cookie` | 无 LB 亲和 → 上游可能每次当新会话处理 |
| 3 | **探针身份 ≠ 业务身份** | 探针仅 6 个头且 `originator=codex_cli_rs` + 裸 UA；业务 19 个头、镜像真实客户端 | 极简 CLI 探针采到的票，被"全副武装"的业务请求回放 |
| 4 | 请求体形态不一致 | 探针体极简；业务体原样转发客户端（含 `prompt_cache_key`/`client_metadata`） | 同上 |

（账号 120 无代理，采票与业务同为直连，**出口一致**；缺陷 1 只影响配了代理的号。）

## 14.5 建议（按证据强度排序）

1. **把采票出口对齐业务出口**（最高性价比、最低风险）：采票改走
   `s.httpUpstream.Do(req, proxyURL, account.ID, ...)`，与业务同一出口。
   —— 该分支正是这么修的；我方是明显遗漏。
2. **把探针头身份对齐业务身份**：探针复用账号镜像身份与真实客户端头集合
   （我方已实现 `codexIdentityCandidateUA` 与真实 19 头集合，接上去即可）。
3. **采集并注入上游签发的 LB cookie**（有界窗口），**前置条件是 1 已完成**：
   否则 cookie 的 IP 绑定与回放出口不符，注入反而制造不一致。
4. **不要照搬它的头/体剔除清单**：会删掉真实客户端 123/123 全发的 5 个头，
   属于反方向的取舍；除非整体切换到它的"探针同一性"模型。

**警惕**：其核心收益（钉住 292/332 提升质量）**连它自己都未复现**，
且我的数据显示同一亲和值也能出 312。故 1、2 应作为**一致性修复**（无副作用）先做，
3 应作为**带度量的实验**（观察出站是否开始更多拿到 292/332 形态）。
