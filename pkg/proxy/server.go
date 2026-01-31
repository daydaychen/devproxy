package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/elazarl/goproxy"
)

// ProxyRule 定义一组匹配规则及其对应的重写动作
type ProxyRule struct {
	Name      string
	Matchers  []URLMatcher
	Rewriters []*HeaderRewriter
}

// ProxyServer MITM代理服务器
type ProxyServer struct {
	Port          int
	UpstreamProxy string
	Rules         []*ProxyRule
	Verbose       bool
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

// Start 启动代理服务器
func (s *ProxyServer) Start() error {
	s.proxy = goproxy.NewProxyHttpServer()
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

		if s.Verbose {
			s.Logger.Printf("[REQUEST] %s %s", req.Method, reqURL)
		}

		for _, rule := range s.Rules {
			matched := false
			if len(rule.Matchers) == 0 {
				if len(rule.Rewriters) > 0 {
					matched = true
				}
			} else {
				for _, matcher := range rule.Matchers {
					if matcher.Match(reqURL) {
						matched = true
						break
					}
				}
			}

			if matched {
				if s.Verbose {
					s.Logger.Printf("[RULE:%s MATCHED] %s", rule.Name, reqURL)
				}
				for _, rewriter := range rule.Rewriters {
					rewriter.Rewrite(req)
					if s.Verbose {
						s.Logger.Printf("[RULE:%s REWRITE] %s -> %s", rule.Name, rewriter.HeaderName, rewriter.HeaderValue)
					}
				}
			}
		}

		return req, nil
	})

	// 设置响应拦截和日志
	s.proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if s.Verbose {
			reqURL := ctx.Req.URL.String()
			if resp != nil {
				s.Logger.Printf("[RESPONSE] %s %s -> %s", ctx.Req.Method, reqURL, resp.Status)
			} else {
				s.Logger.Printf("[RESPONSE ERROR] %s %s: 响应为空", ctx.Req.Method, reqURL)
			}
		}
		return resp
	})

	// 启动HTTP服务器
	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.Port),
		Handler: s.proxy,
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
	// 目前判断逻辑：只要有任意一条规则的任意一个 Matcher 匹配该 host
	// 注意：Matcher 是基于完整 URL 的，这里我们构造一个简单的 URL 进行匹配测试
	testURL := fmt.Sprintf("https://%s/", host)

	for _, rule := range s.Rules {
		// 如果规则没有 Matcher 但有 Rewriter，说明是全局规则，应该 MITM
		if len(rule.Matchers) == 0 && len(rule.Rewriters) > 0 {
			return true
		}
		for _, matcher := range rule.Matchers {
			if matcher.Match(testURL) {
				return true
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
