# 二十三、移植 OpenCode Go 用量窗口能力（对齐上游 v0.2.8）

## 23.1 需求与范围判断

上游 v0.2.8（2026-09-23 发布）新增「OpenCode Go 用量窗口：官方用量查询、自动刷新、
同 Key 组共享、主动查询，账号列表展示余额（7d/1m 徽章）」。

该能力在上游由 **21 个文件**构成，拆开是三件事：

| 上游组件 | 规模 | 本仓处置 |
|---|---|---|
| **官方用量窗口查询** | 核心约 150 行（协议已核对） | ✅ **本次移植** |
| OpenCode 一等平台（含 `zen`/`go` 模式、DB CHECK 约束） | 含独立迁移 238；平台枚举横跨后端多处 + 前端 39 文件（对照 kimi：`api/admin/kimi.ts`、平台选择器、图标、配额弹窗、i18n…） | ⏸ **不做**：见 23.2 |
| 多协议路由（同一 Key 按模型规则走 OpenAI Chat 或 Anthropic Messages） | 本仓是「一平台一协议」结构 | ⏸ **不做**（除非确需 Anthropic 协议入口） |

## 23.2 为什么可以不新增平台

`Account.GetOpenAIBaseURL()` 对 **API-key 账号**直接返回 `credentials.base_url`，
而 OpenCode Go 是 **OpenAI 兼容**上游 ⇒ 现有「API-key 账号 + 自定义 base URL」
已经能完整转发它。真正缺的只是**官方用量窗口**这一件事。

**接法**（无需新平台）：

```
platform = openai, type = apikey
credentials.base_url = https://opencode.ai/zen/go/v1
credentials.api_key  = <OpenCode Go 的 key>
```

## 23.3 实现

`internal/service/opencode_go_usage.go`（核心，约 240 行）：

- `GET https://opencode.ai/zen/go/v1/usage`，`Authorization: Bearer <api_key>`，
  UA `sub2api-opencode-go-usage/1`，**禁用重定向**，走账号自身代理。
- 解析**宽容**：窗口可能在 `{"usage":{...}}` 里，也可能直接在顶层；
  缺窗口/时间格式非法都降级为零值，只有结构性非法 JSON 才算失败。
- 窗口：`rolling` / `weekly` / `monthly`（即 release 里说的 7d/1m）。
- 失败也落库（带短原因与状态码）：面板要能区分「没查过」与「查了但失败」。

**安全边界（三条硬要求）**：

1. **host+path 精确等于官方基址**才发请求——只做前缀匹配会放进
   `https://opencode.ai.evil.example/zen/go/v1` 这类地址，等于把 key 交给别人。
   发请求前与拿到响应后各校验一次。
2. 重定向禁用。
3. 落库与响应**只含窗口数据与短原因，绝不含 API key**（有测试断言）。

**接线**：查询走 `AccountTestService.QueryOpenCodeGoUsage`（它同时具备 `HTTPUpstream`
与 `accountRepo`，且管理端 handler 已持有它）⇒ **不需要改依赖注入图**。
管理端点：`GET /api/v1/admin/accounts/:id/opencode-go-usage`；
非该类账号返回 `applicable=false`，查询失败返回 200 + 快照里的 `error`（否则"查了但失败"会不可见）。

**前端**：`OpenCodeGoUsagePanel.vue` 挂在账号详情抽屉的**额度页签**；
按 base URL 先做一次本地判定（同样拒绝伪装 host），非该类账号不发请求也不渲染；
三个窗口用百分比条展示，失败显示原因而不是隐藏。

## 23.4 验证

| 断言 | 位置 |
|---|---|
| 官方基址（含尾斜杠、`/usage`）接受；`http://`、缺段、前缀伪装、`..` 路径全部拒绝 | Go 单测 |
| 仅 OpenAI API-key 且 base URL 精确匹配才算 OpenCode Go；OAuth、别家中继、nil 均不适用 | Go 单测 |
| 宽容解析（包裹/直连、缺窗口、非法时间） | Go 单测 |
| 成功路径：请求形状正确 + **落库不含 key** | Go 单测 |
| 失败路径（401/403/429）：仍落库并带原因与状态码 | Go 单测 |
| 不适用账号**不发任何请求** | Go 单测 |
| 面板渲染三窗口、伪装 host 不发请求、失败显示原因、`applicable=false` 时隐藏 | 前端 5 例 |

**上线核验**：`/opencode-go-usage` 未鉴权返回 **401**（对照路径 404）⇒ 路由已注册；
线上容器 `running/healthy`、公网 `/health` 200；二进制含 `isOpenCodeGoUsageURL`。

## 23.5 补全：自动刷新 + 账号列表徽章

第一版只做了「按需查询」，徽章会长期陈旧，所以补齐上游 release 里的另外两件事。

**自动刷新**（`opencode_go_usage_refresh.go`）：

| 参数 | 值 | 依据 |
|---|---|---|
| 扫描周期 | 5 分钟 | = 上游的**硬下限**，保证不比官方客户端更频繁地打官方接口 |
| 陈旧阈值 | 15 分钟 | = 上游的**默认上限** |
| 失败退避 | 同样 15 分钟 | 失败也写入 `fetched_at`，401/403 不会变成每轮重试 |

与上游"活动驱动 + debounce"的差别**只在时效性**（最多晚 15 分钟），请求量不多打一次。
逐账号串行并留 400ms 间隔（官方接口是外部依赖）；启动即扫一轮，进程重启不会让用量停更一个周期。

**账号列表徽章**（`OpenCodeGoUsageCell.vue` + `AccountsView` 新列 `opencode_go`）：

