package proxy

import (
	"crypto/tls"
	"sync"
	"time"
)

// CertCache 缓存 MITM 证书（LRU + TTL）
type CertCache struct {
	mu      sync.RWMutex
	certs   map[string]*tls.Certificate
	expiry  map[string]time.Time
	ttl     time.Duration
	maxSize int
}

// NewCertCache 创建证书缓存
func NewCertCache(ttl time.Duration, maxSize int) *CertCache {
	return &CertCache{
		certs:   make(map[string]*tls.Certificate, maxSize/2), // 预分配
		expiry:  make(map[string]time.Time, maxSize/2),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get 从缓存获取证书
func (c *CertCache) Get(host string) (*tls.Certificate, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cert, exists := c.certs[host]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(c.expiry[host]) {
		return nil, false
	}

	return cert, true
}

// Set 设置证书到缓存
func (c *CertCache) Set(host string, cert *tls.Certificate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 限制缓存大小（LRU 淘汰）
	if len(c.certs) >= c.maxSize {
		c.evictOldest()
	}

	c.certs[host] = cert
	c.expiry[host] = time.Now().Add(c.ttl)
}

// evictOldest 淘汰最旧的证书（简单实现：随机淘汰 10%）
func (c *CertCache) evictOldest() {
	count := 0
	for host := range c.certs {
		delete(c.certs, host)
		delete(c.expiry, host)
		count++
		if count >= c.maxSize/10 {
			break
		}
	}
}
