# 三方指纹对比（全实测）：ChatGPT.app / Codex CLI / sub 网关

数据来源：2026-09-22 mitm **全量头**抓包（无白名单），同一端点 `/backend-api/codex/responses`。
sub 侧为真实转发路径回放（把真实客户端全量头灌入 `Forward`，见
`internal/service/openai_codex_outbound_fingerprint_test.go`）。

## 一、三方的头名与取值

| 头 | `codex exec` | 交互式 TUI | **ChatGPT.app** | **sub 出站** |
|---|---|---|---|---|
| 头数量 | 17 | 17 | **18** | 11–12 |
| `originator` | `codex_exec` | `codex-tui` | `Codex Desktop` | `codex_cli_rs`（恒定） |
| `user-agent` | `codex_exec/0.155.1 (Mac OS 27.0.0; arm64) unknown (codex_exec; 0.155.1)` | `codex-tui/0.155.1 (Mac OS 27.0.0; arm64) xterm-256color (codex-tui; 0.155.1)` | `Codex Desktop/0.155.0-alpha.9.2 (Mac OS 27.0.0; arm64) unknown (Codex Desktop; 26.915.31945)` | `codex_cli_rs/0.153.3 (Ubuntu 24.4.0; x86_64) xterm-256color` |
| `version` | 不发 | 不发 | 不发 | **`0.153.3`** |
| `x-codex-routing-hint` | `model=gpt-6-astra` | 同 | 同 | **丢弃** |
| `content-encoding` | `zstd` | `zstd` | `zstd` | **无（不压缩）** |
| `x-oai-attestation` | 不发 | 不发 | **发** | **丢弃** |
| `x-codex-beta-features` | `prevent_idle_sleep,remote_compaction_v2` | 同 | 同 | 透传 |
| `x-openai-internal-codex-responses-lite` | `true` | 同 | 同 | 透传 |
| `x-codex-installation-id` | 不发 | 不发 | 不发 | **有**（device/session/full） |
| `session-id` / `thread-id` / `x-client-request-id` / `x-codex-window-id` | 客户端 UUIDv7，四者同值 | 同 | 同 | 隔离/派生值 |
| `chatgpt-account-id` / `authorization` | 客户端自己的 | 同 | 同 | 账号的（**正确替换**） |
| `cookie` | 发 | 发 | 发 | **丢弃（正确）** |
| `accept` | `text/event-stream` | 同 | 同 | 同 |

**两个真实入口（exec/TUI）头名完全相同**，只有取值不同；Desktop 多一个 `x-oai-attestation`。

## 二、UA 模板与缺失的尾段

真实模板：`<originator>/<version> (<OS>; <arch>) <终端串或 unknown> (<originator>; <版本或构建号>)`

- TUI 用**终端串** `xterm-256color`，Desktop/exec 用 `unknown` —— 所以「终端串 = 假客户端」**不成立**
- **三个真实客户端末尾都有 `(<originator>; <版本或构建号>)` 段，sub 构造上就没有**
  （`codexCLIUserAgent = "codex_cli_rs/" + version + " (Ubuntu …) xterm-256color"`）
- Desktop 的尾段是**构建号** `26.915.31945`，不是 semver

## 三、四条可修的偏离（按硬度排序）

| # | 偏离 | 方向 | 说明 |
|---|---|---|---|
| 1 | **`x-oai-attestation` 缺失** | 客户端有、sub 无 | **密码学可验证的设备证明**；见第四节 |
| 2 | **`x-codex-routing-hint` 缺失** | 客户端有、sub 无 | **三个真实客户端 100% 都发**，sub 从不发 |
| 3 | `version` 存在 | sub 有、客户端无 | 纯增量；真实客户端从不发这个头 |
| 4 | `x-codex-installation-id` 存在 | sub 有、客户端无 | 真实客户端只把它藏在 `turn-metadata` 里 |
| 5 | UA 缺尾段 | 值不符 | 三个真实客户端都有 |
| 6 | `content-encoding: zstd` 缺失 | 客户端有、sub 无 | 真实客户端会压缩请求体；sub 不压 |

