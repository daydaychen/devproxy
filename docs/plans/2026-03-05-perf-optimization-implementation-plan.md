---
title: DevProxy 性能优化实施计划
type: feat
status: active
date: 2026-03-05
---

# DevProxy 性能优化实施计划

## 概述

本计划针对 devproxy 代理服务器进行系统性性能优化，目标是减少内存分配、降低 GC 压力、提升吞吐量和降低延迟。通过代码分析识别出 12 个优化点，按优先级分三个阶段实施。

## 问题陈述

### 当前性能瓶颈

通过代码分析发现以下关键性能问题：

1. **高频内存分配**: `io.ReadAll` 和 `bytes.NewReader` 每次请求都分配新 buffer
2. **重复字符串操作**: URL 规范化、Header 查找、ShouldMITM 判断存在大量重复计算
3. **串行插件执行**: 多个插件串行处理，无法充分利用多核
4. **线性规则匹配**: 规则数量增长时匹配性能线性下降
5. **连接池未优化**: 默认 `http.Transport` 配置未针对代理场景调优

### 影响

- GC 频繁导致延迟抖动
- 高并发下吞吐量上不去
- 内存占用持续增长

## 优化方案

### 第一阶段：高优先级优化（快速收益）

#### 1.1 Buffer Pooling 🥇

**目标**: 减少 60-80% 的内存分配

**实施内容**:

```go
// pkg/proxy/buffer_pool.go
package proxy

import (
    "bytes"
    "sync"
)

// BufferPool 复用 bytes.Buffer
var BufferPool = sync.Pool{
    New: func() interface{} {
        return bytes.NewBuffer(make([]byte, 0, 4096)) // 预分配 4KB
    },
}

// GetBuffer 从池中获取 buffer
func GetBuffer() *bytes.Buffer {
    return BufferPool.Get().(*bytes.Buffer)
}

// PutBuffer 归还 buffer 到池中
func PutBuffer(buf *bytes.Buffer) {
    buf.Reset()
    BufferPool.Put(buf)
}
```

**修改位置**:
- `server.go:154-167` 请求体读取
- `plugin_codex.go:44-51` JSON 处理
- `plugin_openai_responses.go:127-135` 响应体处理

**预期收益**: GC 暂停时间降低 40%+

---

#### 1.2 Header 查找优化 🥇

**目标**: 减少字符串重复计算

**实施内容**:

```go
// pkg/proxy/rewriter.go
type HeaderRewriter struct {
    HeaderName   string
    HeaderValue  string
    headerKey    string  // 缓存标准化后的 key
}

func NewHeaderRewriter(name, value string) *HeaderRewriter {
    return &HeaderRewriter{
        HeaderName:  name,
        HeaderValue: value,
        headerKey:   http.CanonicalHeaderKey(name), // 预计算
    }
}

func (r *HeaderRewriter) Rewrite(req *http.Request) {
    req.Header[r.headerKey] = []string{r.HeaderValue}
}
```

**预期收益**: 每次 Rewrite 减少 1-2 次字符串操作

---

#### 1.3 URL 规范化缓存 🥇

**目标**: 避免重复 URL 处理

**实施内容**:

```go
// pkg/proxy/matcher.go
import "sync"

var urlNormCache = sync.Map{} // map[string]string

func NormalizeURL(u string) string {
    // 快速路径：无默认端口
    if !strings.Contains(u, ":443") && !strings.Contains(u, ":80") {
        return u
    }
    
    // 查缓存
    if cached, ok := urlNormCache.Load(u); ok {
        return cached.(string)
    }

    // 执行规范化
    result := normalizeURLSlow(u)

    // 写缓存 (限制大小)
    if urlNormCache.Len() < 10000 {
        urlNormCache.Store(u, result)
    }

    return result
}
```

**预期收益**: 缓存命中率 80%+ 场景下性能提升 3-5 倍

---

#### 1.4 插件链式管道 🥇

**目标**: 减少 Body 重复读取

**实施内容**:

