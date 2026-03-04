package process

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elazarl/goproxy"
)

// ExportProxyCA 导出代理服务器使用的默认 CA 证书到一个临时文件。
// 返回临时文件的绝对路径以及一个用于清理该文件的函数。
func ExportProxyCA() (string, func(), error) {
	// 创建一个临时文件来存储 goproxy 默认的根证书 (PEM格式)
	certFile, err := os.CreateTemp("", "devproxy-ca-*.pem")
	if err != nil {
		return "", func() {}, fmt.Errorf("创建证书临时文件失败: %w", err)
	}

	path := certFile.Name()

	// 写入 goproxy 的系统内置 CA
	if _, err := certFile.Write(goproxy.CA_CERT); err != nil {
		certFile.Close()
		os.Remove(path)
		return "", func() {}, fmt.Errorf("写入 CA 证书失败: %w", err)
	}

	certFile.Close()

	// 返回清理函数
	cleanup := func() {
		os.Remove(path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return path, cleanup, nil
	}
	return absPath, cleanup, nil
}
