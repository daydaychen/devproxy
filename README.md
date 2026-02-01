<div align="center">

# Dev Proxy

[![Go Version](https://img.shields.io/badge/Go-1.25-blue?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/daydaychen/smart-proxy)](https://goreportcard.com/report/github.com/daydaychen/smart-proxy)

一个用 Go 实现的智能 MITM 代理工具，能够拦截并修改指定 URL 的 HTTP/HTTPS 请求。

[English](README.md) | [中文](README_CN.md)

</div>

## ✨ 核心特性

- 🔒 **HTTPS MITM 支持** - 自动拦截和解密 HTTPS 流量
- 🎯 **URL 匹配** - 支持正则表达式和字符串匹配
- ✏️ **请求头重写** - 灵活修改 User-Agent 等请求头
- 🔄 **上游代理** - 支持转发到 HTTP/SOCKS 代理（如 Clash）
- 🎲 **随机端口** - 自动分配可用端口，避免冲突
- 🔐 **进程隔离** - 只代理启动的子进程，不影响系统其他程序
- 💻 **交互式应用** - 支持 vim、bash 等交互式程序
- 📝 **详细日志** - 可选的详细日志输出，方便调试

## 🚀 快速开始

### 安装

**使用 Go 安装:**

```bash
go install github.com/daydaychen/smart-proxy@latest
```

**从源码编译:**

```bash
git clone https://github.com/daydaychen/smart-proxy.git
cd devproxy
make build
./devproxy --help
```

### 基本用法

```bash
devproxy [flags] -- <command> [args...]
```

**最简单的示例:**

```bash
# 使用命令行参数
devproxy --match "httpbin.org" --overwrite useragent=MyBot -- curl http://httpbin.org/headers

# 使用配置文件
devproxy --config config.yaml -- curl http://httpbin.org/headers
```

## 📖 使用示例

### 1. 使用配置文件 (YAML)

这是推荐的使用方式，可以将常用规则持久化。支持多级配置，加载顺序及优先级如下（后者覆盖前者，但列表类配置会累加）：

1. **全局配置**: `~/.config/devproxy/global.yaml` (存放通用规则)
2. **目录配置**: 当前目录下的 `devproxy.yaml` 或 `.devproxy.yaml` (存放项目特定规则)
3. **显式配置**: 通过 `--config` 指定的文件 (优先级高于默认目录配置)
4. **命令行参数**: 优先级最高。

#### 规则组模式 (推荐)

你可以将 `match` 和 `overwrite` 成组设置，实现针对不同域名的差异化修改：

```yaml
rules:
  - name: "google-api"
    match: ["google.com/api"]
    overwrite:
      Authorization: "Bearer token1"
  - name: "github-api"
    match: ["github.com"]
    overwrite:
      Authorization: "token2"
      User-Agent: "GithubBot"

# 全局通用规则 (对所有匹配请求生效)
match: ["*"]
overwrite:
  X-Smart-Proxy: "v1"
```

运行:

```bash
devproxy -- node server.js  # 自动集成全局和当前的 devproxy.yaml
```

### 2. 基本代理 + UA 重写

```bash
devproxy \
    --match "example.com/api" \
    --overwrite useragent=CustomUA \
    -- node server.js
```

### 3. 多个匹配规则

```bash
devproxy \
    --match "domain1.com" \
    --match "domain2.com/v1" \
    --overwrite useragent=Bot/1.0 \
    -- npm start
```

### 4. 使用上游代理（转发到 Clash）

```bash
devproxy \
    --upstream http://127.0.0.1:7890 \
    --match "google.com" \
    --overwrite useragent=ProxyBot \
    -- curl https://google.com
```

### 5. 指定端口 + 详细日志

```bash
devproxy \
    --port 8888 \
    --match "/api/" \
    --overwrite useragent=Test \
    --verbose \
    -- node app.js
```

### 6. 重写多个请求头

```bash
devproxy \
    --match "api.example.com" \
    --overwrite useragent=CustomBot \
    --overwrite referer=https://example.com \
    --overwrite origin=https://example.com \
    -- python script.py
```

### 7. 交互式应用（vim、bash 等）

```bash
# 在代理环境下运行 vim
devproxy --match "githubusercontent.com" --verbose -- vim

# 在代理环境下运行交互式 bash
devproxy --upstream http://127.0.0.1:7890 -- bash

# 在代理环境下运行 Python 交互式解释器
devproxy --match "pypi.org" -- python3
```

## ⚙️ 命令行参数

| 参数 | 简写 | 说明 | 示例 |
|------|------|------|------|
| `--config` | `-c` | 配置文件路径 (YAML) | `--config config.yaml` |
| `--match` | - | URL 匹配规则（可多次指定） | `--match "domain.com/api"` |
| `--overwrite` | - | 请求头重写（格式: `header=value`） | `--overwrite useragent=Bot` |
| `--upstream` | - | 上游代理地址 | `--upstream http://127.0.0.1:7890` |
| `--port` | - | 指定代理端口（默认随机） | `--port 8888` |
| `--verbose` | `-V` | 详细日志输出 | `--verbose` |
| `--log-file` | - | 日志文件路径 | `--log-file proxy.log` |
| `--version` | `-v` | 查看版本号 | `devproxy -v` |

### 请求头简写

为了方便使用，支持以下简写：

- `useragent` / `ua` → `User-Agent`
- `referer` → `Referer`
- `origin` → `Origin`

其他请求头请使用完整名称，如 `Authorization`、`Cookie` 等。

## 🔧 工作原理

1. **启动代理服务器** - devproxy 在随机端口（或指定端口）启动 MITM 代理
2. **注入环境变量** - 为子进程设置代理环境变量：

```bash
HTTP_PROXY=http://127.0.0.1:<port>
HTTPS_PROXY=http://127.0.0.1:<port>
ALL_PROXY=http://127.0.0.1:<port>
NODE_TLS_REJECT_UNAUTHORIZED=0
```

3. **启动子进程** - 启动你指定的命令（target-node）
4. **拦截和修改** - 根据 URL 匹配规则修改请求头
5. **转发流量** - 将流量转发到上游代理（可选）或直接发送

## 🛡️ 安全说明

> **⚠️ 警告**: 此工具设置了 `NODE_TLS_REJECT_UNAUTHORIZED=0` 来绕过 HTTPS 证书验证，**仅适用于开发和测试环境**。请勿在生产环境中使用。

## 🎯 实际应用场景

### 场景 1: 爬虫开发

某些网站会检测 User-Agent，使用 devproxy 可以轻松修改：

```bash
devproxy \
    --match "target-site.com" \
    --overwrite useragent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36" \
    -- node crawler.js
```

### 场景 2: API 测试

需要在请求中添加特定的 Referer 或 Origin：

```bash
devproxy \
    --match "api.example.com" \
    --overwrite referer=https://example.com \
    --overwrite origin=https://example.com \
    -- curl https://api.example.com/data
```

### 场景 3: 国际化测试

通过上游代理切换 IP 地址，测试不同地区的 API 响应：

```bash
devproxy \
    --upstream http://127.0.0.1:7890 \
    --match "geo-api.com" \
    -- node test-geo.js
```

## 📁 项目结构

```
devproxy/
├── main.go                 # 程序入口
├── cmd/
│   └── root.go            # 命令行定义和主逻辑
├── pkg/
│   ├── config/
│   │   └── config.go      # 配置文件加载
│   ├── proxy/
│   │   ├── server.go      # MITM 代理服务器
│   │   ├── matcher.go     # URL 匹配引擎
│   │   └── rewriter.go    # 请求头重写器
│   ├── process/
│   │   └── launcher.go    # 子进程启动器
│   └── util/
│       └── port.go        # 随机端口分配
├── examples/              # 示例配置文件
├── Makefile
├── go.mod
└── README.md
```

## 🧰 技术栈

- **Go** - 编程语言
- [elazarl/goproxy](https://github.com/elazarl/goproxy) - MITM 代理库
- [spf13/cobra](https://github.com/spf13/cobra) - 命令行框架

## 📝 开发

```bash
# 克隆项目
git clone https://github.com/daydaychen/smart-proxy.git
cd devproxy

# 安装依赖
go mod download

# 运行测试
go test ./...

# 代码检查
golangci-lint run ./...

# 编译
make build

# 运行
./devproxy --help
```

## 🐛 故障排查

### 问题 1: 证书错误

如果遇到 SSL/TLS 证书错误，确保：

- `NODE_TLS_REJECT_UNAUTHORIZED=0` 已设置（devproxy 会自动设置）
- 如果是其他语言（Python、Ruby 等），可能需要额外配置

### 问题 2: 代理不生效

确认：

1. 子进程是否支持 `HTTP_PROXY` 环境变量
2. 使用 `--verbose` 查看详细日志
3. 检查 URL 匹配规则是否正确

### 问题 3: 端口冲突

使用 `--port` 参数指定一个未被占用的端口。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！请先阅读[贡献指南](CONTRIBUTING.md)。

## 📄 许可

MIT License - 详见 [LICENSE](LICENSE) 文件。

## 🛡️ 安全

如发现安全漏洞，请阅读 [安全策略](SECURITY.md) 获取报告方式。
