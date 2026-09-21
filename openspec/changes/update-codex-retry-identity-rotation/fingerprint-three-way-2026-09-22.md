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

- 中间段是**终端 token**（由环境决定，非入口）：TUI 在真实终端里得 `xterm-256color`，
  无 TTY 时得 `unknown` —— 所以「终端串 = 假客户端」**不成立**（成因见第九节核实）
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
2. **CLI 两个入口都不发**（源码证实：只有声明 `request_attestation` 能力的连接才提供）——
   所以「有没有 attestation」直接区分「App 流量」与「CLI 流量」。
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

## 七、阶段 1 实现记录（2026-09-22）

按「扮 App」方向落地了四项**自洽性**修复（不含 attestation）：

| # | 改动 | 位置 |
|---|---|---|
| 1 | 补 `x-codex-routing-hint: model=<出站 body 的 model>` | `openai_gateway_forward.go` |
| 2 | UA 保证带 `(<originator>; <版本>)` 尾段；规范 UA 常量本身补全 | `openai/request.go`、`codexCLIUserAgent` |
| 3 | 不再新增 `x-codex-installation-id` 独立头（只保留在 turn-metadata 内） | `openai_codex_fingerprint.go` |
| 4 | **按入站客户端类型镜像出站身份**（originator + UA 形状/平台/终端） | `openai_gateway_service.go` `codexIdentityCandidateUA` |

镜像的优先级：账号级显式 UA > 官方 Codex 家族（镜像）> 账号派生 UA（回退）。

**镜像必须带完整性守卫**：残缺 UA（如裸 `codex_cli_rs/0.1.0`，无 OS 组）镜像后会产生现实中
不存在的 UA 形态（`codex_cli_rs/0.153.3 (codex_cli_rs; 0.153.3)`），比统一到账号派生身份更糟。
因此只镜像「至少带一个 OS 组」的完整形态，其余回退。这条由实测中的既有测试暴露。

尾段同步规则（`EnsureCodexUserAgentTrailer`）：**仅当尾段等于首段原版本时**才随版本改写；
ChatGPT.app 的尾段是构建号（`26.915.31945`）与首段版本本就不同，必须原样保留。
不用「像不像版本号」的形状判断 —— `26.915.31945` 同样能解析成三段数字。

## 八、下一步可验证的动作

| 动作 | 说明 | 风险 |
|---|---|---|
| 把 outbound 对齐到 **CLI** 身份 | 补 UA 尾段、删 `version`、补 `x-codex-routing-hint`、增 `zstd` | 中（`version` 删除需单账号先验） |
| 给 `turn-metadata` 对齐 CLI 语义 | 去掉/伪造 Desktop 独有字段，或反过来把 UA 换成 Desktop | 需先决定「扮 CLI 还是扮 Desktop」 |
| 阶段 2：删 `version` 头 | 三个真实客户端都不发 | ⚠️ 需单账号先验 404 风险 |
| 阶段 2：转发 App 的 `x-oai-attestation` | token 内无账号标识（只有 bundle/locale/tz/设备 UUID） | 需接受隐私代价 + 单账号灰度 |
| 阶段 3：`content-encoding: zstd` | 真实客户端会压缩请求体 | 待评估 |

---

# 九、用源码/二进制反向核实（2026-09-22）

对照 `codex-rs` 源码（`/tmp/codex-src`）逐条核实本文档前面的实测结论。

## 9.1 UA 的真实构造（`login/src/auth/default_client.rs::get_codex_user_agent`）

```rust
let build_version = env!("CARGO_PKG_VERSION");
let prefix = format!("{}/{build_version} ({} {}; {}) {}",
    originator, os_info.os_type(), os_info.version(),
    os_info.architecture(), user_agent());          // user_agent() = 终端 token
let suffix = USER_AGENT_SUFFIX.lock().map(|v| format!(" ({v})"));  // 尾段 = " (<suffix>)"
```

- **证实**：模板与本文档第三节完全一致。
- **证实并解释**：尾段是**调用方可设置的 `USER_AGENT_SUFFIX`**，不必然是版本号 ——
  CLI 设为 `codex-tui; 0.155.1`（名字;版本），ChatGPT.app 设为 `Codex Desktop; 26.915.31945`
  （名字;构建号）。这解释了两者形态差异，也说明本文档采用的
  「仅当尾段等于首段版本时才随版本改写」是正确取舍。
- **修正**：中间分量来自 `terminal-detection`（`TERM_PROGRAM/TERM` → 内置名，无终端则 `unknown`）。
  所以 TUI 的 `xterm-256color` 与 exec 的 `unknown` 是**环境差异**（我跑 exec 时无 TTY），
  不是入口差异 —— 本文档第二节把成因归到入口是错的。
- **证实**：UA 的版本段就是 `CARGO_PKG_VERSION`；源码中不存在单独构造 `version` 头的代码，
  与「三个客户端都不发 version 头」一致。

## 9.2 `x-oai-attestation` 的真实机制

```rust
// core/src/attestation.rs —— 宿主集成边界
pub trait AttestationProvider { fn header_for_request(&self, ctx: AttestationContext) -> ... }
pub struct AttestationContext { pub thread_id: ThreadId }

// core/src/client.rs
if !self.state.include_attestation { return None; }     // = provider.supports_attestation()
self.state.attestation_provider.as_ref()?.header_for_request(...).await

// model-provider/src/provider.rs
fn supports_attestation(&self) -> bool { /* 该 provider 有缓存的 ChatGPT 认证 */ }

// app-server/src/attestation.rs
let connection_id = thread_state_manager
    .first_attestation_capable_connection_for_thread(thread_id).await?;
outgoing.send_request_to_connections(..., AttestationGenerate(AttestationGenerateParams {}),
                                     /*thread_id*/ None).await;   // 空参数、不随线程
// 100ms 超时；由客户端返回不透明 token
struct AppServerAttestationEnvelope { v: u8, s: u8, t: Option<&str> }  // t = 客户端不透明载荷
```

- **证实**：只有「声明了 `request_attestation` 能力」的连接能提供该值。CLI/TUI 没有这种连接
  → 永远不发该头，与实测一致。
- **证实**：`t` 是**客户端不透明载荷**（测试注释原文 "wraps opaque client payloads"），
  codex 层不解释内容 —— 本文档第四节解出的 bundle_id / locale / 时区 / 设备 UUID 就是它。
- **修正**：信封里的 `s` 是**状态码**（0=Ok, 1=Timeout, 2=RequestFailed, 3=RequestCanceled,
  4=MalformedResponse），不是设备状态字段。实测 `s:0` 即「生成成功」。
- **修正**：本文档早前用带引号的 `rg '"x-oai-attestation"'` 得出「codex-rs 不生成 attestation」是
  **错的** —— 头名在常量 `X_OAI_ATTESTATION_HEADER` 里，我漏检了。
- **新证据（对「转发」可行性关键）**：索要 attestation 的请求**参数为空**、
  `thread_id: None`；thread_id 只用于**挑选一个支持该能力的连接**。
  ⇒ **token 不随线程/提示词/账号变化**，因此「转发客户端 attestation + 网关改写 thread_id」
  **在结构上不冲突**。但它绑定客户端设备（客户端生成），网关无法自行生成。

## 9.3 仍无法判定

- 上游是否把 attestation 与**账号 / token**做绑定校验：codex 层把它当不透明数据，无从判断。
- 客户端侧具体生成方式（Electron / 原生模块内部，`app.asar` 与 native 模块），
  本文档只观测到客户端回传的不透明载荷。
