# Design: 账号可用性判定收敛

## 背景与问题模型

调度器选号前要先回答一个问题："这个账号现在能不能上？"。当前答案由 6 个分散位置拼出来：

```
候选池 = DB快照(sched:*)                        ← 异步 outbox 重建，可能空
  └─ IsSchedulable()  = active && schedulable    ← DB 永久开关
                      && !temp_unschedulable     ← DB 冷却
                      && !overload_until         ← DB 冷却
                      && !rate_limit_reset_at    ← DB 冷却
                      && !quota_exceeded         ← DB(apikey 配额)
  └─ isOpenAIAccountRuntimeBlocked()             ← 进程内存冷却(429/transport)
  └─ IsSchedulableForModel()                     ← model_rate_limits + 模型映射
  └─ hasTurnStateReserve(model)                  ← account.Extra 内存副本的票
```

故障模式：
- **内存拉黑**(`openaiAccountRuntimeBlockUntil`)不写 DB，重启即丢、多实例不一致、无审计。
- **票在 Extra**：seeder 与调度器各持一份 `*Account` 副本，写一边另一边看不到；代理断了票还在，5xx 清票误伤。
- **快照空**：`GetSnapshot` 命中空 ZSET 直接返回空，不走 DB，导致"DB 有 4 个号但报无号"。

## 决策

### D1: 单一冷却真相源 = `accounts.temp_unschedulable_until`

所有"暂时别调度这个号"都写这一个 DB 字段，配 `temp_unschedulable_reason` 记录原因。理由：
- DB 是唯一跨实例、可查询、可审计、重启不丢的存储。
- 内存拉黑的本意是"比 DB 快"，但冷却都是分钟级，DB 写入延迟可忽略；换来的一致性收益远大于那点延迟。
- 已有先例：token refresh 耗尽、401 credential 都走 `SetTempUnschedulable`，证明这条链路可用。

删除 `openaiAccountRuntimeBlockUntil` 后，`isOpenAIAccountRuntimeBlocked` 改为直接读 `account.TempUnschedulableUntil`（对象已在内存，零额外查询）。

风险：依赖调度器每次拿到最新 `account` 对象。快照缓存的 `Account` 可能滞后——但 `recheckSelectedOpenAIAccountFromDB` 已在选中路径做 DB 复核，可覆盖。

### D2: 票存 Redis `turnstate:acc:<id>:<pool>`

- key 粒度 = 池槽位，value = 票字符串，TTL = 票有效期（当前默认 60min，写票时设置）。
- 读：`MGET turnstate:acc:<id>:<pool>`；写：`SETEX`。
- 为什么不是存 `sched:acc` 快照：票生命周期独立于账号快照（票 1h、快照随 outbox 变），分开避免互相踩踏；且快照是整个账号对象，塞票会放大写。
- 为什么不是继续放 Extra：Extra 是账号静态属性的载体，票是高频刷新的会话态，混一起导致 seeder/调度器对象漂移——本次 69 号"有票但代理死了"就是这么误判的。

迁移期双读（先 Redis 后 Extra）一个版本，写入只写 Redis。

### D3: 空快照 = miss

`GetSnapshot` 返回 `(nil, false, nil)` 当 ZSET 为空，让 `ListSchedulableAccounts` 走 DB + `SetSnapshot`。这是一行级的兜底，消除"ready=1 但候选空"的悬空态。

### D4: 模型级无候选单独报错

`noAvailableOpenAISelectionError` 增加 `reason` 字段：`platform_empty` / `model_unmapped` / `all_cooled_down`。日志与返回错误区分，便于定位"是池空了还是没人映射这个模型"。

## 不做的事

- 不合并三层粘性（UserPin/SessionSticky/PreviousResponse）——它们解决不同问题，且 `promote-unified-scoring-scheduler` 统一引擎时会处理。
- 不删 `schedulable` 字段——它是人工开关，只要求写入处记审计。
- 不改打分/TopK/加权随机。

## 回滚

每步独立：拉黑合一、票迁移、快照兜底、错误分级可单独 revert。票迁移保留 Extra 读兜底一个版本，旧票仍可读。
