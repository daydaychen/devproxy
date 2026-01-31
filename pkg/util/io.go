package util

import (
	"os"
	"syscall"
)

// RedirectStderr 将标准错误 (FD 2) 重定向到指定文件
// 这在交互式应用中非常有用，可以确保任何漏网的日志（如第三方库直接写入 FD 2）都不会破坏终端 UI
func RedirectStderr(f *os.File) error {
	if f == nil {
		return nil
	}
	
	// syscall.Dup2 在 Unix 系统上将 FD 2 重定向到文件的 FD
	err := syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
	if err != nil {
		return err
	}
	
	// 同时更新 Go 的 os.Stderr 变量，确保一致性
	// 注意：这不会影响已经捕获了原始 FD 2 的对象，但 Dup2 已经从底层处理了
	os.Stderr = f
	
	return nil
}
