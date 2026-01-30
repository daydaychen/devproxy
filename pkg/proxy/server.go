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
	proxy         *goproxy.ProxyHttpServer
	server        *http.Server
	defaultRule   *ProxyRule
}

// NewProxyServer 创建新的代理服务器
func NewProxyServer(port int, upstream string, verbose bool) *ProxyServer {
	defaultRule := &ProxyRule{Name: "default"}
	return &ProxyServer{
		Port:          port,
		UpstreamProxy: upstream,
		Verbose:       verbose,
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
	s.proxy.Verbose = s.Verbose

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
	s.proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)

	// 设置请求拦截和修改
	s.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// 构建完整的 URL（包括 Host）
		reqURL := req.URL.String()
		if req.URL.Host == "" && req.Host != "" {
			// 对于某些请求，URL.Host 可能为空，使用 req.Host
			scheme := "http"
			if req.TLS != nil {
				scheme = "https"
			}
			reqURL = fmt.Sprintf("%s://%s%s", scheme, req.Host, req.URL.Path)
			if req.URL.RawQuery != "" {
				reqURL += "?" + req.URL.RawQuery
			}
		}

		if s.Verbose {
			log.Printf("[REQUEST] %s %s", req.Method, reqURL)
		}

		// 遍历所有规则组
		for _, rule := range s.Rules {
			matched := false
			if len(rule.Matchers) == 0 {
				// 如果该规则没有配置匹配器，且不是默认规则（或者默认规则有重写内容），则匹配所有请求
				// 注意：这里为了灵活，空匹配器视为匹配所有
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

			// 如果该组规则匹配，则应用该组的重写规则
			if matched {
				if s.Verbose {
					log.Printf("[RULE:%s MATCHED] %s %s", rule.Name, req.Method, reqURL)
				}
				for _, rewriter := range rule.Rewriters {
					rewriter.Rewrite(req)
					if s.Verbose {
						log.Printf("[RULE:%s REWRITE] %s: %s", rule.Name, rewriter.HeaderName, rewriter.HeaderValue)
					}
				}
			}
		}

		return req, nil
	})

	// 启动HTTP服务器
	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.Port),
		Handler: s.proxy,
	}

	log.Printf("代理服务器启动在 http://127.0.0.1:%d", s.Port)
	if s.UpstreamProxy != "" {
		log.Printf("上游代理: %s", s.UpstreamProxy)
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("代理服务器错误: %v", err)
		}
	}()

	// 等待一小段时间确保服务器启动
	time.Sleep(100 * time.Millisecond)
	return nil
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
