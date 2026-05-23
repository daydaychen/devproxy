<div align="center">

# Dev Proxy

[![Go Version](https://img.shields.io/badge/Go-1.25-blue?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/daydaychen/devproxy)](https://goreportcard.com/report/github.com/daydaychen/devproxy)
[![CI](https://github.com/daydaychen/devproxy/actions/workflows/ci.yml/badge.svg)](https://github.com/daydaychen/devproxy/actions/workflows/ci.yml/badge.svg)
[![Latest Release](https://img.shields.io/github/v/release/daydaychen/devproxy)](https://github.com/daydaychen/devproxy/releases)

一个用 Go 实现的智能 MITM 代理工具，专为 AI 开发者设计，能够拦截、修改并适配复杂的 LLM API 请求（支持 OpenAI Responses API 转换）。

[English](README.md) | [中文](README_CN.md)

</div>

## ✨ 核心特性

- 🔒 **HTTPS MITM 支持** - 自动拦截和解密 HTTPS 流量
- 🤖 **AI 协议适配** - **独家支持**将 OpenAI 最新的 `Responses API` (`/v1/responses`) 自动转换为标准的 `Chat Completions API`
- 🛠️ **工具调用转换** - 自动将 Responses API 的内建工具（如 `web_search`）包装为标准的 `function_call`，兼容所有上游 Provider
- 🌊 **流式协议增强** - 自动补全复杂的流式事件序列（`created` → `added` → `delta` → `done` → `completed`），确保与 Codex、OpenAI SDK 的完美兼容
- 🧠 **Anthropic 思维修复** - 修复 Anthropic 流式生命周期事件，丢弃无效的 `signature_delta`，处理并行工具调用索引冲突 bug
- 🔄 **双向转换** - 同时支持正向（Responses → Chat Completions）和反向（Chat Completions → Responses）转换
- 🎯 **URL 匹配** - 支持正则表达式和字符串匹配，带 URL 规范化处理
- ✏️ **请求头重写** - 灵活修改 User-Agent、Authorization 等请求头
- 🔄 **上游代理** - 支持转发到 HTTP/SOCKS 代理（如 Clash）
- 🎲 **随机端口** - 自动分配可用端口，避免冲突
- 🔐 **进程隔离** - 只代理启动的子进程，不影响系统其他程序
- 💻 **交互式应用** - 支持 vim、bash、python 等交互式程序
- 📝 **详细日志** - 详细的流量日志与插件执行记录，方便调试

## 🚀 快速开始

### 安装

**使用 Go 安装:**

```bash
go install github.com/daydaychen/devproxy@latest
```

**从源码编译:**

```bash
git clone https://github.com/daydaychen/devproxy.git
cd devproxy
make build
./devproxy --help
```

### 基本用法

```bash
devproxy [flags] -- <command> [args...]
```

**示例:**

```bash
# 代理 curl 请求并重写请求头
devproxy --overwrite Authorization="Bearer sk-xxx" -- curl https://api.example.com/data

# 使用 Responses API 适配运行 Codex 连接 DeepSeek
devproxy -c config.yaml -- codex "explain this code"

# 通过上游代理（Clash）运行
devproxy --upstream http://127.0.0.1:7890 -- python script.py
```

## 📖 插件与 AI 适配

### 1. OpenAI Responses API 自动适配 (`responses-api` 插件)

如果你正在使用支持新版 `Responses API` 的工具（如 Codex），但希望连接到只支持标准 `Chat Completions` 的上游（如 DeepSeek），可以使用此插件：

```yaml
rules:
  - name: "adapter-to-deepseek"
    match: ["https://api.openai.com/v1/responses"]
    plugins:
      - "responses-api"
    overwrite:
      Authorization: "Bearer your-deepseek-key"
```

**特性：**
- 自动将 `input_text` 转换为 `text`
- 自动将内建工具 `web_search` 转换为函数调用
- 自动修复 `additionalProperties` 导致的 Provider 校验失败
- 补全流式响应中的所有必需事件，防止客户端断开连接
- 双向转换：同时处理请求（Responses → Chat）和响应（Chat → Responses）

### 2. OpenAI Responses 反向转换 (`openai-responses` 插件)

将标准 `Chat Completions` 请求/响应转换为 `Responses API` 格式（反向转换）：

```yaml
rules:
  - name: "reverse-adapter"
    match: ["https://api.openai.com/v1/chat/completions"]
    response-plugins:
      - "openai-responses"
```

### 3. Codex 响应格式修复 (`codex-fix` 插件)

针对某些模型（如 Minimax、DeepSeek）在处理工具调用或思维链（CoT）时返回的非标准 JSON 数组内容，自动将其展平为字符串以兼容 Codex：

```yaml
rules:
  - name: "fix-codex-content"
    match: ["/chat/completions"]
    plugins:
      - "codex-fix"
```

**带模型替换**（将请求的模型替换为另一个模型）：

```yaml
plugins:
  - "codex-fix:deepseek-chat"
```

### 4. Anthropic 思维修复 (`anthropic-thinking-fix` 插件)

修复 Anthropic 流式传输中导致客户端失败的问题：

```yaml
rules:
  - name: "fix-anthropic"
    match: ["https://api.anthropic.com"]
    plugins:
      - "anthropic-thinking-fix"
```

**特性：**
- 丢弃 `signature_delta` 事件（伪造签名导致校验失败）
- 补全缺失的 `message_delta`/`message_stop`/`content_block_stop` 生命周期事件
- 处理并行工具调用 bug：将同一 `index` 上的多个工具调用拆分为独立的 `content_block_start/stop` 对

### 5. 强制流式输出 (`force-stream` 插件)

当请求包含 `stream_options` 但缺少 `stream` 字段时，强制设置 `stream:true`：

```yaml
rules:
  - name: "force-streaming"
    match: ["/chat/completions"]
    plugins:
      - "force-stream"
```

### 6. 常规请求头重写（无需插件）

```yaml
rules:
  - name: "github-api"
    match: ["github.com"]
    overwrite:
      Authorization: "token your-token"
      User-Agent: "GithubBot"
```

## ⚙️ 配置参考

### YAML 配置格式

```yaml
# 全局设置
verbose: true
upstream: "http://127.0.0.1:7890"
port: 8080
log-file: "./proxy.log"

# 全局匹配/重写（应用于默认规则）
match:
  - "google.com"
overwrite:
  User-Agent: "devproxy/1.0"

# 全局插件
plugins:
  - "codex-fix"
response-plugins:
  - "openai-responses"

# 规则组（优先级更高，独立匹配）
rules:
  - name: "adapter-to-deepseek"
    match: ["https://api.openai.com/v1/responses"]
    plugins:
      - "responses-api"
    overwrite:
      Authorization: "Bearer sk-xxx"
  - name: "fix-codex-content"
    match: ["/chat/completions"]
    plugins:
      - "codex-fix:deepseek-chat"
```

### 配置源优先级（低 → 高）

1. **全局配置**: `~/.config/devproxy/global.yaml`
2. **目录配置**: 当前工作目录下的 `devproxy.yaml` 或 `.devproxy.yaml`
3. **显式配置**: `--config <path>`
4. **命令行参数**: `--match`, `--overwrite` 等

列表类型（match、overwrite、plugins、rules）在各配置源之间**累加合并**。单值类型（port、upstream、verbose）使用**后者覆盖**。

### 命令行参数

| 参数 | 简写 | 说明 | 示例 |
|------|------|------|------|
| `--config` | `-c` | 配置文件路径 (YAML) | `--config config.yaml` |
| `--match` | - | URL 匹配规则（可多次指定） | `--match "domain.com/api"` |
| `--overwrite` | - | 请求头重写（格式: `header=value`） | `--overwrite ua=Bot` |
| `--upstream` | - | 上游代理地址 | `--upstream http://127.0.0.1:7890` |
| `--port` | - | 指定代理端口（默认随机） | `--port 8888` |
| `--verbose` | `-V` | 详细日志输出 | `--verbose` |
| `--version` | `-v` | 查看版本号 | `devproxy -v` |

## 🏗️ 架构

Dev Proxy 使用**双进程模型**：

- **父进程**: CLI 入口（Cobra），配置加载，启动代理工作进程 + 目标命令
- **代理工作进程** (`__internal_proxy_worker`): 在隔离子进程中运行 MITM 代理服务器，实现日志隔离

**请求流程：**
1. 目标进程发送 HTTP/HTTPS 请求（通过注入的环境变量）
2. 代理接收 → URL 规范化 → 规则匹配 → 插件执行 → 请求头重写 → 转发到上游
3. 上游响应 → 规则匹配 → 响应插件执行 → 返回客户端

## 📁 项目结构

```
devproxy/
├── main.go                    # 入口点
├── cmd/root.go                # Cobra CLI: 配置加载、进程编排
├── pkg/config/config.go       # YAML 配置加载 (RuleConfig 结构体)
├── pkg/proxy/
│   ├── plugin.go              # RequestPlugin/ResponsePlugin 接口与注册表
│   ├── plugin_responses_api.go    # Responses → Chat Completions
│   ├── plugin_openai_responses.go # Chat Completions → Responses (反向)
│   ├── plugin_codex.go            # 内容展平 + 模型替换
│   ├── plugin_anthropic_thinking.go # Anthropic 流式生命周期修复
│   ├── plugin_force_stream.go     # 强制 stream:true
│   ├── server.go              # ProxyServer 核心 (MITM, 规则引擎)
│   ├── matcher.go             # URLMatcher, RegexMatcher, StringMatcher
│   ├── rewriter.go            # 预计算字节请求头重写
│   ├── buffer_pool.go         # sync.Pool 缓冲区回收
│   ├── cert_cache.go          # LRU+TTL TLS 证书缓存
├── pkg/process/
│   ├── launcher.go            # 环境变量注入、子进程生命周期
│   ├── certs.go               # 导出 goproxy CA 证书到临时文件
├── pkg/util/                  # 版本、端口、ANSI、FD 劫持
├── examples/                  # 示例配置和测试脚本
├── docs/                      # 架构文档
├── scripts/                   # 性能测试和测试脚本
├── Makefile                   # 构建、发布、交叉编译目标
```

## 🔧 开发

```bash
make build          # 标准构建（带版本注入）
make build-opt      # 优化构建 (-s -w 标志，更小的二进制文件)
make cross-build    # 交叉编译 linux/darwin/windows (amd64/arm64)
make release        # 优化构建 + 安装到 ~/.local/bin/
make clean          # 删除二进制文件和构建目录

# 测试
go test ./pkg/proxy/   # 运行代理包测试
go test ./...           # 运行所有测试
```

### 添加新插件

1. 创建 `pkg/proxy/plugin_<name>.go`
2. 实现 `RequestPlugin` 接口（可选 `ResponsePlugin`）
3. 在 `plugin.go` init() 中注册: `RegisterPlugin(&YourPlugin{})`
4. 在 `pkg/proxy/plugin_<name>_test.go` 中添加测试

## 🛡️ 安全说明

> **⚠️ 警告**: 此工具会自动设置 `NODE_TLS_REJECT_UNAUTHORIZED=0` 等环境变量来绕过 HTTPS 证书验证，**仅适用于开发和测试环境**。请勿在生产环境中使用。

## 📄 许可

MIT License - 详见 [LICENSE](LICENSE) 文件。