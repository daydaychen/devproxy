# Smart-Proxy

一个用 Go 实现的智能 MITM 代理工具，能够拦截并修改指定 URL 的 HTTP/HTTPS 请求。

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

```bash
# 编译项目
go build -o smart-proxy

# 或者直接运行
go run main.go -- <command>
```

### 基本用法

```bash
smart-proxy [flags] -- <command> [args...]
```

**最简单的示例**:

```bash
# 代理 curl 请求，重写 User-Agent
smart-proxy --match "httpbin.org" --overwrite useragent=MyBot -- curl http://httpbin.org/headers
```

## 📖 使用示例

### 1. 基本代理 + UA 重写

```bash
smart-proxy \
    --match "example.com/api" \
    --overwrite useragent=CustomUA \
    -- node server.js
```

### 2. 多个匹配规则

```bash
smart-proxy \
    --match "domain1.com" \
    --match "domain2.com/v1" \
    --overwrite useragent=Bot/1.0 \
    -- npm start
```

### 3. 使用上游代理（转发到 Clash）

```bash
smart-proxy \
    --upstream http://127.0.0.1:7890 \
    --match "google.com" \
    --overwrite useragent=ProxyBot \
    -- curl https://google.com
```

### 4. 指定端口 + 详细日志

```bash
smart-proxy \
    --port 8888 \
    --match "/api/" \
    --overwrite useragent=Test \
    --verbose \
    -- node app.js
```

### 5. 重写多个请求头

```bash
smart-proxy \
    --match "api.example.com" \
    --overwrite useragent=CustomBot \
    --overwrite referer=https://example.com \
    --overwrite origin=https://example.com \
    -- python script.py
```

### 6. Node.js 应用代理

```bash
# 代理 Node.js 应用的所有外部请求
smart-proxy \
    --match "api.github.com" \
    --overwrite useragent=MyGitHubBot \
    -- node -e "require('https').get('https://api.github.com/users/github', res => res.pipe(process.stdout))"
```

### 7. 交互式应用（vim、bash 等）

```bash
# 在代理环境下运行 vim
smart-proxy --match "githubusercontent.com" --verbose -- vim

# 在代理环境下运行交互式 bash
smart-proxy --upstream http://127.0.0.1:7890 -- bash

# 在代理环境下运行 Python 交互式解释器
smart-proxy --match "pypi.org" -- python3
```

## ⚙️ 命令行参数

| 参数 | 简写 | 说明 | 示例 |
|------|------|------|------|
| `--match` | - | URL 匹配规则（可多次指定） | `--match "domain.com/api"` |
| `--overwrite` | - | 请求头重写（格式: `header=value`） | `--overwrite useragent=Bot` |
| `--upstream` | - | 上游代理地址 | `--upstream http://127.0.0.1:7890` |
| `--port` | - | 指定代理端口（默认随机） | `--port 8888` |
| `--verbose` | `-v` | 详细日志输出 | `--verbose` |

### 请求头简写

为了方便使用，支持以下简写：

- `useragent` / `ua` → `User-Agent`
- `referer` → `Referer`
- `origin` → `Origin`

其他请求头请使用完整名称，如 `Authorization`、`Cookie` 等。

## 🔧 工作原理

1. **启动代理服务器** - smart-proxy 在随机端口（或指定端口）启动 MITM 代理
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

某些网站会检测 User-Agent，使用 smart-proxy 可以轻松修改：

```bash
smart-proxy \
    --match "target-site.com" \
    --overwrite useragent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36" \
    -- node crawler.js
```

### 场景 2: API 测试

需要在请求中添加特定的 Referer 或 Origin：

```bash
smart-proxy \
    --match "api.example.com" \
    --overwrite referer=https://example.com \
    --overwrite origin=https://example.com \
    -- curl https://api.example.com/data
```

### 场景 3: 国际化测试

通过上游代理切换 IP 地址，测试不同地区的 API 响应：

```bash
smart-proxy \
    --upstream http://127.0.0.1:7890 \
    --match "geo-api.com" \
    -- node test-geo.js
```

## 📁 项目结构

```
smart-proxy/
├── main.go                 # 程序入口
├── cmd/
│   └── root.go            # 命令行定义和主逻辑
├── pkg/
│   ├── proxy/
│   │   ├── server.go      # MITM 代理服务器
│   │   ├── matcher.go     # URL 匹配引擎
│   │   └── rewriter.go    # 请求头重写器
│   ├── process/
│   │   └── launcher.go    # 子进程启动器
│   └── util/
│       └── port.go        # 随机端口分配
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
cd smart-proxy

# 安装依赖
go mod download

# 运行测试
go test ./...

# 编译
go build -o smart-proxy

# 运行
./smart-proxy --help
```

## 🐛 故障排查

### 问题 1: 证书错误

如果遇到 SSL/TLS 证书错误，确保：
- `NODE_TLS_REJECT_UNAUTHORIZED=0` 已设置（smart-proxy 会自动设置）
- 如果是其他语言（Python、Ruby 等），可能需要额外配置

### 问题 2: 代理不生效

确认：
1. 子进程是否支持 `HTTP_PROXY` 环境变量
2. 使用 `--verbose` 查看详细日志
3. 检查 URL 匹配规则是否正确

### 问题 3: 端口冲突

使用 `--port` 参数指定一个未被占用的端口。

## 📄 许可

MIT License

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！
