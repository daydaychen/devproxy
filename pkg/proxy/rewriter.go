package proxy

import "net/http"

// HeaderRewriter 定义请求头重写器
type HeaderRewriter struct {
	HeaderName  string
	HeaderValue string
}

// NewHeaderRewriter 创建请求头重写器
func NewHeaderRewriter(name, value string) *HeaderRewriter {
	return &HeaderRewriter{
		HeaderName:  name,
		HeaderValue: value,
	}
}

// Rewrite 重写HTTP请求头
func (r *HeaderRewriter) Rewrite(req *http.Request) {
	req.Header.Set(r.HeaderName, r.HeaderValue)
}
