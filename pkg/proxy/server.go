package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/elazarl/goproxy"
)

// ProxyRule 定义一组匹配规则及其对应的重写动作
type ProxyRule struct {
	Name            string
	Matchers        []URLMatcher
	Rewriters       []*HeaderRewriter
	Plugins         []RequestPlugin
	ResponsePlugins []ResponsePlugin
}

// ProxyServer MITM代理服务器
type ProxyServer struct {
	Port          int
	UpstreamProxy string
	Rules         []*ProxyRule
	Verbose       bool
	DumpTraffic   bool
	Logger        *log.Logger
	proxy         *goproxy.ProxyHttpServer
	server        *http.Server
	defaultRule   *ProxyRule
}

// NewProxyServer 创建新的代理服务器
func NewProxyServer(port int, upstream string, verbose bool, logger *log.Logger) *ProxyServer {
	if logger == nil {
		logger = log.Default()
	}
	defaultRule := &ProxyRule{Name: "default"}
	// 默认禁用 Accept-Encoding 以确保插件能处理明文响应
	defaultRule.Rewriters = append(defaultRule.Rewriters, NewHeaderRewriter("Accept-Encoding", "identity"))
	return &ProxyServer{
		Port:          port,
		UpstreamProxy: upstream,
		Verbose:       verbose,
		Logger:        logger,
		Rules:         []*ProxyRule{defaultRule},
		defaultRule:   defaultRule,
	}
}

// AddRule 添加一个新的规则组
func (s *ProxyServer) AddRule(rule *ProxyRule) {
	s.Rules = append(s.Rules, rule)
}

// AddMatcher 添加全局URL匹配器 (添加到默认规则)
func (s *ProxyServer) AddMatcher(matcher URLMatcher) {
	s.defaultRule.Matchers = append(s.defaultRule.Matchers, matcher)
}

// AddRewriter 添加请求头重写器 (添加到默认规则)
func (s *ProxyServer) AddRewriter(rewriter *HeaderRewriter) {
	s.defaultRule.Rewriters = append(s.defaultRule.Rewriters, rewriter)
}

// AddPlugin 添加请求插件 (添加到默认规则)
func (s *ProxyServer) AddPlugin(plugin RequestPlugin) {
	s.defaultRule.Plugins = append(s.defaultRule.Plugins, plugin)
}

// AddResponsePlugin 添加响应插件 (添加到默认规则)
func (s *ProxyServer) AddResponsePlugin(plugin ResponsePlugin) {
	s.defaultRule.ResponsePlugins = append(s.defaultRule.ResponsePlugins, plugin)
}

