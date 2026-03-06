<div align="center">

# Dev Proxy

[![Go Version](https://img.shields.io/badge/Go-1.25-blue?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/daydaychen/devproxy)](https://goreportcard.com/report/github.com/daydaychen/devproxy)

An intelligent MITM proxy tool implemented in Go, designed for AI developers to intercept, modify, and adapt complex LLM API requests (supporting OpenAI Responses API transformation).

[English](README.md) | [中文](README_CN.md)

</div>

## ✨ Core Features

- 🔒 **HTTPS MITM Support** - Automatically intercept and decrypt HTTPS traffic.
- 🤖 **AI Protocol Adaptation** - **Exclusive support** for automatically converting OpenAI's latest `Responses API` (`/v1/responses`) to the standard `Chat Completions API`.
- 🛠️ **Tool Call Transformation** - Automatically wrap Responses API built-in tools (e.g., `web_search`) into standard `function_call` format, ensuring compatibility with all upstream providers (e.g., DeepSeek, SiliconFlow).
- 🌊 **Streaming Protocol Enhancement** - Automatically complete complex streaming event sequences (`created` -> `added` -> `delta` -> `done` -> `completed`), ensuring perfect compatibility with Codex and OpenAI SDKs.
- 🎯 **URL Matching** - Supports both regex and string matching.
- ✏️ **Header Overwriting** - Flexible modification of headers like User-Agent, Authorization, etc.
- 🔄 **Upstream Proxy** - Supports forwarding to HTTP/SOCKS proxies (e.g., Clash).
- 🎲 **Random Port** - Automatically assigns available ports to avoid conflicts.
- 🔐 **Process Isolation** - Only proxies the started subprocess, not affecting the rest of the system.
- 💻 **Interactive App Support** - Supports interactive programs like vim, bash, python, etc.
- 📝 **Detailed Logging** - Provides detailed traffic logs and plugin execution records for debugging.

## 🚀 Quick Start

### Installation

**Install with Go:**

```bash
go install github.com/daydaychen/devproxy@latest
```

**Build from source:**

```bash
git clone https://github.com/daydaychen/devproxy.git
cd devproxy
make build
./devproxy --help
```

### Basic Usage

```bash
devproxy [flags] -- <command> [args...]
```

## 📖 Plugins & AI Adaptation Examples

### 1. OpenAI Responses API Adaptation (responses-api plugin)

If you are using tools that support the new `Responses API` (like Codex) but want to connect to an upstream that only supports standard `Chat Completions` (like DeepSeek), use this plugin:

```yaml
rules:
  - name: "adapter-to-deepseek"
    match: ["https://api.openai.com/v1/responses"]
    plugins:
      - "responses-api" # Automatically handles bi-directional conversion
    overwrite:
      Authorization: "Bearer your-deepseek-key"
```

**Features:**
- Converts `input_text` to `text`.
- Wraps built-in tool `web_search` into function calls.
- Fixes `additionalProperties` validation issues for various providers.
- Completes all required events in the streaming response to prevent client disconnects.

### 2. Codex Content Format Fix (codex-fix plugin)

For models (like Minimax, DeepSeek) that return non-standard JSON arrays in `content` when using CoT or tool calls, this plugin flattens them to strings for Codex compatibility:

```yaml
rules:
  - name: "fix-codex-content"
    match: ["/chat/completions"]
    plugins:
      - "codex-fix" # Flattens content: [{type: "text", text: "..."}] to string
```

### 3. Regular Header Overwriting

```yaml
rules:
  - name: "github-api"
    match: ["github.com"]
    overwrite:
      Authorization: "token your-token"
      User-Agent: "GithubBot"
```

## ⚙️ CLI Flags

| Flag | Short | Description | Example |
|------|------|------|------|
| `--config` | `-c` | Config file path (YAML) | `--config config.yaml` |
| `--match` | - | URL matching rule (can be multiple) | `--match "domain.com/api"` |
| `--overwrite` | - | Header overwrite (format: `header=value`) | `--overwrite ua=Bot` |
| `--upstream` | - | Upstream proxy address | `--upstream http://127.0.0.1:7890` |
| `--port` | - | Specify proxy port (default random) | `--port 8888` |
| `--verbose` | `-V` | Detailed log output | `--verbose` |
| `--version` | `-v` | Show version | `devproxy -v` |

## 🛡️ Security

> **⚠️ Warning**: This tool sets environment variables like `NODE_TLS_REJECT_UNAUTHORIZED=0` to bypass HTTPS certificate verification. **Only use in development and testing environments.** Do not use in production.

## 📄 License

MIT License - see the [LICENSE](LICENSE) file for details.
