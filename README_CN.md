<div align="center">

# Dev Proxy

[![Go Version](https://img.shields.io/badge/Go-1.25-blue?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/daydaychen/devproxy)](https://goreportcard.com/report/github.com/daydaychen/devproxy)

一个用 Go 实现的智能 MITM 代理工具，专为 AI 开发者设计，能够拦截、修改并适配复杂的 LLM API 请求（支持 OpenAI Responses API 转换）。

[English](README.md) | [中文](README_CN.md)

</div>

## ✨ 核心特性

- 🔒 **HTTPS MITM 支持** - 自动拦截和解密 HTTPS 流量
- 🤖 **AI 协议适配** - **独家支持**将 OpenAI 最新的 `Responses API` (`/v1/responses`) 自动转换为标准的 `Chat Completions API`。
- 🛠️ **工具调用转换** - 自动将 Responses API 的内建工具（如 `web_search`）包装为标准的 `function_call`，兼容所有上游 Provider（如 DeepSeek、SiliconFlow）。
- 🌊 **流式协议增强** - 自动补全复杂的流式事件序列（`created` -> `added` -> `delta` -> `done` -> `completed`），确保与 Codex、OpenAI SDK 的完美兼容。
- 🎯 **URL 匹配** - 支持正则表达式和字符串匹配。
- ✏️ **请求头重写** - 灵活修改 User-Agent、Authorization 等请求头。
- 🔄 **上游代理** - 支持转发到 HTTP/SOCKS 代理（如 Clash）。
- 🎲 **随机端口** - 自动分配可用端口，避免冲突。
- 🔐 **进程隔离** - 只代理启动的子进程，不影响系统其他程序。
- 💻 **交互式应用** - 支持 vim、bash、python 等交互式程序。
- 📝 **详细日志** - 详细的流量日志与插件执行记录，方便调试。

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

## 📖 插件与 AI 适配示例

### 1. OpenAI Responses API 自动适配 (responses-api 插件)

如果你正在使用支持新版 `Responses API` 的工具（如 Codex），但希望连接到只支持标准 `Chat Completions` 的上游（如 DeepSeek），可以使用此插件：

```yaml
rules:
  - name: "adapter-to-deepseek"
    match: ["https://api.openai.com/v1/responses"]
    plugins:
      - "responses-api" # 自动处理 /v1/responses 到 /v1/chat/completions 的双向转换
    overwrite:
      Authorization: "Bearer your-deepseek-key"
```

**特性：**
- 自动将 `input_text` 转换为 `text`。
- 自动将内建工具 `web_search` 转换为函数调用。
- 自动修复 `additionalProperties` 导致的 Provider 校验失败。
- 补全流式响应中的所有必需事件，防止客户端断开连接。

### 2. Codex 响应格式修复 (codex-fix 插件)

针对某些模型（如 Minimax、DeepSeek）在处理工具调用或思维链（CoT）时返回的非标准 JSON 数组内容，自动将其展平为字符串以兼容 Codex：

```yaml
rules:
  - name: "fix-codex-content"
    match: ["/chat/completions"]
    plugins:
      - "codex-fix" # 自动将 content: [{type: "text", text: "..."}] 展平为字符串
```

### 3. 常规请求头重写

```yaml
rules:
  - name: "github-api"
    match: ["github.com"]
    overwrite:
      Authorization: "token your-token"
      User-Agent: "GithubBot"
```

## ⚙️ 命令行参数

| 参数 | 简写 | 说明 | 示例 |
|------|------|------|------|
| `--config` | `-c` | 配置文件路径 (YAML) | `--config config.yaml` |
| `--match` | - | URL 匹配规则（可多次指定） | `--match "domain.com/api"` |
| `--overwrite` | - | 请求头重写（格式: `header=value`） | `--overwrite ua=Bot` |
| `--upstream` | - | 上游代理地址 | `--upstream http://127.0.0.1:7890` |
| `--port` | - | 指定代理端口（默认随机） | `--port 8888` |
| `--verbose` | `-V` | 详细日志输出 | `--verbose` |
| `--version` | `-v` | 查看版本号 | `devproxy -v` |

## 🛡️ 安全说明

> **⚠️ 警告**: 此工具会自动设置 `NODE_TLS_REJECT_UNAUTHORIZED=0` 等环境变量来绕过 HTTPS 证书验证，**仅适用于开发和测试环境**。请勿在生产环境中使用。

## 📁 项目结构

- `pkg/proxy/plugin_responses_api.go`: 核心 AI 协议适配逻辑。
- `pkg/proxy/server.go`: 支持规则持久化与状态追踪的 MITM 代理引擎。
- `pkg/proxy/matcher.go`: 高性能 URL 匹配引擎。

## 📄 许可

MIT License - 详见 [LICENSE](LICENSE) 文件。
