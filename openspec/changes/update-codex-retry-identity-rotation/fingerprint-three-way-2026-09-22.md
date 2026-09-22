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

## 八、阶段 2 实现记录（删 version 头 + 转发 attestation）

| # | 改动 | 结果 |
|---|---|---|
| 5 | **不再发送 `version` 头** | 清除了 9 处写入点（forward / passthrough / 身份收口 3 处 / WS / alpha search / 票探针 / 模型目录 / 账号测试 / 用量探针），并把 `version` 从两个透传白名单移除 |
| 6 | **转发客户端自带的 `x-oai-attestation`** | 加入两个透传白名单，并限定只对 OAuth 账号（API-key 走 api.openai.com，设备证明无意义） |

`version` 的判据是**实测**而非注释：142 条全量抓包（App / TUI / exec 三个入口、`/responses` 与 `/codex/models`）
里**没有任何含 "version" 的请求头**。源码里内置 `openai` provider 确实声明了
`http_headers: {"version": CARGO_PKG_VERSION}`，但它没有出现在打往 `chatgpt.com` 的请求上
（机制未定位，但不影响结论：以实测为准）。

阶段 2 后的出站差异（同一份真实客户端头集合回放）：

```
[新增] 客户端没发、网关发了 : []                          ← 关键：不再凭空新增任何头
[丢失] 客户端发了、网关没发 : [content-encoding cookie]    ← cookie 正确丢弃；zstd 属阶段 3
[改写] 两边都有但值不同   : [authorization chatgpt-account-id
                            session-id thread-id x-client-request-id x-codex-window-id
                            user-agent x-codex-turn-metadata]
                            ← 前两个是正确替换；中间四个是多租户隔离；
                              后两个只归一化版本段 / 改写身份字段
```

## 九、下一步可验证的动作

| 动作 | 说明 | 风险 |
|---|---|---|
| 把 outbound 对齐到 **CLI** 身份 | 补 UA 尾段、删 `version`、补 `x-codex-routing-hint`、增 `zstd` | 中（`version` 删除需单账号先验） |
| 给 `turn-metadata` 对齐 CLI 语义 | 去掉/伪造 Desktop 独有字段，或反过来把 UA 换成 Desktop | 需先决定「扮 CLI 还是扮 Desktop」 |
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

---

# 十、蓝绿上线记录（2026-09-22，阶段 1 + 阶段 2）

## 10.1 上线前的两个隐患（都已处理）

**隐患一：工作区含并行会话的半成品。** 直接 `go build` 会把另一个会话的 51 个未提交改动
（含 11 个已暂存删除）编进二进制。改法：从**我的提交**建独立 `git worktree` 构建，
构建树与并行会话完全隔离（抽查确认不含 `user_msg_queue`）。

**隐患二：我的分支曾被误提交混入并行会话的删除。** 早前一次 `docs(openspec)` 提交把索引里
他人已暂存的 11 个删除一起提交了（`user_msg_queue_helper`、`rpm_cache`、`session_limit_cache`
等端口/实现/测试），导致 `internal/repository/wire.go` 引用到不存在的符号、**干净构建失败**。
已新增提交从 master 恢复全部 11 个文件；恢复后分支相对 master **无任何删除**。

这类事故的共同成因：**在有并行会话的仓库里用裸 `git commit` 或目录级 `git add`**，
会把他人暂存的索引状态一起提交。本次共发生 3 次，均已修正。

## 10.2 上线方式与验证

| 项 | 结果 |
|---|---|
| 方式 | 新建候选容器（绿灯，18349）+ nginx 上游切换 + `reload` |
| 版本 | `0.1.391-idmirror-81ec719f3`（公网已可见） |
| 绿灯 | `running/healthy`，重启 0 |
| 旧绿灯 | 优雅排空（切换后 8s 内请求归零），保持运行作秒级回滚 |
| 回滚 | nginx 改回 `18348` + reload；配置已备份 |

**阶段 2 最大风险（删 `version` 头是否触发上游 404）已实测排除**：上线后 10 分钟内
OpenAI 请求全部 200（账号 120 OAuth ×9、账号 89 API-key ×4），**无 404、无身份相关错误**；
仅 4 条与身份无关的 429 failover。零 ERROR。

