//go:build windows

package process

import "fmt"

// startWithPty Windows 下暂不支持 PTY，降级为普通启动（或报错提示）
// 注意：如果需要支持 Windows 交互，可能需要引入 conpty 相关库
func (l *ProcessLauncher) startWithPty() error {
	return fmt.Errorf("Windows temporary does not support PTY interactive mode, please use standard mode")
}