// Start 启动代理服务器
func (s *ProxyServer) Start() error {
	s.proxy = goproxy.NewProxyHttpServer()
	// 重定向 goproxy 的日志到我们的 Logger
	s.proxy.Logger = s.Logger
	// 禁用 goproxy 内部过度详细且难以阅读的日志
	s.proxy.Verbose = false

	// 设置上游代理
	if s.UpstreamProxy != "" {
		upstreamURL, err := url.Parse(s.UpstreamProxy)
		if err != nil {
			return fmt.Errorf("无效的上游代理地址: %w", err)
		}
		s.proxy.Tr.Proxy = http.ProxyURL(upstreamURL)
		s.proxy.ConnectDial = s.proxy.NewConnectDialToProxy(s.UpstreamProxy)
	}

	// 设置HTTPS MITM
	s.proxy.OnRequest().HandleConnectFunc(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		// 只有当域名匹配了某些规则时，才进行 MITM (解密)
		// 这样可以避免对未匹配域名 (如 bun.report) 产生不必要的证书泄露和验证错误
		if s.ShouldMITM(host) {
			if s.Verbose {
				s.Logger.Printf("[CONNECT:MITM] %s", host)
			}
			return goproxy.MitmConnect, host
		}

		if s.Verbose {
			s.Logger.Printf("[CONNECT:PASSTHROUGH] %s", host)
		}
		// 否则仅作为普通代理转发，不解密流量，保留原始证书
		return goproxy.OkConnect, host
	})

	// 设置请求拦截和修改
	s.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqURL := req.URL.String()
		if req.URL.Host == "" && req.Host != "" {
			scheme := "http"
			if req.TLS != nil {
				scheme = "https"
			}
			reqURL = fmt.Sprintf("%s://%s%s", scheme, req.Host, req.URL.Path)
		}

		// 1. 标准化 URL 用于匹配
		matchURL := NormalizeURL(reqURL)

		// 2. 预判是否需要读取/缓存 Body
		// 严重警告：为了保护 Passthrough (直连) 请求不被中断（特别是流式请求），
		// 我们【仅】在匹配了带有插件且需要修改 Body 的规则时才读取 Body。
		var matchedRule *ProxyRule
		for _, rule := range s.Rules {
			matched := false
			if len(rule.Matchers) == 0 {
				if len(rule.Rewriters) > 0 || len(rule.Plugins) > 0 {
					matched = true
				}
			} else {
				for _, matcher := range rule.Matchers {
					if matcher.Match(matchURL) {
						matched = true
						break
					}
				}
			}
			if matched {
				matchedRule = rule
				break
			}
		}

		needBody := false
		if matchedRule != nil && len(matchedRule.Plugins) > 0 {
			needBody = true
		}

		// 3. 按需读取并恢复 Body (仅为插件服务)
		if needBody && req.Body != nil && req.Body != http.NoBody {
			body, err := io.ReadAll(req.Body)
			if err == nil {
				req.GetBody = func() (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(body)), nil
				}
				req.Body, _ = req.GetBody()
				
				// 关键点：同步状态，但减少人工 Header 干预，让 net/http 标准库处理
				req.ContentLength = int64(len(body))
				req.Header.Del("Transfer-Encoding")
				req.Header.Del("Expect") 
			}
		}

		// 4. 日志记录
		if s.DumpTraffic && req.Method != http.MethodConnect {
			dump, err := httputil.DumpRequest(req, needBody)
			if err == nil {
				s.Logger.Printf("[REQUEST DUMP] %s %s\n%s", req.Method, reqURL, string(dump))
			} else {
				s.Logger.Printf("[REQUEST DUMP ERROR] %s %s: %v", req.Method, reqURL, err)
			}
		} else if s.Verbose {
			s.Logger.Printf("[REQUEST] %s %s (Headers: %d, Length: %d)", req.Method, reqURL, len(req.Header), req.ContentLength)
		}

		// 5. 执行规则逻辑
		if matchedRule != nil {
			if s.Verbose {
				s.Logger.Printf("[RULE:%s MATCHED] %s", matchedRule.Name, matchURL)
			}

			// A. 执行插件 (插件内部如需修改 Body，其自身应负责同步 req.ContentLength)
			for _, plugin := range matchedRule.Plugins {
				err := plugin.ProcessRequest(req)
				if s.Verbose {
					if err != nil {
						s.Logger.Printf("[RULE:%s PLUGIN ERROR] %s: %v", matchedRule.Name, plugin.Name(), err)
					} else {
						s.Logger.Printf("[RULE:%s PLUGIN APPLY] %s applied on %s", matchedRule.Name, plugin.Name(), matchURL)
					}
				}
			}

			// B. 执行重写逻辑
			for _, rewriter := range matchedRule.Rewriters {
				rewriter.Rewrite(req)
				if s.Verbose {
					s.Logger.Printf("[RULE:%s REWRITE] %s -> %s", matchedRule.Name, rewriter.HeaderName, rewriter.HeaderValue)
				}
			}
		}

		return req, nil
	})

	// 设置响应拦截和日志
	s.proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		reqURL := ctx.Req.URL.String()
		if resp != nil {
			// 1. 标准化 URL 用于匹配
			matchURL := NormalizeURL(reqURL)

			// 2. 匹配规则
			var matchedRule *ProxyRule
			for _, rule := range s.Rules {
				matched := false
				if len(rule.Matchers) == 0 {
					if len(rule.ResponsePlugins) > 0 {
						matched = true
					}
				} else {
					for _, matcher := range rule.Matchers {
						if matcher.Match(matchURL) {
							matched = true
							break
						}
					}
				}
				if matched {
					matchedRule = rule
					break
				}
			}

			// 3. 执行响应插件
			if matchedRule != nil && len(matchedRule.ResponsePlugins) > 0 {
				for _, plugin := range matchedRule.ResponsePlugins {
					err := plugin.ProcessResponse(resp, ctx, s.Verbose)
					if err != nil {
						s.Logger.Printf("[RULE:%s RESPONSE PLUGIN ERROR] %s: %v", matchedRule.Name, plugin.Name(), err)
					} else if s.Verbose {
						s.Logger.Printf("[RULE:%s RESPONSE PLUGIN APPLY] %s applied on %s", matchedRule.Name, plugin.Name(), matchURL)
					}
				}
			}

			// 重点排查 401 错误的具体负载
			if resp.StatusCode == http.StatusUnauthorized {
				dump, err := httputil.DumpResponse(resp, true)
				if err == nil {
					s.Logger.Printf("[DIAGNOSTIC] 401 UNAUTHORIZED DETAIL (%s):\n%s", reqURL, string(dump))
				}
			}

			if s.DumpTraffic && ctx.Req.Method != http.MethodConnect {
				// 跳过流式响应的 dump，避免消耗 io.Pipe body
				contentType := resp.Header.Get("Content-Type")
				if strings.Contains(contentType, "text/event-stream") {
					s.Logger.Printf("[RESPONSE STREAM] %s %s -> %s (流式响应, 跳过 dump)", ctx.Req.Method, reqURL, resp.Status)
				} else {
					dump, err := httputil.DumpResponse(resp, true)
					if err == nil {
						s.Logger.Printf("[RESPONSE DUMP] %s %s -> %s\n%s", ctx.Req.Method, reqURL, resp.Status, string(dump))
					} else {
						s.Logger.Printf("[RESPONSE DUMP ERROR] %s %s: %v", ctx.Req.Method, reqURL, err)
					}
				}
			} else if s.Verbose {
				s.Logger.Printf("[RESPONSE] %s %s -> %s", ctx.Req.Method, reqURL, resp.Status)
			}
		} else {
			if s.Verbose {
				s.Logger.Printf("[RESPONSE ERROR] %s %s: 响应为空", ctx.Req.Method, reqURL)
			}
		}
		return resp
	})

	// 启动HTTP服务器
	s.server = &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", s.Port),
		Handler:           s.proxy,
		ErrorLog:          s.Logger,
		ReadHeaderTimeout: 10 * time.Second, // 限制读取头部超时
		IdleTimeout:       30 * time.Second, // 限制空闲连接超时
	}

	s.Logger.Printf("代理服务器启动在 http://127.0.0.1:%d", s.Port)
	if s.UpstreamProxy != "" {
		s.Logger.Printf("上游代理: %s", s.UpstreamProxy)
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Logger.Printf("代理服务器错误: %v", err)
		}
	}()

	// 等待一小段时间确保服务器启动
	time.Sleep(100 * time.Millisecond)
	return nil
}

