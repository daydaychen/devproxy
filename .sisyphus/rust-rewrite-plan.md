# Smart Proxy Rust 重写计划

## 1. 项目分析总结

### 原始Go项目概述
- **名称**: Smart Proxy
- **功能**: 智能MITM代理工具，拦截并修改HTTP/HTTPS请求头
- **代码规模**: 1067行Go代码，11个源文件
- **核心特性**:
  - HTTPS MITM支持（自动解密）
  - URL匹配（正则+字符串）
  - 请求头重写
  - 上游代理支持
  - 随机端口分配
  - 进程隔离（只代理子进程）
  - PTY交互式支持（vim、bash等）
  - 多级配置系统

### Go项目架构
```
devproxy/
├── main.go              # 入口
├── cmd/root.go          # CLI + 主逻辑（双进程架构）
├── pkg/
│   ├── config/          # YAML配置
│   ├── proxy/           # MITM代理核心
│   ├── process/         # 子进程管理
│   └── util/            # 工具函数
```

### 关键依赖
- `elazarl/goproxy` - MITM代理
- `spf13/cobra` - CLI框架
- `creack/pty` - PTY支持
- `gopkg.in/yaml.v3` - YAML解析

---

## 2. Rust重写设计方案

### 2.1 目录结构
```
rust-devproxy/
├── Cargo.toml
├── src/
│   ├── main.rs              # 程序入口
│   ├── cli.rs               # CLI参数解析 (clap)
│   ├── config/
│   │   ├── mod.rs
│   │   ├── loader.rs        # 配置加载
│   │   └── types.rs         # 配置结构
│   ├── proxy/
│   │   ├── mod.rs
│   │   ├── server.rs        # 代理服务器
│   │   ├── handler.rs       # 请求处理器
│   │   ├── matcher.rs       # URL匹配
│   │   ├── rewriter.rs      # 头重写
│   │   ├── mitm.rs          # MITM证书
│   │   └── upstream.rs      # 上游代理
│   ├── process/
│   │   ├── mod.rs
│   │   ├── launcher.rs      # 子进程启动
│   │   ├── env.rs           # 环境变量
│   │   └── pty.rs           # PTY支持
│   └── utils/
│       ├── mod.rs
│       ├── port.rs          # 端口分配
│       ├── ansi.rs          # ANSI处理
│       ├── io.rs            # IO工具
│       ├── logging.rs       # 日志系统
│       └── version.rs       # 版本信息
└── assets/
    └── ca_cert.pem          # MITM CA证书
```

### 2.2 核心模块设计

#### CLI模块 (`cli.rs`)
```rust
use clap::{Parser, Subcommand};

#[derive(Parser)]
pub struct Cli {
    #[arg(short, long)]
    pub config: Option<String>,
    #[arg(long)]
    pub match_pattern: Vec<String>,
    #[arg(long)]
    pub overwrite: Vec<String>,
    #[arg(long)]
    pub upstream: Option<String>,
    #[arg(short, long)]
    pub port: u16,
    #[arg(short = 'V', long)]
    pub verbose: bool,
    #[arg(long)]
    pub log_file: Option<String>,
    #[command(trailing_var_arg = true)]
    pub args: Vec<String>,
}

#[derive(Subcommand)]
pub enum Commands {
    #[command(name = "__internal_proxy_worker", hide = true)]
    ProxyWorker,
}
```

#### 配置模块 (`config/types.rs`)
```rust
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct RuleConfig {
    pub name: String,
    pub match_patterns: Vec<String>,
    pub overwrite: std::collections::HashMap<String, String>,
}

#[derive(Debug, Clone, Deserialize, Serialize, Default)]
pub struct Config {
    pub rules: Vec<RuleConfig>,
    pub match_patterns: Vec<String>,
    pub overwrite: std::collections::HashMap<String, String>,
    pub upstream: Option<String>,
    pub port: u16,
    pub verbose: bool,
    pub log_file: Option<String>,
}
```