```go
// pkg/proxy/server.go
func (s *ProxyServer) executePluginsPipeline(rule *ProxyRule, req *http.Request) error {
    // 预读取 body (只读一次)
    originalBody, err := io.ReadAll(req.Body)
    if err != nil {
        return err
    }

    for _, plugin := range rule.Plugins {
        req.Body = io.NopCloser(bytes.NewReader(originalBody))
        req.GetBody = func() (io.ReadCloser, error) {
            return io.NopCloser(bytes.NewReader(originalBody)), nil
        }
        
        if err := plugin.ProcessRequest(req); err != nil {
            return err
        }
        
        // 收集插件修改后的 body 供下一个插件使用
        originalBody, _ = io.ReadAll(req.Body)
    }

    // 最终设置
    req.Body = io.NopCloser(bytes.NewReader(originalBody))
    req.ContentLength = int64(len(originalBody))
    return nil
}
```

**预期收益**: 减少 50%+ 的 Body 读取次数

---

### 第二阶段：中优先级优化（架构改进）

#### 2.1 ShouldMITM 索引优化 🥈

**目标**: CONNECT 请求判断从 O(n*m) 降至 O(1)

**实施内容**:

```go
// pkg/proxy/server.go
type ProxyServer struct {
    // ... 现有字段
    mitmHosts      map[string]bool  // 预计算的 MITM 域名集合
    mitmPatterns   []*StringMatcher // 预计算的模式匹配器
    hasRegexRule   bool             // 是否有正则规则
    hasGlobalRule  bool             // 是否有全局规则
}

// AddRule 时预计算
func (s *ProxyServer) AddRule(rule *ProxyRule) {
    s.Rules = append(s.Rules, rule)
    s.rebuildMITMIndex()
}

func (s *ProxyServer) rebuildMITMIndex() {
    s.mitmHosts = make(map[string]bool)
    s.mitmPatterns = nil
    s.hasRegexRule = false
    s.hasGlobalRule = false

    for _, rule := range s.Rules {
        if len(rule.Matchers) == 0 && (len(rule.Rewriters) > 0 || len(rule.Plugins) > 0) {
            s.hasGlobalRule = true
            continue
        }

        for _, matcher := range rule.Matchers {
            if _, ok := matcher.(*RegexMatcher); ok {
                s.hasRegexRule = true
                return // 有正则规则，默认全部 MITM
            }

            if sm, ok := matcher.(*StringMatcher); ok {
                if !strings.HasPrefix(sm.Pattern, "/") && !strings.Contains(sm.Pattern, "/") {
                    s.mitmHosts[sm.Pattern] = true
                } else {
                    s.mitmPatterns = append(s.mitmPatterns, sm)
                }
            }
        }
    }
}

// 优化后的 ShouldMITM
func (s *ProxyServer) ShouldMITM(host string) bool {
    domain := host
    if pos := strings.Index(host, ":"); pos != -1 {
        domain = host[:pos]
    }

    if s.mitmHosts[domain] {
        return true
    }
    if s.hasGlobalRule {
        return true
    }
    if s.hasRegexRule {
        return true
    }
    for _, sm := range s.mitmPatterns {
        if sm.Match(domain) {
            return true
        }
    }
    return false
}
```

---

#### 2.2 Transport 连接池优化 🥈

**目标**: 提升连接复用率，降低延迟

**实施内容**:

```go
// pkg/proxy/server.go
func NewProxyServer(port int, upstream string, verbose bool, logger *log.Logger) *ProxyServer {
    // ... 现有代码

    transport := &http.Transport{
        // 连接池
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,

        // 连接建立超时
        DialContext: (&net.Dialer{
            Timeout:   30 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,

        // TLS 配置
        TLSHandshakeTimeout:   10 * time.Second,
        ResponseHeaderTimeout: 30 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,

        // 复用连接
        ForceAttemptHTTP2: true,

        // 禁用压缩（让插件处理）
        DisableCompression: true,
    }

    proxy := goproxy.NewProxyHttpServer()
    proxy.Tr = transport
    // ...
}
```

