# 二十二、采票静默全灭 19 小时：上游改了票的形态，我们的长度上限拒掉了每一张

## 22.1 现象与问题

「打票不是自动的吗，为啥会过期很久都不打？」

查证结论：**采票一直在自动跑**（每 2 分钟一轮、6 个模型全试），但**每一张票都被我们自己丢掉了**。

## 22.2 证据链

**第一步：采票确实在跑，但全部失败。**

```
19:14:02  refresh need=2 onlyID=0 ids=[120 136]        ← 扫描到并触发
19:14:13  probe_no_ticket account=120 model=gpt-6-astra status=200 completed=true
19:16:26  refresh need=2 onlyID=0 ids=[120 136]        ← 2 分钟后又一轮，仍失败
```

近 90 分钟**全服务器零成功**（acct 容器跑了 13 小时、1885 条全失败，0 成功）。

**第二步：最后一次成功是什么时候、由哪个版本采的。**

| 容器（都是含新探针的版本） | 最后一次成功 |
|---|---|
| routepair3 | 2026-09-22 **23:12** |
| ageprobe / overloadepoch | 2026-09-22 19:14 / 18:08 |
| 今天所有容器（acct / ticketui / ticketfix） | **零成功** |

⇒ 断点在**昨晚 23:12 之后**，而昨晚的成功恰恰来自**改过探针的新版本** ⇒ **不是探针改动导致的**。

**第三步：补上诊断盲区，一次定位。**

原失败日志只有 `status/completed`，**无法区分「上游没发」与「发了但被我们拒掉」**。
补 `raw_ticket_len` 与 `verdict` 后第一轮就给出答案：

```
probe_no_ticket account=120 model=gpt-6-astra status=200 completed=true
  raw_ticket_len=780        ← 上游**发了**票，长度 780
```

而校验窗口是 `[200, 400]`：

```go
openAICodexTurnStateMinLen = 200
openAICodexTurnStateMaxLen = 400   // ← 780 > 400，每一张都被拒
```

## 22.3 影响

**约 19 小时（2026-09-22 23:12 → 2026-09-23 19:21）内，网关一张票都没能采到**，
所有 Codex 流量在无票状态下回放（丢失池位钉住与跨请求身份轮换的票侧效果）。
面板上的「已清理」长期不消失，正是这件事的表象。

## 22.4 修复与验证

- 上限 `400 → 4096`：该窗口的职责是「别把错误页当票」的兜底，**不是形态白名单**。
  注释里写明了实测形态（292/312/332/356 → 780）。
- 失败日志加 `verdict`（`upstream_sent_none` / `rejected_shape` / `incomplete_stream`）+ `raw_ticket_len`，
  这两类失败从此不可能再长得一样。
- 单测钉住：历史形态、新形态 780、两端边界、前缀与空白。

**上线后第一轮即恢复**：

```
19:21:56 seeded account=120 requested_model=gpt-5.5      pool=gpt-5.5      len=780 cookies=3
19:21:58 seeded account=120 requested_model=gpt-5.6-sol  pool=gpt-5.6-sol  len=780
19:22:21 seeded account=136 requested_model=gpt-6-astra  pool=gpt-6-astra  len=780
（账号 120 随即变为：活票位 5、单值票长 780）
```

## 22.5 连带作废的结论（重要）

第十七～十九章关于「形态 ↔ 池」的实测（`astra↔292`、`luna/sol/terra↔312`、`332/356`）
**只对 2026-09-22 的旧形态成立**。上游已把形态改到 780，那些映射**不再适用**；
面板对 292 的高亮同理不再触发（不会误报，但也不再指示任何池）。

**任何依赖票长度做判断的逻辑，都必须接受上游随时改形态这一前提。**
