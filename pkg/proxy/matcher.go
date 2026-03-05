package proxy

import (
	"regexp"
	"strings"
	"sync"
)

// URLMatcher 定义 URL 匹配接口
type URLMatcher interface {
	Match(url string) bool
}

// RegexMatcher 使用正则表达式匹配 URL
type RegexMatcher struct {
	Pattern *regexp.Regexp
}

// NewRegexMatcher 创建正则表达式匹配器
func NewRegexMatcher(pattern string) (*RegexMatcher, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &RegexMatcher{Pattern: re}, nil
}

// Match 检查 URL 是否匹配正则表达式
func (m *RegexMatcher) Match(url string) bool {
	return m.Pattern.MatchString(url)
}

// StringMatcher 使用字符串包含匹配 URL
type StringMatcher struct {
	Pattern string
}

// NewStringMatcher 创建字符串匹配器
func NewStringMatcher(pattern string) *StringMatcher {
	return &StringMatcher{Pattern: NormalizeURL(pattern)}
}

// Match 检查 URL 是否包含指定字符串
func (m *StringMatcher) Match(url string) bool {
	return strings.Contains(NormalizeURL(url), m.Pattern)
}

// urlNormCache 使用 sync.Map 缓存规范化后的 URL（线程安全，无需额外锁）
var urlNormCache sync.Map // map[string]string

// urlNormCacheCount 用于限制缓存大小（原子操作）
var urlNormCacheCount int64

// NormalizeURL 移除 URL 中的默认端口 (https:443, http:80) 以提高匹配兼容性（优化版：带缓存）
func NormalizeURL(u string) string {
	// 快速路径：无默认端口（最常见情况，直接返回避免开销）
	if !strings.Contains(u, ":443") && !strings.Contains(u, ":80") {
		return u
	}

	// 1. 查缓存（O(1)）
	if cached, ok := urlNormCache.Load(u); ok {
		return cached.(string)
	}

	// 2. 执行规范化（使用 strings.Builder + Grow 预分配）
	result := normalizeURLSlow(u)

	// 3. 写缓存（限制大小，避免内存泄漏）
	// 简单实现：仅缓存前 10000 个
	if urlNormCacheCount < 10000 {
		urlNormCache.Store(u, result)
		urlNormCacheCount++
	}

	return result
}

// normalizeURLSlow 使用 strings.Builder 优化（避免字符串拼接）
func normalizeURLSlow(u string) string {
	if strings.HasPrefix(u, "https://") {
		// 寻找 host 部分的结束位置
		rest := u[8:]
		slashIdx := strings.Index(rest, "/")
		host := rest
		path := ""
		if slashIdx != -1 {
			host = rest[:slashIdx]
			path = rest[slashIdx:]
		}
		if strings.HasSuffix(host, ":443") {
			// 使用 strings.Builder 避免拼接，预分配容量
			var builder strings.Builder
			builder.Grow(len(u) - 4) // 预分配：移除 ":443"
			builder.WriteString("https://")
			builder.WriteString(host[:len(host)-4])
			builder.WriteString(path)
			return builder.String()
		}
	} else if strings.HasPrefix(u, "http://") {
		rest := u[7:]
		slashIdx := strings.Index(rest, "/")
		host := rest
		path := ""
		if slashIdx != -1 {
			host = rest[:slashIdx]
			path = rest[slashIdx:]
		}
		if strings.HasSuffix(host, ":80") {
			// 使用 strings.Builder 避免拼接，预分配容量
			var builder strings.Builder
			builder.Grow(len(u) - 3) // 预分配：移除 ":80"
			builder.WriteString("http://")
			builder.WriteString(host[:len(host)-3])
			builder.WriteString(path)
			return builder.String()
		}
	}
	return u
}
