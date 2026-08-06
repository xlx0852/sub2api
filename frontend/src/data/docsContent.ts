/**
 * SicTs 站内接入教程内容
 * 结构对齐常见中转站 /docs 侧栏：Node.js → 各 CLI → API 脚本
 */

export type DocsSectionId =
  | 'nodejs'
  | 'codex'
  | 'codex-imagegen'
  | 'claude'
  | 'gemini'
  | 'trae-solo'
  | 'openclaw'
  | 'hermes'
  | 'api-scripts'
  | 'pi-droid'
  | 'faq'

export interface DocsSection {
  id: DocsSectionId
  /** URL hash, e.g. Nodejs */
  hash: string
  title: string
  summary: string
  markdown: string
}

export function renderDocsMarkdown(section: DocsSection, apiBaseUrl: string): string {
  return section.markdown
    .split('https://code.sicts.shop')
    .join(apiBaseUrl.replace(/\/+$/, ''))
}

/** 额外 hash 别名 → section id */
export const docsHashAliases: Record<string, DocsSectionId> = {
  nodejs: 'nodejs',
  node: 'nodejs',
  codex: 'codex',
  openai: 'codex',
  codeximagegen: 'codex-imagegen',
  imagegen: 'codex-imagegen',
  sictsimagegen: 'codex-imagegen',
  sictsimage: 'codex-imagegen',
  claude: 'claude',
  claudecode: 'claude',
  gemini: 'gemini',
  geminicli: 'gemini',
  traesolo: 'trae-solo',
  trae: 'trae-solo',
  openclaw: 'openclaw',
  hermes: 'hermes',
  apiscripts: 'api-scripts',
  api: 'api-scripts',
  scripts: 'api-scripts',
  pidroid: 'pi-droid',
  pi: 'pi-droid',
  droid: 'pi-droid',
  faq: 'faq',
  common: 'faq'
}

