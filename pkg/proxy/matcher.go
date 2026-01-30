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
	return &StringMatcher{Pattern: pattern}
}

// Match 检查URL是否包含指定字符串
func (m *StringMatcher) Match(url string) bool {
	return strings.Contains(url, m.Pattern)
}
