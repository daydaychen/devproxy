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

// ProxyServer MITM代理服务器
type ProxyServer struct {
	Port          int
	UpstreamProxy string
	Matchers      []URLMatcher
	Rewriters     []*HeaderRewriter
	Verbose       bool
	proxy         *goproxy.ProxyHttpServer
	server        *http.Server
}

// NewProxyServer 创建新的代理服务器
func NewProxyServer(port int, upstream string, verbose bool) *ProxyServer {
	return &ProxyServer{
		Port:          port,
		UpstreamProxy: upstream,
		Verbose:       verbose,
		Matchers:      []URLMatcher{},
		Rewriters:     []*HeaderRewriter{},
	}
}

// AddMatcher 添加URL匹配器
func (s *ProxyServer) AddMatcher(matcher URLMatcher) {
	s.Matchers = append(s.Matchers, matcher)
}

// AddRewriter 添加请求头重写器
func (s *ProxyServer) AddRewriter(rewriter *HeaderRewriter) {
	s.Rewriters = append(s.Rewriters, rewriter)
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

		// 检查是否匹配任何规则
		matched := false
		if len(s.Matchers) == 0 {
			// 如果没有配置匹配器，则匹配所有请求
			matched = true
		} else {
			for _, matcher := range s.Matchers {
				if matcher.Match(reqURL) {
					matched = true
					break
				}
			}
		}

		// 如果匹配，则应用重写规则
		if matched {
			if s.Verbose {
				log.Printf("[MATCHED] %s %s", req.Method, reqURL)
			}
			for _, rewriter := range s.Rewriters {
				rewriter.Rewrite(req)
				if s.Verbose {
					log.Printf("[REWRITE] %s: %s", rewriter.HeaderName, rewriter.HeaderValue)
				}
			}
		} else if s.Verbose {
			log.Printf("[SKIP] %s %s", req.Method, reqURL)
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