**预期收益**: 连接复用率提升 60%+, 延迟降低 30%+

---

#### 2.3 TLS 证书缓存 🥈

**目标**: 减少重复域名 TLS 握手时间

**实施内容**:

```go
// pkg/proxy/cert_cache.go
package proxy

import (
    "crypto/tls"
    "sync"
    "time"
)

type CertCache struct {
    mu     sync.RWMutex
    certs  map[string]*tls.Certificate
    ttl    time.Duration
    expiry map[string]time.Time
}

func NewCertCache(ttl time.Duration) *CertCache {
    return &CertCache{
        certs:  make(map[string]*tls.Certificate),
        expiry: make(map[string]time.Time),
        ttl:    ttl,
    }
}

func (c *CertCache) Get(host string) (*tls.Certificate, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    cert, exists := c.certs[host]
    if !exists || time.Now().After(c.expiry[host]) {
        return nil, false
    }
    return cert, true
}

func (c *CertCache) Set(host string, cert *tls.Certificate) {
    c.mu.Lock()
    defer c.mu.Unlock()

    if len(c.certs) > 1000 {
        c.evictExpired()
    }
    c.certs[host] = cert
    c.expiry[host] = time.Now().Add(c.ttl)
}
```

---

#### 2.4 规则索引加速 🥈

**目标**: 规则多时匹配性能提升 10 倍+

**实施内容**:

```go
// pkg/proxy/rule_index.go
package proxy

import (
    "strings"
    "sync"
)

type RuleIndex struct {
    mu          sync.RWMutex
    hostRules   map[string][]*ProxyRule
    pathRules   []*ProxyRule
    globalRules []*ProxyRule
    regexRules  []*ProxyRule
}

func (ri *RuleIndex) Match(url string) *ProxyRule {
    ri.mu.RLock()
    defer ri.mu.RUnlock()

    // 1. 全局规则优先
    for _, rule := range ri.globalRules {
        return rule
    }

    // 2. 提取 host
    host := extractHost(url)

    // 3. Host 精确匹配 (O(1))
    if rules, ok := ri.hostRules[host]; ok {
        return rules[0]
    }

    // 4-6. 其他匹配...
}
```

---

### 第三阶段：低优先级优化（代码质量）

#### 3.1 日志级别系统 🥉

**目标**: 更清晰的日志控制，减少运行时开销

**实施内容**:

```go
// pkg/proxy/logger.go
package proxy

type LogLevel int

const (
    LevelSilent LogLevel = iota
    LevelError
    LevelWarn
    LevelInfo
    LevelDebug
)

type Logger struct {
    *log.Logger
    level LogLevel
}

func (l *Logger) Debug(format string, v ...interface{}) {
    if l.level >= LevelDebug {
        l.Printf(format, v...)
    }
}
// ... 其他级别类似
```

---

#### 3.2 错误处理完善 🥉

**目标**: 完善错误处理和监控

**实施内容**:

- 添加自定义错误类型
- 记录错误统计指标
- 添加错误恢复机制

---

#### 3.3 JSON 库替换 🥉

**目标**: 验证更快的 JSON 库收益

**评估选项**:
- `github.com/goccy/go-json`
- `github.com/json-iterator/go`

**注意**: 需验证兼容性

---

## 实施计划

### 阶段一：基础优化（Week 1-2）

| 任务 | 负责人 | 预计工时 | 依赖 |
|------|--------|---------|------|
| 1.1 Buffer Pooling | - | 4h | 无 |
| 1.2 Header 查找优化 | - | 2h | 无 |
| 1.3 URL 规范化缓存 | - | 4h | 无 |
| 1.4 插件链式管道 | - | 6h | 1.1 |
| **阶段一小计** | | **16h** | |

### 阶段二：架构优化（Week 3-4）

| 任务 | 负责人 | 预计工时 | 依赖 |
|------|--------|---------|------|
| 2.1 ShouldMITM 索引 | - | 6h | 无 |
| 2.2 Transport 优化 | - | 4h | 无 |
| 2.3 TLS 证书缓存 | - | 8h | 无 |
| 2.4 规则索引加速 | - | 12h | 无 |
| **阶段二小计** | | **30h** | |

