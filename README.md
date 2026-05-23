<div align="center">

# Dev Proxy

[![Go Version](https://img.shields.io/badge/Go-1.25-blue?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/daydaychen/devproxy)](https://goreportcard.com/report/github.com/daydaychen/devproxy)
[![CI](https://github.com/daydaychen/devproxy/actions/workflows/ci.yml/badge.svg)](https://github.com/daydaychen/devproxy/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/daydaychen/devproxy)](https://github.com/daydaychen/devproxy/releases)

An intelligent MITM proxy tool implemented in Go, designed for AI developers to intercept, modify, and adapt complex LLM API requests (supporting OpenAI Responses API transformation).

[English](README.md) | [中文](README_CN.md)

</div>

## ✨ Core Features

- 🔒 **HTTPS MITM Support** - Automatically intercept and decrypt HTTPS traffic
- 🤖 **AI Protocol Adaptation** - **Exclusive support** for automatically converting OpenAI's latest `Responses API` (`/v1/responses`) to the standard `Chat Completions API`
- 🛠️ **Tool Call Transformation** - Automatically wrap Responses API built-in tools (e.g., `web_search`) into standard `function_call` format, ensuring compatibility with all upstream providers
- 🌊 **Streaming Protocol Enhancement** - Automatically complete complex streaming event sequences (`created` → `added` → `delta` → `done` → `completed`), ensuring perfect compatibility with Codex and OpenAI SDKs
- 🧠 **Anthropic Thinking Fix** - Fix Anthropic streaming lifecycle events, drop invalid `signature_delta`, handle parallel tool call index collision bug
- 🔄 **Bidirectional Conversion** - Both forward (Responses → Chat Completions) and reverse (Chat Completions → Responses) conversion supported
- 🎯 **URL Matching** - Supports both regex and string matching with URL normalization
- ✏️ **Header Overwriting** - Flexible modification of headers like User-Agent, Authorization, etc.
- 🔄 **Upstream Proxy** - Supports forwarding to HTTP/SOCKS proxies (e.g., Clash)
- 🎲 **Random Port** - Automatically assigns available ports to avoid conflicts
- 🔐 **Process Isolation** - Only proxies the started subprocess, not affecting the rest of the system
- 💻 **Interactive App Support** - Supports interactive programs like vim, bash, python, etc.
- 📝 **Detailed Logging** - Provides detailed traffic logs and plugin execution records for debugging

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

**Examples:**

```bash
# Proxy a curl request with header rewriting
devproxy --overwrite Authorization="Bearer sk-xxx" -- curl https://api.example.com/data

# Run Codex with Responses API adaptation to DeepSeek
devproxy -c config.yaml -- codex "explain this code"

# Use with upstream proxy (Clash)
devproxy --upstream http://127.0.0.1:7890 -- python script.py
```

## 📖 Plugins & AI Adaptation

### 1. Responses API Adaptation (`responses-api` plugin)

If you are using tools that support the new `Responses API` (like Codex) but want to connect to an upstream that only supports standard `Chat Completions` (like DeepSeek), use this plugin:

```yaml
rules:
  - name: "adapter-to-deepseek"
    match: ["https://api.openai.com/v1/responses"]
    plugins:
      - "responses-api"
    overwrite:
      Authorization: "Bearer your-deepseek-key"
```

**Features:**
- Converts `input_text` to `text`
- Wraps built-in tool `web_search` into function calls
- Fixes `additionalProperties` validation issues for various providers
- Completes all required events in the streaming response to prevent client disconnects
- Bidirectional: handles both request (Responses → Chat) and response (Chat → Responses) conversion

### 2. OpenAI Responses Reverse Conversion (`openai-responses` plugin)

For converting standard `Chat Completions` requests/responses into the `Responses API` format (reverse direction):

```yaml
rules:
  - name: "reverse-adapter"
    match: ["https://api.openai.com/v1/chat/completions"]
    response-plugins:
      - "openai-responses"
```

### 3. Codex Content Format Fix (`codex-fix` plugin)

For models (like Minimax, DeepSeek) that return non-standard JSON arrays in `content` when using CoT or tool calls, this plugin flattens them to strings for Codex compatibility:

```yaml
rules:
  - name: "fix-codex-content"
    match: ["/chat/completions"]
    plugins:
      - "codex-fix"
```

**With model substitution** (replace the requested model with another):

```yaml
plugins:
  - "codex-fix:deepseek-chat"
```

### 4. Anthropic Thinking Fix (`anthropic-thinking-fix` plugin)

Fixes Anthropic streaming issues that cause client-side failures:

```yaml
rules:
  - name: "fix-anthropic"
    match: ["https://api.anthropic.com"]
    plugins:
      - "anthropic-thinking-fix"
```

**Features:**
- Drops `signature_delta` events (fake signatures cause validation failures)
- Patches missing `message_delta`/`message_stop`/`content_block_stop` lifecycle events
- Handles parallel tool call bug: splits tool calls on the same `index` into separate `content_block_start/stop` pairs

### 5. Force Stream (`force-stream` plugin)

Force `stream:true` when a request contains `stream_options` but missing the `stream` field:

```yaml
rules:
  - name: "force-streaming"
    match: ["/chat/completions"]
    plugins:
      - "force-stream"
```

### 6. Header Overwriting (no plugin needed)

```yaml
rules:
  - name: "github-api"
    match: ["github.com"]
    overwrite:
      Authorization: "token your-token"
      User-Agent: "GithubBot"
```

## ⚙️ Configuration Reference

### YAML Config Format

```yaml
# Global settings
verbose: true
upstream: "http://127.0.0.1:7890"
port: 8080
log-file: "./proxy.log"

# Global match/overwrite (applied to default rule)
match:
  - "google.com"
overwrite:
  User-Agent: "devproxy/1.0"

# Global plugins
plugins:
  - "codex-fix"
response-plugins:
  - "openai-responses"

# Named rule groups (higher priority, separate matching)
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

### Config Source Priority (low → high)

1. **Global**: `~/.config/devproxy/global.yaml`
2. **Directory**: `devproxy.yaml` or `.devproxy.yaml` in current working directory
3. **Explicit**: `--config <path>`
4. **CLI flags**: `--match`, `--overwrite`, etc.

Lists (match, overwrite, plugins, rules) are **accumulated** across sources. Single values (port, upstream, verbose) use last-wins.

### CLI Flags

| Flag | Short | Description | Example |
|------|------|------|------|
| `--config` | `-c` | Config file path (YAML) | `--config config.yaml` |
| `--match` | - | URL matching rule (can be multiple) | `--match "domain.com/api"` |
| `--overwrite` | - | Header overwrite (format: `header=value`) | `--overwrite ua=Bot` |
| `--upstream` | - | Upstream proxy address | `--upstream http://127.0.0.1:7890` |
| `--port` | - | Specify proxy port (default random) | `--port 8888` |
| `--verbose` | `-V` | Detailed log output | `--verbose` |
| `--version` | `-v` | Show version | `devproxy -v` |

## 🏗️ Architecture

Dev Proxy uses a **two-process model**:

- **Parent process**: CLI entry point (Cobra), config loading, spawns proxy worker + target command
- **Proxy worker process** (`__internal_proxy_worker`): Runs the MITM proxy server in an isolated subprocess for log isolation

**Request flow:**
1. Target process sends HTTP/HTTPS request (via injected env vars)
2. Proxy receives → URL normalization → rule matching → plugin execution → header rewriting → forward to upstream
3. Upstream responds → rule matching → response plugin execution → return to client

## 📁 Project Structure

```
devproxy/
├── main.go                    # Entry point
├── cmd/root.go                # Cobra CLI: config loading, process orchestration
├── pkg/config/config.go       # YAML config loading (RuleConfig struct)
├── pkg/proxy/
│   ├── plugin.go              # RequestPlugin/ResponsePlugin interfaces & registries
│   ├── plugin_responses_api.go    # Responses → Chat Completions
│   ├── plugin_openai_responses.go # Chat Completions → Responses (reverse)
│   ├── plugin_codex.go            # Content flattening + model substitution
│   ├── plugin_anthropic_thinking.go # Anthropic stream lifecycle fix
│   ├── plugin_force_stream.go     # Force stream:true
│   ├── server.go              # ProxyServer core (MITM, rule engine)
│   ├── matcher.go             # URLMatcher, RegexMatcher, StringMatcher
│   ├── rewriter.go            # Header rewriting with pre-computed bytes
│   ├── buffer_pool.go         # sync.Pool-based buffer recycling
│   ├── cert_cache.go          # LRU+TTL TLS certificate cache
├── pkg/process/
│   ├── launcher.go            # Env var injection, subprocess lifecycle
│   ├── certs.go               # Export goproxy CA cert to temp file
├── pkg/util/                  # Version, port, ANSI, FD hijacking
├── examples/                  # Sample configs and test scripts
├── docs/                      # Architecture docs
├── scripts/                   # Benchmarks and test scripts
├── Makefile                   # Build, release, cross-build targets
```

## 🔧 Development

```bash
make build          # Standard build with version injection
make build-opt      # Optimized build (-s -w flags, smaller binary)
make cross-build    # Cross-compile for linux/darwin/windows (amd64/arm64)
make release        # Build-opt + install to ~/.local/bin/
make clean          # Remove binaries and build dir

# Tests
go test ./pkg/proxy/   # Run proxy package tests
go test ./...           # Run all tests
```

### Adding a New Plugin

1. Create `pkg/proxy/plugin_<name>.go`
2. Implement `RequestPlugin` interface (and optionally `ResponsePlugin`)
3. Register in `plugin.go` init(): `RegisterPlugin(&YourPlugin{})`
4. Add tests in `pkg/proxy/plugin_<name>_test.go`

## 🛡️ Security

> **⚠️ Warning**: This tool sets environment variables like `NODE_TLS_REJECT_UNAUTHORIZED=0` to bypass HTTPS certificate verification. **Only use in development and testing environments.** Do not use in production.

## 📄 License

MIT License - see the [LICENSE](LICENSE) file for details.