#### 代理服务器 (`proxy/server.rs`)
```rust
use tokio::net::TcpListener;
use std::sync::Arc;
use tokio::sync::RwLock;

pub struct ProxyServer {
    port: u16,
    upstream: Option<String>,
    rules: Arc<RwLock<Vec<ProxyRule>>>,
    default_rule: Arc<RwLock<ProxyRule>>,
    verbose: bool,
    ca_cert: Arc<mitm::CACert>,
}

pub struct ProxyRule {
    pub name: String,
    pub matchers: Vec<Box<dyn matcher::Matcher + Send + Sync>>,
    pub rewriters: Vec<rewriter::HeaderRewriter>,
}

impl ProxyServer {
    pub async fn start(&self) -> Result<(), Box<dyn std::error::Error>> {
        let listener = TcpListener::bind(format!("127.0.0.1:{}", self.port)).await?;
        loop {
            let (stream, addr) = listener.accept().await?;
            let rules = self.rules.clone();
            let default_rule = self.default_rule.clone();
            let verbose = self.verbose;
            let upstream = self.upstream.clone();
            let ca_cert = self.ca_cert.clone();

            tokio::spawn(async move {
                handler::handle_connection(stream, addr, rules, default_rule, upstream, verbose, ca_cert).await;
            });
        }
    }
}
```

#### URL匹配器 (`proxy/matcher.rs`)
```rust
pub trait Matcher: Send + Sync {
    fn match_url(&self, url: &str) -> bool;
}

pub struct RegexMatcher {
    pattern: regex::Regex,
}

pub struct StringMatcher {
    pattern: String,
}

impl RegexMatcher {
    pub fn new(pattern: &str) -> Result<Self, regex::Error> {
        Ok(Self { pattern: regex::Regex::new(pattern)? })
    }
}

impl Matcher for RegexMatcher {
    fn match_url(&self, url: &str) -> bool {
        self.pattern.is_match(url)
    }
}

impl StringMatcher {
    pub fn new(pattern: String) -> Self {
        Self { pattern }
    }
}

impl Matcher for StringMatcher {
    fn match_url(&self, url: &str) -> bool {
        url.contains(&self.pattern)
    }
}
```

#### HTTP头重写器 (`proxy/rewriter.rs`)
```rust
pub struct HeaderRewriter {
    pub header_name: String,
    pub header_value: String,
}

impl HeaderRewriter {
    pub fn new(name: String, value: String) -> Self {
        let normalized_name = match name.to_lowercase().as_str() {
            "useragent" | "ua" => "User-Agent".to_string(),
            "referer" => "Referer".to_string(),
            "origin" => "Origin".to_string(),
            _ => name,
        };
        Self {
            header_name: normalized_name,
            header_value: value,
        }
    }

    pub fn rewrite(&self, headers: &mut http::HeaderMap) {
        if let Ok(name) = http::header::HeaderName::from_bytes(self.header_name.as_bytes()) {
            headers.insert(name, http::HeaderValue::from_str(&self.header_value).unwrap_or_default());
        }
    }
}
```

#### MITM证书处理 (`proxy/mitm.rs`)
```rust
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use rcgen::{Certificate, CertificateParams, KeyPair};

pub struct CACert {
    cert: Certificate,
    key_pair: KeyPair,
}

impl CACert {
    pub fn generate() -> Result<Self, Box<dyn std::error::Error>> {
        let mut params = CertificateParams::new(vec!["Smart Proxy CA".to_string()]);
        params.is_ca = rcgen::IsCa::Ca(rcgen::BasicConstraints::Unconstrained);
        let key_pair = KeyPair::generate()?;
        params.key_pair = Some(key_pair);
        let cert = Certificate::from_params(params)?;
        Ok(Self { cert, key_pair })
    }

    pub fn generate_server_cert(&self, domain: &str) -> Result<(Vec<u8>, Vec<u8>), Box<dyn std::error::Error>> {
        let mut params = CertificateParams::new(vec![domain.to_string()]);
        let key_pair = KeyPair::generate()?;
        params.key_pair = Some(key_pair);
        params.distinguished_name = rcgen::DistinguishedName::new();
        params.distinguished_name.push(rcgen::DnType::CommonName, domain);

        let cert = Certificate::from_params(params)?;
        let cert_pem = cert.serialize_pem_with_signer(&self.cert)?;
        let key_pem = key_pair.serialize_pem();
        Ok((cert_pem.into_bytes(), key_pem.into_bytes()))
    }
}
```