出站白名单（`openaiAllowedHeaders`）里**不含** `x-oai-attestation` 与 `x-codex-routing-hint`，
且 `rg -i 'attestation|routing-hint' internal/` **零命中** —— 网关既不透传也不生成。

## 四、`x-oai-attestation`：结构性天花板

Desktop 实测值（截断）：

```json
{"v":1,"s":0,"t":"v1.pGV0b2tlbnkLpEFnQUFBSFN4VG1qclRjUitpSmxaT2gvdTNYa0VVTmswK…"}
```

base64 解出可读片段：`zh-CN` / `Asia/Shanghai` / `5c18dd97-8646-4a92-8144-374663bc2bc3` /
`ibundle_id` = **`com.openai.codexapp`**。

意义：

1. 它是**设备级**证明（locale、时区、设备/安装 id、bundle id），且可被上游密码学验证。
2. **CLI 两个入口都不发** —— 所以「有没有 attestation」直接区分「App 流量」与「CLI 流量」。
3. **网关不可能自行生成**：真 attestation 需要设备私钥与 App Attest 链路。
4. 转发客户端自带的 attestation 也不可行得干净：
   - 它是**按客户端设备**签的，而网关用的是**另一个账号**的 token；若上游把 attestation 与
     账号/session 绑定，就是硬冲突；
   - 它内含**每个用户的 locale/时区/设备 id**，透传等于把这些交给上游，是隐私回退。

结论：**只要流量经过网关，就无法在 App 这一侧做到「像客户端」**。能做到的是「像一个 CLI」——
那就要把 UA/originator/尾段/头集合**全部对齐到 CLI**，而不是现在的混合体。

## 五、`turn-metadata` 字段差异：sub 现在是混合体

| 字段 | exec（17 字段） | Desktop（22 字段） | sub |
|---|---|---|---|
| `turn_trigger` | 无 | `composer` | **透传** |
| `workspace_kind` | 无 | `projectless` | **透传** |
| `analytics_enabled` | 无 | `true` | **透传** |
| `model` / `reasoning_effort` | 无 | 有 | **透传** |
| `context_window_id` | 有 | 有 | **从不改写** |

于是 sub 出去的请求：**头自称 CLI，元数据自称 Desktop**。两边都不像，是**最容易被识别的组合**。

## 六、抓包方法学（本轮踩过的三个坑，必须留档）

1. **白名单会制造假发现**：旧 addon 只记 14 个头，且 `_hdr` 把 `session_id` 回退到 `session-id`，
   把「客户端只发连字符版」误报成「也发了下划线版」，据此得出「网关删下划线头是偏离」的错误结论。
   已改：全量头 + 严格按名取。
2. **入口不同、身份不同**：`codex exec` → `codex_exec`；TUI 的 `/responses` → `codex-tui`；
   TUI 的 `/codex/models` → `codex_cli_rs`；Desktop → `Codex Desktop`。
   拿一个入口的结论推广到「CLI 整体」是错的（我犯过两次）。
3. **「终端串」不能作为判假依据**：TUI 就用 `xterm-256color`。真正的判据是**尾段的有无**。

## 七、下一步可验证的动作

| 动作 | 说明 | 风险 |
|---|---|---|
| 把 outbound 对齐到 **CLI** 身份 | 补 UA 尾段、删 `version`、补 `x-codex-routing-hint`、增 `zstd` | 中（`version` 删除需单账号先验） |
| 给 `turn-metadata` 对齐 CLI 语义 | 去掉/伪造 Desktop 独有字段，或反过来把 UA 换成 Desktop | 需先决定「扮 CLI 还是扮 Desktop」 |
| 测 attestation 影响 | 无法生成，只能测「有/无」在上游的表现差异 | 需要上游可观测指标 |
