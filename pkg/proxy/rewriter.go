package proxy

import "net/http"

// HeaderRewriter 定义请求头重写器（优化版：预计算 key 和 value）
type HeaderRewriter struct {
	HeaderName       string
	HeaderValue      string
	headerKey        string   // 缓存标准化后的 key，避免重复计算
	headerValueBytes []byte   // 预转换为 []byte 避免重复转换
}

// NewHeaderRewriter 创建请求头重写器（优化版）
func NewHeaderRewriter(name, value string) *HeaderRewriter {
	return &HeaderRewriter{
		HeaderName:       name,
		HeaderValue:      value,
		headerKey:        http.CanonicalHeaderKey(name), // 预计算标准化 key
		headerValueBytes: []byte(value),                 // 预转换避免运行时转换
	}
}

// Rewrite 重写 HTTP 请求头（优化版：直接使用预计算的 key 和 []byte）
func (r *HeaderRewriter) Rewrite(req *http.Request) {
	// 直接使用预计算的 key，避免重复的 http.CanonicalHeaderKey 调用
	// 使用 Set 方法保持兼容性
	req.Header.Set(r.headerKey, string(r.headerValueBytes))
}