# 十一、阶段 3 上线记录（请求体 zstd 压缩，2026-09-22）

| 项 | 结果 |
|---|---|
| 方式 | 新建候选容器（18350）+ nginx 上游切换 + reload |
| 版本 | `0.1.391-zstd-2a1f9696c`（公网可见） |
| 绿灯 | `healthy`，重启 0 |
| 回滚 | nginx 改回 `18349` + reload（配置已备份） |

**上游是否接受 `content-encoding: zstd` 已实测确认**（这是本阶段唯一的大风险）：

```
切换后 4 分钟累计：
  /responses 状态码: {200: 15}     ← 全部走通
  全部状态码:        {200: 88, 401: 2, 404: 1}
  上游失败事件:      0
  ERROR:            0
```

（401×2 为客户端鉴权失败，404×1 为客户端打错 `/v1/user/balance`，均与压缩无关。）

## 三阶段完成后的出站差异（对照真实客户端全量头集合）

```
[新增] 客户端没发、网关发了 : []            ← 已清零
[丢失] 客户端发了、网关没发 : [cookie]      ← 正确（网关用账号鉴权，不带客户端 cookie）
[改写] authorization chatgpt-account-id     ← 正确替换为账号鉴权
       session-id thread-id x-client-request-id x-codex-window-id  ← 多租户隔离（设计）
       user-agent                            ← 仅版本段归一化（防钉在陈旧版本）
       x-codex-turn-metadata                 ← 仅身份字段改写，语义字段透传
```

**头级别与请求体编码的冒充差距已全部清除**，剩余差异均为「刻意为之」：

1. `cookie` 不带 —— 网关用账号鉴权；
2. `authorization` / `chatgpt-account-id` 替换 —— 多租户必然；
3. 会话标识隔离 —— 防跨用户会话碰撞，且是 `thread-id` 轮换机制的基础；
4. `user-agent` 版本段归一 —— 防止身份被钉在陈旧版本（陈旧身份会被上游优先降载）。

**结构性不可消除的部分**：`x-oai-attestation` 只有 ChatGPT.app 会带，网关只能转发、无法生成（需客户端设备能力）。

# 十二、生产实据：轮换机制在真实客户端上生效（2026-09-22）

阶段 3 上线后 17 分钟窗口（账号 120，全部 `path=stream_failed`）：

```
account=120 reason=server_overloaded path=stream_failed
  burst_attempt=1  session_epoch=1
  burst_attempt=2  session_epoch=2
  burst_attempt=3  session_epoch=1
  burst_attempt=4  session_epoch=1
  burst_attempt=5  session_epoch=2
  burst_attempt=6  session_epoch=1
  burst_attempt=7  session_epoch=2
```

解读：

- **`burst_attempt` 1→7**：爆发窗口计数生效（同一窗口内的连续过载累加）。
- **`session_epoch` 在 1/2 之间**：epoch 是**按 (账号, 客户端会话)** 计的，每个真实客户端会话各有一条计数；
  同一会话重复过载时 1→2 递增 —— **跨请求身份轮换已在真实 Codex 客户端上生效**。
- 对比此前 4 条 `session_epoch=0`：那批请求未带 Codex 会话头（`/v1/responses` 上的第三方客户端），
  没有会话标识就无从轮换，属预期。

同窗口整体健康度：

```
/responses 状态码: {200: 154, 101: 9}        ← 零 4xx/5xx
全部状态码:        {200: 483, 404: 4, 101: 9, 401: 2}
上游 4xx/5xx:      无
ERROR:             无
容器:              running/healthy 重启=0
```

`404` 全为客户端打错 `/v1/user/balance`，`401` 为客户端鉴权失败。

**两项仍未观测到**（需要相应条件，非缺陷）：

- `overload_path=http_503`：至今所有过载都走流内通道（path2），HTTP 503 通道（path1）未出现；
- `thread_id_rotated=true`：同账号重试路径尚未被触发（`openai.pool_mode_same_account_retry` 零次）。

# 十三、社区情报核实：292 票的「240s 寿命 + 必须带 cookie」（2026-09-22）