#### 进程启动器 (`process/launcher.rs`)
```rust
use tokio::process::Command;

pub struct ProcessLauncher {
    command: String,
    args: Vec<String>,
    proxy_port: u16,
    verbose: bool,
}

impl ProcessLauncher {
    pub async fn start(&self) -> Result<tokio::process::Child, std::io::Error> {
        let proxy_url = format!("http://127.0.0.1:{}", self.proxy_port);
        let mut cmd = Command::new(&self.command);
        cmd.args(&self.args)
            .env("HTTP_PROXY", &proxy_url)
            .env("HTTPS_PROXY", &proxy_url)
            .env("ALL_PROXY", &proxy_url)
            .env("NODE_TLS_REJECT_UNAUTHORIZED", "0")
            .env("BUN_TLS_REJECT_UNAUTHORIZED", "0")
            .env("npm_config_strict_ssl", "false");

        if is_interactive_terminal() {
            self.start_with_pty(cmd).await
        } else {
            cmd.spawn()
        }
    }
}
```

### 2.3 Cargo.toml配置
```toml
[package]
name = "devproxy"
version = "2.0.0"
edition = "2021"

[dependencies]
# 异步运行时
tokio = { version = "1.43", features = ["full"] }
tokio-util = { version = "0.7", features = ["codec"] }

# HTTP处理
hyper = { version = "1.5", features = ["full"] }
hyper-util = { version = "0.1", features = ["full"] }
http = "1.2"
http-body-util = "0.1"

# TLS/SSL
rustls = { version = "0.23", features = ["ring"] }
tokio-rustls = "0.26"
rcgen = "0.13"

# HTTP客户端
reqwest = { version = "0.12", features = ["rustls-tls", "http2"] }

# CLI框架
clap = { version = "4.5", features = ["derive", "cargo"] }

# 配置
serde = { version = "1.0", features = ["derive"] }
serde_yaml = "0.9"

# 其他
regex = "1.11"
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["env-filter", "fmt"] }
crossterm = "0.28"
lazy_static = "1.5"
anyhow = "1.0"
thiserror = "2.0"
url = "2.5"

[profile.release]
opt-level = 3
lto = true
codegen-units = 1
panic = "abort"
strip = true
```

---

## 3. 实现计划

### 阶段1: 项目初始化和基础工具
- [ ] 创建Rust项目结构
- [ ] 配置Cargo.toml依赖
- [ ] 实现utils模块（port, ansi, io, logging）

### 阶段2: 配置系统
- [ ] 实现config模块（YAML/TOML加载）
- [ ] 多级配置合并逻辑
- [ ] CLI参数解析

### 阶段3: 代理核心
- [ ] 实现matcher模块（字符串+正则）
- [ ] 实现rewriter模块（头重写）
- [ ] 实现MITM证书生成

### 阶段4: HTTP代理服务器
- [ ] 实现proxy/server.rs
- [ ] 实现proxy/handler.rs
- [ ] 实现上游代理支持

### 阶段5: 进程管理
- [ ] 实现process/launcher.rs
- [ ] 实现process/pty.rs
- [ ] 实现信号处理

### 阶段6: 集成和测试
- [ ] 实现main.rs主流程
- [ ] 编写单元测试
- [ ] 编写集成测试
- [ ] 性能基准测试

### 阶段7: 文档和发布
- [ ] 编写README
- [ ] 创建Makefile
- [ ] 配置CI/CD
- [ ] 发布到crates.io

---

## 4. 风险评估

### 技术风险
- **MITM证书兼容性**: 中风险，需要测试主流浏览器
- **性能**: 低风险，Tokio异步模型更高效
- **依赖库成熟度**: 中风险，选择成熟crates

### 依赖风险
- `tokio-pty-process`: 维护不活跃，可能需要自己实现
- `rcgen`: 稳定，有替代方案（openssl）

---

## 5. 改进点（相比Go版本）
- 零拷贝优化（使用bytes crate）
- 内存安全（Rust所有权系统）
- 更好的并发（Tokio work-stealing调度器）
- 更小的二进制（静态链接）
- 支持TOML配置（更现代）
- 热重载配置（可选）
- Web界面管理（可选）

---

*计划创建日期: 2026-01-31*
*计划版本: 1.0*
