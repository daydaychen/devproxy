package proxy

import (
	"regexp"
	"strings"
)

// URLMatcher 定义URL匹配接口
type URLMatcher interface {
	Match(url string) bool
}

// RegexMatcher 使用正则表达式匹配URL
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

// Match 检查URL是否匹配正则表达式
func (m *RegexMatcher) Match(url string) bool {
	return m.Pattern.MatchString(url)
}

// StringMatcher 使用字符串包含匹配URL
type StringMatcher struct {
	Pattern string
}

// NewStringMatcher 创建字符串匹配器
func NewStringMatcher(pattern string) *StringMatcher {
	return &StringMatcher{Pattern: NormalizeURL(pattern)}
}

// Match 检查URL是否包含指定字符串
func (m *StringMatcher) Match(url string) bool {
	return strings.Contains(NormalizeURL(url), m.Pattern)
}

// NormalizeURL 移除 URL 中的默认端口 (https:443, http:80) 以提高匹配兼容性。
func NormalizeURL(u string) string {
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
			return "https://" + strings.TrimSuffix(host, ":443") + path
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
			return "http://" + strings.TrimSuffix(host, ":80") + path
		}
	}
	return u
}