// ShouldMITM 判断是否需要对该 host 进行 MITM
func (s *ProxyServer) ShouldMITM(host string) bool {
	// 剥离端口号 (例如从 "example.com:443" 得到 "example.com")
	domain := host
	if pos := strings.Index(host, ":"); pos != -1 {
		domain = host[:pos]
	}

	for _, rule := range s.Rules {
		// 如果规则没有 Matcher 但有 Rewriter 或 Plugin，说明是全局规则，必须 MITM
		if len(rule.Matchers) == 0 && (len(rule.Rewriters) > 0 || len(rule.Plugins) > 0) {
			return true
		}

		for _, matcher := range rule.Matchers {
			// 如果是正则表达式匹配，由于无法预测，默认开启 MITM 以确保规则生效
			if _, ok := matcher.(*RegexMatcher); ok {
				return true
			}

			// 如果是字符串匹配
			if sm, ok := matcher.(*StringMatcher); ok {
				pattern := sm.Pattern
				// 情况 1: 模式中直接包含当前域名 (如匹配 "google.com/api" 且 host 是 "google.com")
				if strings.Contains(pattern, domain) {
					return true
				}
				// 情况 2: 当前域名包含模式 (如匹配 "google" 且 host 是 "google.com")
				if strings.Contains(domain, pattern) {
					return true
				}
				// 情况 3: 模式看起来像是一个全局路径匹配 (如以 "/" 开头，或者是特定的文件名如 "api.json")
				// 这种情况下我们无法在未解密前确定路径，所以必须解密。
				if strings.HasPrefix(pattern, "/") || (!strings.Contains(pattern, ".") && strings.Contains(pattern, "/")) {
					return true
				}
				
				// 情况 4: 针对用户常见的特殊文件名匹配 (如 "api.json")
				if !strings.Contains(pattern, "/") && strings.Contains(pattern, ".") && !strings.Contains(pattern, domain) {
					// 如果模式带点但不是当前域名，它可能是一个路径后缀，保险起见开启 MITM
					return true
				}
			}
		}
	}
	return false
}

// Stop 停止代理服务器
func (s *ProxyServer) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}
