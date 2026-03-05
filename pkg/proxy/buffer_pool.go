package proxy

import (
	"bytes"
	"sync"
)

// bufferPool 实现 Go 官方 fmt 包模式的 buffer 池
type bufferPool struct {
	pool *sync.Pool
}

// newBufferPool 创建新的 buffer 池
func newBufferPool() *bufferPool {
	return &bufferPool{
		pool: &sync.Pool{
			New: func() interface{} {
				// 返回指针类型，避免 interface 包装分配
				// 预分配 4KB 初始容量，适合大多数请求
				return bytes.NewBuffer(make([]byte, 0, 4096))
			},
		},
	}
}

// Get 从池中获取 buffer
func (bp *bufferPool) Get() *bytes.Buffer {
	return bp.pool.Get().(*bytes.Buffer)
}

// Put 归还 buffer 到池中（必须 Reset 状态）
func (bp *bufferPool) Put(buf *bytes.Buffer) {
	buf.Reset() // 关键：重置状态
	bp.pool.Put(buf)
}

// 全局 buffer pool（包级变量，禁止复制）
var BufferPool = newBufferPool()

// GetBuffer 从全局池获取 buffer（便捷函数）
func GetBuffer() *bytes.Buffer {
	return BufferPool.Get()
}

// PutBuffer 归还 buffer 到全局池（便捷函数）
func PutBuffer(buf *bytes.Buffer) {
	BufferPool.Put(buf)
}
