# 三方指纹对比：ChatGPT.app / Codex CLI / sub 网关

全部来自 2026-09-22 的 mitm **全量头**抓包（不再用白名单）与真实转发路径回放。
同一端点 `/backend-api/codex/responses`。

## 之一：两个真实客户端的头名集合完全相同

`codex exec` 与交互式 TUI 各 17 个头，**头名零差异**，只有取值不同。所以「客户端发什么头」这件事
对 Codex 家族是统一的；差异只在身份取值与 body。

## 之二：UA 的真实模板

| 入口 | User-Agent |
|---|---|
| TUI | `codex-tui/0.155.1 (Mac OS 27.0.0; arm64) xterm-256color (codex-tui; 0.155.1)` |
| exec | `codex_exec/0.155.1 (Mac OS 27.0.0; arm64) unknown (codex_exec; 0.155.1)` |
| Desktop | `Codex Desktop/0.155.0-alpha.9.2 (Mac OS 27.0.0; arm64) unknown (Codex Desktop; 26.915.31945)` |
| **sub** | `codex_cli_rs/0.153.3 (Ubuntu 24.4.0; x86_64) xterm-256color` |

模板：`<originator>/<version> (<OS>; <arch>) <终端串或 unknown> (<originator>; <版本或构建号>)`

**sub 缺少末尾的 `(<originator>; <version>)` 段**（`codexCLIUserAgent` 常量由
`codex_cli_rs/<ver>` + `" (Ubuntu …; x86_64) xterm-256color"` 拼成，构造上就没有这一段）。
真实客户端**无一例外**都有这一段 —— 这是一个精确、易修、高置信的识别点。

注意 TUI 用的是**终端串** `xterm-256color`，所以「终端串 = 假」不成立；我之前据此下的结论是错的。
真正的差异是**末尾那一段的有无**，而不是「终端串 vs unknown」。

## 之三：头级别差异（sub vs 真实客户端）

真实转发路径回放（把真实客户端全量头灌入 `Forward`）得到：

| 头 | 真实客户端 | sub | 判定 |
|---|---|---|---|
| `version` | **从不发送** | `0.153.3` | **凭空新增** |
| `x-codex-installation-id` | **从不发送**（只在 turn-metadata 内） | 有（device/session/full） | **凭空新增** |
| `x-codex-routing-hint` | `model=<请求模型>` | **丢失** | **丢弃** |
| `content-encoding` | `zstd` | **丢失** | **丢弃** |
| `cookie` | 有 | 丢失 | ✅ 正确（网关用账号鉴权） |
| `accept` | `text/event-stream` | 同 | ✅ |
| `x-codex-beta-features` | `prevent_idle_sleep,remote_compaction_v2` | 透传 | ✅ |
| `x-openai-internal-codex-responses-lite` | `true` | 透传 | ✅ |
| `originator` | `codex-tui` / `codex_exec` / `Codex Desktop` | `codex_cli_rs`（恒定） | 改写 |
| `user-agent` | 见上表 | 见上表 | 改写（缺尾段） |
| `session-id`/`thread-id`/`x-client-request-id`/`x-codex-window-id` | 客户端 UUIDv7 | 隔离/派生值 | 改写（多租户设计） |
| `x-codex-turn-metadata` | 17（exec）/ 22（Desktop）字段 | ID 改写，语义字段透传 | 部分改写 |
| `chatgpt-account-id` / `authorization` | 客户端 | 账号 | ✅ 正确替换 |
| `content-length` | 有 | 由 Go 自动 | ✅（值随体变化） |

## 之四：`x-codex-turn-metadata` 字段差异

| 字段 | exec | Desktop | 说明 |
|---|---|---|---|
| 字段数 | 17 | 22 | |
| `turn_trigger` | 无 | `composer` | **Desktop 独有**，sub 全模式透传 |
| `workspace_kind` | 无 | `projectless` | **Desktop 独有** |
| `analytics_enabled` | 无 | `true` | **Desktop 独有** |
| `model` / `reasoning_effort` | 无 | 有 | **Desktop 独有** |
| `context_window_id` | 有 | 有 | 客户端原生 UUID，sub 从不改写 |
| `request_kind` / `thread_source` / `sandbox*` / `node_repl_*` / `window_number` | 有 | 有 | sub 透传 |

所以 sub 当前是「**CLI 外壳（UA/originator/version 头） + Desktop 内核（turn_trigger 等字段）**」，
两边都不自洽。若 meta 来自 Desktop，UA 就该是 `Codex Desktop/...`；若按 CLI 呈现，
meta 里就不该出现 Desktop 独有字段。

## 之五：抓包方法学（本轮踩过的坑，必须留档）

1. **白名单会制造假发现**。旧 addon 只记 14 个头，且 `_hdr` 对 `session_id` 回退到 `session-id`，
   于是把「客户端只发连字符版」误报成「也发了下划线版」，据此得出「网关删下划线头是偏离」的
   **错误结论**。已改：全量头 + 严格按名取。
2. **入口不同身份不同**。`codex exec`（`codex_exec`）、TUI `/responses`（`codex-tui`）、
   TUI `/codex/models`（`codex_cli_rs`）、Desktop（`Codex Desktop`）。拿一个入口的结论推广到
   「CLI」整体是错的。
3. **UDP/入口混用会让结论自相矛盾**。必须先确认入口，再谈取值。

## 待补

- **Desktop 的全量头**：现有 Desktop 数据仍是旧白名单抓包（身份字段齐，但 `x-codex-beta-features`
  / `x-codex-routing-hint` / `content-encoding` 等未采集）。需要把 Desktop 重新挂到代理上并发一条消息。
- **sub 侧仍是重建**，不是生产出站字节。要做成真·生产抓包见下：
  生产 `security.url_allowlist.enabled=false`，可把灰度账号的 `base_url` 指向自控回显服务器，
  网关会把**真实出站请求**发到那里 —— 不触碰上游、不消耗配额。