社区帖称：292 长度的 turn-state 票原本可用约 1 小时，上游某次变更后**只剩约 240 秒**，
且**必须携带 cookie 才能持续**。逐条核实结果如下。

## 13.1 票的格式与时间戳语义（已亲自解码）

票是 **Fernet 格式**（`gAAAAA` 前缀、版本字节 `0x80`、紧随 8 字节大端时间戳）：

```
292 字符 → 解码 217 字节      312 字符 → 解码 233 字节
（Fernet 头 57 字节 = 版本1 + 时间戳8 + IV16 + HMAC32）
```

时间戳是**该 turn 的起始时刻**，不是票的绝对寿命。标定依据：响应票解出的时刻恒在
请求时刻**之前 7~83 秒**（流式响应耗时），而客户端复用的请求票解出时刻在请求前 25~815 秒。

⇒ 因此**不能用票内时间戳测量"上游接受窗口"**，票是每 turn 重签的。

## 13.2 客户端侧实测（我的抓包，312/luna 票）

复用的请求票真实票龄 **25s ~ 815s（13.6 分钟）**，全部正常完成。
上游在 60/60 次里都返回了**不同的**新票——但这是每 turn 重签的必然结果，
**不构成"旧票被拒绝"的证据**（被忽略与被接受在响应里长得一样）。

⇒ 对 312 票，**没有观察到 240 秒限制**（实测用到 13.6 分钟）。
⇒ 对 292 票，我方**没有任何复用样本**（抓包里 292 只出现在响应侧，从未被复用）。

## 13.3 cookie 依赖：结构性天花板（同 attestation）

我的抓包显示 **72/72 请求全部携带 `cookie`**——真实客户端它是常量。
若上游把票绑定到 cookie 所属会话，**网关无法满足**：网关用账号 OAuth token 鉴权，
不持有 `__Secure-next-auth.session-token`（web 会话凭据），且入站客户端的 cookie
属于**用户自己的账号**，转发既越权也泄露凭据。

⇒ 结论：这一条**不可修**，登记为已知结构限制，与 `x-oai-attestation` 同类。

## 13.4 关键反证：过载与票龄无关（生产实测）

上游对"过期票"的处置在响应里不可观测（见 13.2），唯一可用的地面真值是**降载信号本身**。
因此新增只读诊断：失效时记录**被回放的票有多旧**（`tickets=N age_min_s= age_max_s=`），
部署于 `0.1.391-ageprobe-493602d0c`（纯日志、零行为变更、单测覆盖）。

上线后 8 分钟内 7 次过载的票龄：

| 时刻 | 票数 | 最旧票龄 | 最新票龄 |
|---|---|---|---|
| 15:13:10 | 4 | 2774.1s | **29.0s** |
| 15:15:05 | 4 | 102.0s | **24.2s** |
| 15:15:05 | none | – | – |
| 15:16:06 | 4 | 50.0s | **12.4s** |
| 15:16:52 | 2 | 17.9s | **13.9s** |
| 15:16:52 | none | – | – |
| 15:21:48 | 4 | 292.1s | **110.9s** |

**当前请求真正复用的那张票（最新票）在过载时仅 12~111 秒**，全部远低于可疑窗口。
若"过期票回放招致降载"成立，过载应正向关联**大票龄**——实测方向相反。

（"最旧票龄"可达 2774 秒属正常：那是 4 个池槽位里**其他池**的滞留票，
未必参与本次请求；不同池的票龄本来就不该一致。）

## 13.5 处置

| 情报点 | 核实结果 | 动作 |
|---|---|---|
| 票格式/Fernet | ✅ 亲自解码证实 | – |
| 292 只剩 240s | ❌ 我方 312 票实测用到 13.6 分钟；292 无样本 | 不改 TTL（60 分钟），保留票龄诊断持续取证 |
| 必须带 cookie | ⚠️ 无法证实也无法满足 | 登记为结构限制 |
| 过期票回放致降载 | ❌ 生产反证：过载时票龄 12~111s | 保留诊断，继续积累 |

另：生产**从未**命中 `312 / degraded / revoked` 信号（`invalidate` 原因只有
`server_overloaded`），即上游的"撤销票"路径在我们的流量上**一次都没出现过**。