export const docsSections: DocsSection[] = [
  {
    id: 'nodejs',
    hash: 'Nodejs',
    title: 'Node.js 环境安装',
    summary: 'Claude Code、Gemini CLI、Codex CLI 都依赖 Node.js，请先安装 LTS。',
    markdown: `后续 CLI 工具依赖 Node.js。请安装 **LTS** 版本后再继续其它章节。

### Windows

- **官网（推荐）**：[nodejs.org](https://nodejs.org) 下载 LTS 安装包
- **Chocolatey**：\`choco install nodejs-lts\`
- **Scoop**：\`scoop install nodejs-lts\`

### macOS

\`\`\`bash
brew install node
\`\`\`

或从 [Node.js 官网](https://nodejs.org) 下载安装包。

### Linux

**Ubuntu / Debian**

\`\`\`bash
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
sudo apt-get install -y nodejs
\`\`\`

**CentOS / RHEL**

\`\`\`bash
curl -fsSL https://rpm.nodesource.com/setup_lts.x | sudo bash -
sudo yum install -y nodejs
\`\`\`

### 验证

\`\`\`bash
node --version
npm --version
\`\`\`

能输出版本号即可进入下一步。`
  },
  {
    id: 'codex',
    hash: 'Codex',
    title: 'Codex（OpenAI）配置',
    summary: '配置 ~/.codex，供 Codex CLI 与 Codex App 连接 SicTs。',
    markdown: `通过 \`~/.codex\` 连接 SicTs。**Base URL 必须带 \`/v1\`**。

### 安装 Codex CLI

需先完成 [Node.js 环境安装](#Nodejs)。

\`\`\`bash
# macOS / Linux
sudo npm install -g @openai/codex@latest

# Windows
npm install -g @openai/codex@latest

codex --version
\`\`\`

### 写入配置（macOS / Linux）

> 下列脚本会清空旧的 \`~/.codex\`，请先备份。将 \`YOUR_API_KEY\` 换成控制台密钥。

\`\`\`bash
rm -rf ~/.codex && mkdir -p ~/.codex

cat > ~/.codex/config.toml << 'EOF'
model_provider = "sicts"
model = "gpt-5.6-sol"
review_model = "gpt-5.6-sol"
model_reasoning_effort = "high"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 272000
model_auto_compact_token_limit = 272000
effective_context_window_percent = 95

[model_providers.sicts]
name = "sicts"
base_url = "https://code.sicts.shop/v1"
wire_api = "responses"
requires_openai_auth = false
EOF

cat > ~/.codex/auth.json << 'EOF'
{
  "OPENAI_API_KEY": "YOUR_API_KEY"
}
EOF
\`\`\`

### 写入配置（Windows PowerShell）

\`\`\`powershell
if (Test-Path "$env:USERPROFILE\\.codex") { Remove-Item -Recurse -Force "$env:USERPROFILE\\.codex" }
mkdir "$env:USERPROFILE\\.codex"

@"
model_provider = "sicts"
model = "gpt-5.6-sol"
review_model = "gpt-5.6-sol"
model_reasoning_effort = "high"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 272000
model_auto_compact_token_limit = 272000
effective_context_window_percent = 95

[model_providers.sicts]
name = "sicts"
base_url = "https://code.sicts.shop/v1"
wire_api = "responses"
requires_openai_auth = false
"@ | Out-File -FilePath "$env:USERPROFILE\\.codex\\config.toml" -Encoding utf8

@"
{
  "OPENAI_API_KEY": "YOUR_API_KEY"
}
"@ | Out-File -FilePath "$env:USERPROFILE\\.codex\\auth.json" -Encoding utf8
\`\`\`

### 启动

\`\`\`bash
codex
\`\`\`

配置写入后，**已打开的** Codex CLI / App 需要重启才会生效。  
常用模型示例：\`gpt-5.6-sol\`、\`gpt-5.6-terra\`、\`gpt-5.6-luna\`、\`gpt-5.5\`（以控制台定价页为准）。

### 图片生成（API Key 模式）

使用 **SicTs API Key + 自定义 base_url** 时，Codex 往往**不会挂载**内置 \`image_gen\` 工具，系统 \`imagegen\` skill 还可能误走官方 API。  
请安装 **sicts-imagegen** skill，让生图/改图/抠图走 SicTs 网关。详见下一章 [Codex 图片生成](#CodexImagegen)。`
  },
  {
    id: 'codex-imagegen',
    hash: 'CodexImagegen',
    title: 'Codex 图片生成',
    summary: '安装 sicts-imagegen：API Key 模式下用 SicTs 网关生图、改图、抠图。',
    markdown: `在 **Codex Desktop / ChatGPT.app / Codex CLI** 的 API Key（自定义 base_url）会话里，内置 \`image_gen\` 工具经常不可用。  
**sicts-imagegen** 用轻量 CLI + Codex skill，把生图请求固定到 SicTs Images API（\`/v1/images/*\`），避免落到 \`api.openai.com\`。

> 需先完成 [Codex（OpenAI）配置](#Codex)，且分组已开通图片能力。

### 你将获得

| 能力 | 说明 |
|------|------|
| 文生图 | \`generate\`，默认 \`gpt-image-2\` |
| 图编辑 | \`edit\`，支持多参考图 |
| 批量 | \`generate-batch\`，默认 **4 路并行**（\`--concurrency\`） |
| 抠图 / 去背 | \`remove-chroma\`（本地 chroma-key，需 python3 + Pillow） |
| 自动读配置 | 自动使用 \`~/.codex\` 的 base_url 与 API Key |

### 推荐安装：附件 zip + 一句话

最简单、也最推荐：把安装包丢进 Codex，让它自己装。

1. **下载对应系统的 zip**（任选其一）

| 系统 | 下载 |
|------|------|
| macOS Apple Silicon（M 系列） | [/downloads/sicts-imagegen-macos-arm64.zip](/downloads/sicts-imagegen-macos-arm64.zip) |
| macOS Intel | [/downloads/sicts-imagegen-macos-x64.zip](/downloads/sicts-imagegen-macos-x64.zip) |
| Linux x64 | [/downloads/sicts-imagegen-linux-x64.zip](/downloads/sicts-imagegen-linux-x64.zip) |
| Linux arm64 | [/downloads/sicts-imagegen-linux-arm64.zip](/downloads/sicts-imagegen-linux-arm64.zip) |
| Windows x64 | [/downloads/sicts-imagegen-windows-x64.zip](/downloads/sicts-imagegen-windows-x64.zip) |
| Windows arm64 | [/downloads/sicts-imagegen-windows-arm64.zip](/downloads/sicts-imagegen-windows-arm64.zip) |
| 通用包（自动识别平台，约 8MB） | [/downloads/sicts-imagegen-all.zip](/downloads/sicts-imagegen-all.zip) |

2. 打开 **Codex Desktop / ChatGPT.app**，新建对话  
3. **把 zip 当作附件拖进输入框**（或点附件按钮选中 zip）  
4. 发送一句：

\`\`\`text
安装一下这个插件
\`\`\`

5. 等 Codex 解压并执行 \`install.sh\` / \`install.ps1\`。成功时会看到类似：

\`\`\`text
Installed: ~/.codex/skills/sicts-imagegen
CLI: .../scripts/sicts-image-gen
enabled = true
disabled system imagegen
\`\`\`

6. **完全退出并重启** 客户端，再开 **新会话** 验证：

\`\`\`text
用 $sicts-imagegen 生成一张测试图，保存到 /tmp/sicts-test.png
\`\`\`

> 不确定本机架构时，直接下 [通用包](/downloads/sicts-imagegen-all.zip) 即可，安装脚本会自动选二进制。

### 安装时会做什么

1. 安装到 \`~/.codex/skills/sicts-imagegen\`（Windows 为 \`%USERPROFILE%\\.codex\\skills\\sicts-imagegen\`）
2. 把对应系统的 CLI 放到 \`scripts/sicts-image-gen\`
3. 在 \`config.toml\` 中 **启用** 本 skill（\`enabled = true\`）
4. **禁用** 系统 \`imagegen\` skill，避免误走官方 CLI
5. **不会**改动你的 \`model_providers\` / API Key / 模型配置

### 备选：终端手动安装

不方便附件时，可在终端自行安装：

\`\`\`bash
curl -fLO https://code.sicts.shop/downloads/sicts-imagegen-all.zip
unzip sicts-imagegen-all.zip
cd sicts-imagegen-all
./install.sh
\`\`\`

Windows PowerShell：

\`\`\`powershell
# 先下载 sicts-imagegen-windows-x64.zip 或 all 包并解压
powershell -ExecutionPolicy Bypass -File .\\install.ps1
\`\`\`

可选环境变量：

\`\`\`bash
# 不禁用系统 imagegen
SICTS_DISABLE_SYSTEM_IMAGEGEN=0 ./install.sh

# 通用包：安装后只保留本机平台二进制（更省空间）
SICTS_INSTALL_STRIP_OTHER_BINS=1 ./install.sh
\`\`\`

干跑检查是否读到 SicTs：

\`\`\`bash
~/.codex/skills/sicts-imagegen/scripts/sicts-image-gen \\
  --print-config --dry-run generate \\
  --prompt 'probe' --out /tmp/probe.png
\`\`\`

应看到 \`base_url=https://code.sicts.shop/v1\`（或你的站点域名）以及已配置的 Key 来源。

### 常用命令

**生图**

\`\`\`bash
BIN=~/.codex/skills/sicts-imagegen/scripts/sicts-image-gen

"$BIN" generate \\
  --prompt '一只赛博猫咪，产品主图' \\
  --model gpt-image-2 \\
  --size 1024x1024 \\
  --quality high \\
  --out brand/cat.png
\`\`\`

**同一提示多张变体**

\`\`\`bash
"$BIN" generate --prompt 'logo 方案' --n 4 --out-dir /tmp/logos
\`\`\`

**批量（不同 prompt，并行）**

\`\`\`bash
# prompts.jsonl 每行: {"prompt":"..."}
"$BIN" generate-batch \\
  --input prompts.jsonl \\
  --out-dir output/imagegen/batch \\
  --concurrency 4
\`\`\`

**抠图（绿幕 / chroma-key → 透明 PNG）**

\`gpt-image-2\` 不支持 \`background=transparent\`。推荐：先生成绿底，再本地去背。

\`\`\`bash
# 需要: python3 + Pillow（python3 -m pip install pillow）
"$BIN" remove-chroma \\
  --input brand/source/mascot-chroma.png \\
  --out brand/mascot.png \\
  --auto-key border \\
  --soft-matte true \\
  --transparent-threshold 12 \\
  --opaque-threshold 220 \\
  --despill true \\
  --force
\`\`\`

### 凭证优先级

CLI 自动解析（高 → 低）：

1. 命令行 \`--base-url\` / \`--api-key\`
2. 环境变量 \`SICTS_*\` 或 \`OPENAI_*\`
3. Codex \`~/.codex/config.toml\` 当前 provider + \`auth.json\`
4. 默认 base：\`https://code.sicts.shop/v1\`

已按 [Codex 配置](#Codex) 接好 SicTs 时，**一般无需再 export Key**。

### 常见问题

| 现象 | 处理 |
|------|------|
| skill 装了但对话不用 | 重启客户端 + **新开会话**；确认 config 里 sicts-imagegen \`enabled = true\` |
| 仍想走系统 imagegen | 安装时设 \`SICTS_DISABLE_SYSTEM_IMAGEGEN=0\`，或手动改 config |
| HTTP 401 | Key 错误或过期 |
| HTTP 403 | 分组未开通图片 / 无权限 |
| HTTP 404 Images | 当前 Key 平台不支持 Images 路径 |
| remove-chroma 找不到脚本 | 使用通用包 v0.1.2+，脚本在 skill 的 \`scripts/remove_chroma_key.py\` |
| 非 SicTs 中转 | CLI 会跟客户当前 Codex \`base_url\` 走；未配置时才默认 SicTs |
| **Windows：\`too few unicode value digits\` / Codex 打不开** | 旧安装脚本把 path 写成**双引号 + 反斜杠**，TOML 把 \`\\U\` 当 Unicode 转义。用记事本打开 \`%USERPROFILE%\\.codex\\config.toml\`，把 path 外层改成**单引号**（反斜杠可不动）：\`path = 'C:\\Users\\你的用户名\\.codex\\skills\\sicts-imagegen\\SKILL.md'\`。客户实测单引号可用；新版安装包已按此写入。 |

### 卸载

\`\`\`bash
# 包内脚本
./uninstall.sh

# 或手动
rm -rf ~/.codex/skills/sicts-imagegen
\`\`\`

\`config.toml\` 里的 \`[[skills.config]]\` 条目默认保留，可手动删除。`
  },
  {
    id: 'claude',
    hash: 'Claude',
    title: 'Claude Code 配置',
    summary: '通过环境变量把 Claude Code 接到 SicTs Anthropic 兼容入口。',
    markdown: `### 安装

需先完成 [Node.js 环境安装](#Nodejs)。

\`\`\`bash
# Windows
npm install -g @anthropic-ai/claude-code

# macOS / Linux
sudo npm install -g @anthropic-ai/claude-code

claude --version
\`\`\`

### 环境变量

| 变量 | 值 |
|------|-----|
| \`ANTHROPIC_BASE_URL\` | \`https://code.sicts.shop\` |
| \`ANTHROPIC_AUTH_TOKEN\` | 控制台 API Key |
| \`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC\` | \`1\`（建议） |

**注意**：Claude Code 的 Base URL **不要**加 \`/v1\`，客户端会自行拼接路径。

**临时 · macOS / Linux**

\`\`\`bash
export ANTHROPIC_BASE_URL="https://code.sicts.shop"
export ANTHROPIC_AUTH_TOKEN="YOUR_API_KEY"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
cd your-project
claude
\`\`\`

**临时 · PowerShell**

\`\`\`powershell
$env:ANTHROPIC_BASE_URL="https://code.sicts.shop"
$env:ANTHROPIC_AUTH_TOKEN="YOUR_API_KEY"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"
cd your-project
claude
\`\`\`

**永久 · macOS (zsh)**

\`\`\`bash
echo 'export ANTHROPIC_BASE_URL="https://code.sicts.shop"' >> ~/.zshrc
echo 'export ANTHROPIC_AUTH_TOKEN="YOUR_API_KEY"' >> ~/.zshrc
echo 'export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1' >> ~/.zshrc
source ~/.zshrc
\`\`\`

**永久 · Windows（PowerShell）**

\`\`\`powershell
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_BASE_URL", "https://code.sicts.shop", "User")
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_AUTH_TOKEN", "YOUR_API_KEY", "User")
[System.Environment]::SetEnvironmentVariable("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1", "User")
\`\`\`

永久设置后请**重新打开终端**再运行 \`claude\`。`
  },
  {
    id: 'gemini',
    hash: 'Gemini',
    title: 'Gemini CLI（已停用）',
    summary: 'Google 已宣布不再维护 Gemini CLI，SicTs 已下线该供应商。',
    markdown: `### 安装

\`\`\`bash
# Windows
npm install -g @google/gemini-cli

# Linux
sudo npm install -g @google/gemini-cli

# macOS
sudo npm install -g @google/gemini-cli
# 或 brew install gemini-cli

gemini --version
\`\`\`

### 环境变量

| 变量 | 值 |
|------|-----|
| \`GOOGLE_GEMINI_BASE_URL\` | \`https://code.sicts.shop\` |
| \`GEMINI_API_KEY\` | 控制台 API Key |
| \`GEMINI_MODEL\` | 示例 \`gemini-3-pro-preview\`（以定价页为准） |

\`\`\`bash
export GOOGLE_GEMINI_BASE_URL="https://code.sicts.shop"
export GEMINI_API_KEY="YOUR_API_KEY"
export GEMINI_MODEL="gemini-3-pro-preview"
gemini
# 非交互
gemini -p "你好"
\`\`\`

**PowerShell**

\`\`\`powershell
$env:GOOGLE_GEMINI_BASE_URL = "https://code.sicts.shop"
$env:GEMINI_API_KEY = "YOUR_API_KEY"
$env:GEMINI_MODEL = "gemini-3-pro-preview"
\`\`\``
  },
  {
    id: 'trae-solo',
    hash: 'TraeSolo',
    title: 'TRAE SOLO 配置',
    summary: '在 TRAE SOLO 添加自定义模型，分别接 Codex 与 Claude 通道。',
    markdown: `### 进入自定义模型

1. 打开 **设置**
2. 进入 **模型**
3. 点击 **添加模型**
4. 选择 **自定义配置**

### 按通道填写

| 通道 | Base URL | 示例模型 | 说明 |
|------|----------|----------|------|
| **Codex / OpenAI** | \`https://code.sicts.shop/v1\` | \`gpt-5.6-sol\` | **必须带** \`/v1\` |
| **Claude / Anthropic** | \`https://code.sicts.shop\` | \`claude-opus-4-7\` 等 | Anthropic 根地址，**不要**误加路径 |

API Key 一律使用控制台密钥。`
  },
  {
    id: 'openclaw',
    hash: 'OpenClaw',
    title: 'OpenClaw 配置',
    summary: '用 openclaw onboard 接入 Claude 或 OpenAI 通道。',
    markdown: `### 安装

\`\`\`bash
npm install -g @openclaw/cli
\`\`\`

### Claude 通道

\`\`\`bash
export ANTHROPIC_API_KEY="YOUR_API_KEY"
openclaw onboard --auth-choice custom-api-key \\
  --custom-base-url https://code.sicts.shop \\
  --custom-api-key-env ANTHROPIC_API_KEY \\
  --custom-compatibility anthropic \\
  --custom-model claude-opus-4-6
\`\`\`

### OpenAI / Codex 通道

\`\`\`bash
export OPENAI_API_KEY="YOUR_API_KEY"
openclaw onboard --auth-choice custom-api-key \\
  --custom-base-url https://code.sicts.shop/v1 \\
  --custom-api-key-env OPENAI_API_KEY \\
  --custom-compatibility openai \\
  --custom-model gpt-5.6-sol
\`\`\`

\`\`\`bash
openclaw
\`\`\``
  },
  {
    id: 'hermes',
    hash: 'Hermes',
    title: 'Hermes 配置',
    summary: '通过 ~/.hermes/config.yaml 接入 SicTs。',
    markdown: `### 安装

\`\`\`bash
pipx install hermes-agent
\`\`\`

### Claude 通道

\`\`\`yaml
# ~/.hermes/config.yaml
model:
  default: claude-opus-4-7
  provider: sicts-claude
providers:
  sicts-claude:
    api_mode: anthropic_messages
    base_url: https://code.sicts.shop
    api_key: YOUR_API_KEY
    default_model: claude-opus-4-7
    models:
      - claude-opus-4-7
\`\`\`

### Codex 通道

\`\`\`yaml
# ~/.hermes/config.yaml
model:
  default: gpt-5.6-sol
  provider: sicts-codex
providers:
  sicts-codex:
    api_mode: codex_responses
    base_url: https://code.sicts.shop/v1
    api_key: YOUR_API_KEY
    default_model: gpt-5.6-sol
    models:
      - gpt-5.6-sol
\`\`\`

\`\`\`bash
hermes
\`\`\``
  },
  {
    id: 'api-scripts',
    hash: 'ApiScripts',
    title: 'API 脚本接入',
    summary: '直接调用 Responses / Chat Completions / Messages 接口。',
    markdown: `鉴权统一：\`Authorization: Bearer YOUR_API_KEY\`。将密钥换成控制台创建的 Key。

### 地址速查

| 用途 | 地址 |
|------|------|
| OpenAI 兼容根 | \`https://code.sicts.shop/v1\` |
| Responses | \`POST /v1/responses\` |
| Chat Completions | \`POST /v1/chat/completions\` |
| Messages | \`POST /v1/messages\` |
| Images | \`POST /v1/images/generations\` |

### Responses · Python（流式）

\`\`\`python
import json
import urllib.request

API_URL = "https://code.sicts.shop/v1/responses"
API_KEY = "YOUR_API_KEY"

body = {
    "model": "gpt-5.5",
    "stream": True,
    "input": "用中文简单介绍一下 SicTs。",
}

def iter_sse(response):
    buffer = ""
    while chunk := response.read(4096):
        buffer += chunk.decode("utf-8", errors="replace")
        frames = buffer.split("\\n\\n")
        buffer = frames.pop()
        for frame in frames:
            data = "\\n".join(
                line[5:].strip() for line in frame.splitlines() if line.startswith("data:")
            ).strip()
            if data and data != "[DONE]":
                yield data

request = urllib.request.Request(
    API_URL,
    data=json.dumps(body).encode("utf-8"),
    method="POST",
    headers={
        "Authorization": "Bearer " + API_KEY,
        "Content-Type": "application/json",
        "Accept": "text/event-stream",
    },
)

with urllib.request.urlopen(request, timeout=900) as response:
    for data in iter_sse(response):
        event = json.loads(data)
        if event.get("type") == "response.output_text.delta":
            print(event.get("delta", ""), end="", flush=True)
print()
\`\`\`

### Chat Completions · curl

\`\`\`bash
curl -N https://code.sicts.shop/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-5.5",
    "stream": true,
    "messages": [{"role":"user","content":"你好"}]
  }'
\`\`\`

### Messages · curl

\`\`\`bash
curl -N https://code.sicts.shop/v1/messages \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "anthropic-version: 2023-06-01" \\
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "stream": true,
    "messages": [{"role":"user","content":"你好"}]
  }'
\`\`\`

### 视觉 / 图片

| 能力 | 路径 |
|------|------|
| Responses 视觉 | \`POST /v1/responses\`（\`input_image\`） |
| Chat 视觉 | \`POST /v1/chat/completions\`（\`image_url\`） |
| 文生图 | \`POST /v1/images/generations\` |
| 图编辑 | \`POST /v1/images/edits\` |`
  },
  {
    id: 'pi-droid',
    hash: 'PiDroid',
    title: 'pi / Factory Droid',
    summary: '常用桌面 Agent 接入 SicTs 的要点。',
    markdown: `### pi

- Provider：OpenAI Responses 兼容（**不要**使用需要 ChatGPT OAuth \`accountId\` 的官方 Codex 账号模式）
- Base URL：\`https://code.sicts.shop/v1\`
- API Key：控制台 **GPT / Codex 分组** 对应的 Key
- 默认模型示例：\`gpt-5.6-sol\`
- 修改 \`auth.json\` 后需**重启 pi**（\`/reload\` 不一定刷新内存中的 Key）

### Factory Droid

| 模型类型 | Base | Key |
|----------|------|-----|
| GPT / Codex | \`https://code.sicts.shop/v1\` | Codex 分组密钥 |
| Claude | \`https://code.sicts.shop\` | Claude 分组密钥 |

修改 \`settings.json\` 后重启 Droid。

> 不同分组的 Key 不可混用：GPT 流量用 GPT/Codex 组 Key，Claude 用 Claude 组 Key。`
  },
  {
    id: 'faq',
    hash: 'FAQ',
    title: '常见问题',
    summary: 'Base URL、重启、错误码与密钥安全。',
    markdown: `### Base URL 要不要加 \`/v1\`？

| 客户端 | 建议 |
|--------|------|
| Codex CLI / OpenAI SDK / Responses / Chat | \`https://code.sicts.shop/v1\` |
| Claude Code / Anthropic SDK | \`https://code.sicts.shop\`（客户端自拼路径） |
| TRAE SOLO | 按通道二选一，见 [TRAE SOLO](#TraeSolo) |

### 改完配置不生效？

- CLI / 桌面端：**完全退出再开**
- 环境变量：永久设置后需新开终端
- pi：重启进程，不要只依赖 \`/reload\`

### 400 / 502？

- 网关已对常见第三方客户端做兼容（如 strip \`max_output_tokens\`、clamp 超长 \`call_id\`）
- 长流式 / 大上下文可能出现上游超时 502，可缩短上下文后重试
- 把完整错误 JSON 与请求路径反馈运维可进一步排查

### 密钥安全

- 不要把完整 Key 贴到公开仓库或截图
- 泄露后在控制台轮换 / 删除旧 Key
- 密钥管理：登录后打开 [API Keys](/keys)`
  }
]

export function resolveDocsSectionId(hash: string): DocsSectionId | null {
  const normalized = hash.replace(/^#/, '').toLowerCase().replace(/[^a-z0-9]/g, '')
  if (!normalized) return null
  return docsHashAliases[normalized] ?? null
}

export function getDocsSection(id: DocsSectionId): DocsSection {
  const section = docsSections.find((item) => item.id === id)
  if (!section) {
    return docsSections[0]
  }
  return section
}