- 数据直接读列表响应里 `extra.opencode_go_usage` ⇒ **每行不发请求**；
- 只显示 `7d` / `1m` 两个徽章（对齐 release 的"7d/1m 徽章"），按用量分档着色（<70 绿 / <90 黄 / ≥90 红）；
- 失败显示"用量查询失败"并把原因放进 tooltip；尚无快照显示"未查询"；
- 非 OpenCode Go 账号（含 `opencode.ai.evil.example` 这类**伪装 host**）**不渲染**。

## 23.6 验证（本轮新增）

| 断言 | 位置 |
|---|---|
| 无快照/超 15 分钟 → 需刷新；2~3 分钟前 → 不刷；**失败同样计时**；时间在未来 → 重刷；阈值非法回退默认 | Go 单测 |
| 扫描**只碰 OpenCode Go 账号**：中继账号的 key 从未被发出（断言 Authorization 的具体值） | Go 单测 |
| 徽章渲染 7d/1m、着色分档、失败 tooltip 带原因、无快照显示"未查询"、损坏 JSON 不抛错 | 前端 6 例 |
| 伪装 host / OAuth / 非该类账号不渲染 | 前端用例 |

全量：**后端 8 例 + 前端 167 例全绿**。

**上线**：`0.1.391-ocgcell-71fe371a8` @18369；端点未鉴权 401；公网 `/health` 200；
`/responses` 正常；无误报 ERROR。自动刷新循环**无日志即无工作**（当前库里还没有 OpenCode Go 账号）。

## 23.7 回退 23.5 的前端一半：移除账号列表独立用量窗口（2026-09-24）

用户要求「移除掉单独的 opencode go 用量窗口展示」。这里有两处展示，裁定移除的是**账号列表卡片内**
那一处——它与通用「用量窗口」section 并列、标题近乎重复，属「单独的展示」；**用量抽屉面板保留**（按需
实时查询，功能不丢）。提交 `7e1ee1c92`。

**删除**：

| 对象 | 文件 |
|---|---|
| 列表 cell 组件 + 单测 | `frontend/src/components/account/OpenCodeGoUsageCell.vue`、`__tests__/OpenCodeGoUsageCell.spec.ts`（`git rm`） |
| 卡片 section | `frontend/src/features/admin/views/AccountsView.vue`（`<section v-if="isColumnVisible('opencode_go')">`、import、`allColumns` 里的 `opencode_go` 列） |
| 列文案 | `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` 的 `columns.openCodeGo` / `columns.openCodeGoUsage` 对象 |

**保留**（避免误删）：`OpenCodeGoUsagePanel.vue` 及其单测、`AccountUsageDrawer`（用量 tab）、
`api/admin/accounts.ts` 的 `getOpenCodeGoUsage` 与三个类型、i18n `usageDetails.openCodeGo`。

**为何不连带动后端**：快照键 `extra.opencode_go_usage` 的消费方只剩自动刷新自身——列表徽章下线后没有
读它的人（抽屉面板走 `AccountTestService.QueryOpenCodeGoUsage` 现场查询，不读快照）；
`observePlatformQuotaWindowUpdates` 只读扁平键 `codex_5h_*`/`codex_7d_*`，嵌套 JSON 字符串不进配额窗口
ledger、不影响调度。故停用自动刷新是**独立决定**，本轮不动。

**验证**：

| 项 | 结果 |
|---|---|
| `pnpm typecheck`（`vue-tsc --noEmit`） | 通过 |
| `pnpm test:run` | 174 文件中 3 例失败，均为**既存失败**（用 `git stash push -- frontend/src` 在净树复现：`AccountsView.sparkShadow.spec.ts` 的 SuperGrok 大小写、`UserPlatformQuotaModal.spec.ts` 的 7 平台假设） |
| `pnpm build` | 成功，新 chunk `assets/AccountsView-DEtjWFwt.js` |
| 产物残留 | `OpenCodeGoUsageCell` / `opencodeGoUsage` / `weeklyShort` / `opencode_go_usage` 计数 **0**；`OpenCodeGoUsagePanel`、`getOpenCodeGoUsage` 仍在 |
| `localStorage` 僵尸键 | 无害：`loadSavedColumns()` 把 `HIDDEN_COLUMNS_KEY` 逐项塞进 `hiddenColumns` 且无校验，残留的 `opencode_go` 键不再被任何列引用 |

**上线前基线核对**：本地净树 HEAD 构建（`a33b47ef9` 源码等价，差异仅文档）与线上 `current` 目录
**逐文件 md5 完全一致**（172 文件），故本次产物是「相对线上现状仅含目标改动」的可控增量。

**上线**：新 release `0.1.391-ocgremove-7e1ee1c92`（172 文件、排除 macOS `._*`、root:root 644/755），
`rollback` → `0.1.391-noinject-a33b47ef9`，`current` → 新目录。

| 线上断言 | 结果 |
|---|---|
| 打包 md5 | `7e6cb7ecbc69c7426f3d606acb112caa`（本地/上传一致） |
| `GET /`（含公网 DNS） | 200，引用 `/assets/index-B5qfLrym.js` |
| 新入口 / AccountsView chunk | 200 / 200 |
| 被替换的旧 chunk | 404（预期，哈希文件名已废） |
| 线上返回的 `AccountsView-DEtjWFwt.js` 字节 | `OpenCodeGoUsageCell` **0**、`opencodeGoUsage` **0**、`OpenCodeGoUsagePanel` 1、`getOpenCodeGoUsage` 1 |
| `GET /api/v1/settings/public` | 200（后端未受影响） |
| 缓存语义 | `index.html` `cache-control: no-cache`（用户立即拿到新页面）；`assets/*` `immutable` 1 年 |