## 13.6 扩展证据（监控 18 分钟后，共 17 次过载）

| 时刻 | 票数 | 最新票龄（本次真正复用） | 最旧票龄 |
|---|---|---|---|
| 15:13:10 | 4 | 29.0s | 2774.1s |
| 15:15:05 | 4 | 24.2s | 102.0s |
| 15:16:06 | 4 | 12.4s | 50.0s |
| 15:16:52 | 2 | 13.9s | 17.9s |
| 15:21:48 | 4 | 110.9s | 292.1s |
| 15:23:35 | 4 | 14.5s | 55.9s |
| 15:26:32 | 5 | 15.4s | 173.7s |
| 15:29:45 | 5 | 13.3s | 131.5s |
| 15:31:32 | 5 | 16.2s | 86.4s |
| 15:31:39 | 5 | 22.7s | 92.9s |
| 15:36:36 | 5 | 12.7s | 265.5s |

**11 次可测样本中，最新票龄最大仅 110.9 秒，其余全在 12~30 秒。**
结论不变且更强：过载与票龄**无关**，票回放不是降载来源。

**两项排查掉的关键词误报**（避免后续误判）：

- 含有子串 `312` 的 8 条日志**全是误命中**——`response_id=resp_0**312**…`、
  `payload_bytes=` 数字中恰好含 312。**真实 312/降级信号仍为 0**。
- 新增的 `502/504/429` 全为 `pool_mode_error_skipped` 的**账号池切换**
  （含账号 81「国模」的 504），已由 failover 接住，与 Codex 出站无关。

同窗口 `/responses` 状态码：`{200: 293, 101: 21}`（零 4xx/5xx）。
过载通道 **17/17 仍全是 `stream_failed`**，`http_503` 通道依旧未出现；
`thread_id_rotated` 依旧为 0。

## 13.7 本次上线记录（诊断版）

| 项 | 结果 |
|---|---|
| 版本 | `0.1.391-ageprobe-493602d0c`（提交 `493602d0c`） |
| 端口 | 18351，回滚目标 18350（`zstd-2a1f9696c`，保持 running） |
| nginx 备份 | `code-sicts-shop.conf.bak-ageprobe-030748` |
| 二进制校验 | sha256 `61d3f108…6533`，容器内与宿主一致 |
| 变更性质 | **纯日志**（失效时追加 `tickets=N age_min_s= age_max_s=`），零行为变更 |
| 绿灯 | `running/healthy`，重启 0，`/responses` 零 4xx/5xx |
| 启动期 ERROR | 与线上容器**集合一致**（pricing / SubscriptionAutoRenew 为既有噪声） |

排查记录：切换前一度以为新容器独有 pricing ERROR，实为**我先前用 `2>/dev/null` 丢掉了 stderr**
（`legacy_printf` 走 stderr）而误判线上"零 ERROR"。改用 `2>&1` 后两容器 ERROR 集合一致。

## 13.8 抓包收尾（12 小时后主动结束）

抓包（mitmproxy + ChatGPT.app）已停止，App 已恢复正常直连。终止依据：

**12 小时 296 条 `/responses` 记录的票长分布**

| 位置 | 长度 | 次数 |
|---|---|---|
| 请求票（被复用的） | 312 | 105 |
| 响应票 | 312 | 122 |
| 响应票 | **292** | **1** |

**关键结论：292 的请求票一次都没出现。** 也就是说这个 App 实例 12 小时里
始终被路由在 luna(312) 池，**继续抓也拿不到 292 的复用样本**——13.2 节留下的
「292 无复用样本」这一空白**无法通过继续抓包补齐**，只能等自然路由变更。

处置：原始抓包压缩归档到 `scripts/capture-archive/live-2026-09-22-full.jsonl.gz`
（62.2MB → 12MB，读回 296 条一致；`scripts/` 已被 gitignore，不进仓库）。
`/tmp/codex-capture-live/` 保留原文件（可随时删除）。

**收尾核验**：`18888` 无监听、无残留 mitmdump、系统代理 untouched
（`HTTPEnable/HTTPSEnable=0`，脚本本就只用环境变量）、新 App 实例
（pid 25268）**零 18888 连接**且进程树完整（含 `codex app-server`）。