### 阶段三：代码质量（Week 5）

| 任务 | 负责人 | 预计工时 | 依赖 |
|------|--------|---------|------|
| 3.1 日志级别系统 | - | 4h | 无 |
| 3.2 错误处理完善 | - | 6h | 无 |
| 3.3 JSON 库评估 | - | 4h | 无 |
| **阶段三小计** | | **14h** | |

---

## 测试计划

### 基准测试

创建性能基准测试文件 `pkg/proxy/benchmark_test.go`:

```go
package proxy

import (
    "bytes"
    "net/http"
    "net/http/httptest"
    "testing"
)

func BenchmarkBufferPool(b *testing.B) {
    for i := 0; i < b.N; i++ {
        buf := GetBuffer()
        buf.WriteString("test data")
        PutBuffer(buf)
    }
}

func BenchmarkHeaderRewriter(b *testing.B) {
    rw := NewHeaderRewriter("X-Custom-Header", "value")
    req := httptest.NewRequest("GET", "http://example.com", nil)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        rw.Rewrite(req)
    }
}

func BenchmarkNormalizeURL(b *testing.B) {
    url := "https://example.com:443/api/v1/users"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        NormalizeURL(url)
    }
}
```

### 性能指标

| 指标 | 优化前 | 目标 | 测量方法 |
|------|--------|------|---------|
| 内存分配/请求 | ~50KB | ~15KB | `testing.AllocsPerRun` |
| GC 暂停时间 | ~5ms | ~2ms | `GODEBUG=gctrace=1` |
| 请求延迟 (p99) | ~50ms | ~30ms | ab/wrk 压测 |
| 吞吐量 (req/s) | ~1000 | ~2000 | ab/wrk 压测 |

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Buffer Pool 并发竞争 | 高 | 使用 `sync.Pool`，充分测试 |
| 缓存内存泄漏 | 中 | 限制缓存大小，定期清理 |
| TLS 证书安全 | 高 | 设置合理 TTL，不缓存敏感证书 |
| 规则索引复杂度 | 中 | 分阶段实施，充分测试 |
| JSON 库兼容性 | 中 | 保留 fallback 机制 |

---

## 验收标准

### 功能验收

- [ ] 所有现有测试通过
- [ ] 新增基准测试覆盖关键路径
- [ ] 代码审查通过

### 性能验收

- [ ] 内存分配减少 50%+
- [ ] GC 暂停时间降低 40%+
- [ ] 吞吐量提升 50%+
- [ ] p99 延迟降低 30%+

### 代码质量验收

- [ ] 通过 `golangci-lint` 检查
- [ ] 测试覆盖率 > 80%
- [ ] 文档完整

---

## 参考资料

### 内部参考

- 代码分析文档：代码审查笔记
- 现有测试：`pkg/proxy/*_test.go`

### 外部参考

- Go sync.Pool 文档：https://pkg.go.dev/sync#Pool
- goproxy 文档：https://github.com/elazarl/goproxy
- Go 性能优化最佳实践：https://go.dev/doc/effective_go#performance

---

## 附录：优化前后对比

### Buffer Pooling 对比

**优化前**:
```go
body, err := io.ReadAll(req.Body)
req.GetBody = func() (io.ReadCloser, error) {
    return io.NopCloser(bytes.NewReader(body)), nil
}
```

**优化后**:
```go
buf := GetBuffer()
defer PutBuffer(buf)
_, err := io.Copy(buf, req.Body)
body := buf.Bytes()
```

### Header 查找对比

**优化前**:
```go
req.Header.Set(r.HeaderName, r.HeaderValue)
// 每次调用都进行 strings.ToLower + 字符串拼接
```

**优化后**:
```go
req.Header[r.headerKey] = []string{r.HeaderValue}
// 直接使用预计算的 key
```

---

**计划创建时间**: 2026-03-05  
**最后更新**: 2026-03-05  
**状态**: 待审批
