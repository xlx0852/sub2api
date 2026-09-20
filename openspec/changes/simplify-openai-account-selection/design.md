# Design

## 单一规则与兼容边界

保留共享 Engine 和现有槽位/复核/等待出口。简化 profile 按正整数 group ID 列表启用；OpenAI、非 compact 才使用。未启用 scope 的评分参数与排序不变，失效钉选 bug fix 独立生效。

在 profile 内统一检查排除集、平台、组、schedulable-for-model、provider quota、运行时封锁、shadow parent、模型映射、渠道、端点/图片能力、transport、privacy。绑定是过滤后集合中的优先尝试 ID，不是新的硬门槛。对普通绑定统一 latest-state/epoch 复核；busy 则继续其他候选，全部 busy 后才生成有界等待计划。

## 排序

priority 小者优先，同 priority 下实时并发占用率低者优先；同档使用可注入 RNG 洗牌。缺失负载不伪装成空闲；已知负载优先，全部缺失时同优先级随机尝试，仍由原子 acquire 保证不超限。不使用 TTFT、历史 error、reset、quota usage、票、sticky bonus 或 TopK。保留这些观测/协议用途，不删除其他 profile 所需实现。

负载率使用实际 concurrency 作为分母，非管理员软 load_factor。偏好列表为 user pin、session（去重），仅对仍合格且 epoch 一致的来源生效；无效绑定清理，不能继续锁住旧凭证类。

## 真实续链和请求级隔离

有 previous_response_id 时先走既有 response→account 查找，简化路径不允许已知不可移动续链绕到普通负载均衡，也不删除/合成工具上下文。原子 acquire 之前和 DB 复核之后应用统一门槛。明确可移动的请求沿用既有转发层恢复语义。

认证 middleware 为每个请求创建隔离状态；上游 OpenAI 401/403 在错误处理层锁定本次尝试的 credential class。后续选号只允许同类；该状态不写入 Redis，不由忽视 401 开关清除，不改变失败重试预算。其他错误不新建该限制。WS context 生命周期为当前连接，连接内复用原先账号/turn 管理不变。

## 观测与回滚

记录 profile、模型、group、首选/最终 account、候选数量、class 限制来源及无候选原因，不记录 key/token/prompt/session 原文。新增回归覆盖 89 disabled → 132 eligible、绑定忙让路、原号续链、认证失败后不能跨类、同档新号轮转、DB 复核后不可用、其他平台/compact 不受影响。

删除 group ID 即回滚 profile（配置重载/重启按既有配置加载方式），数据库账号设置、旧权重和旧发布保留。本 change 不主动清空真实用户会话绑定、不把 132 写死为首选。
