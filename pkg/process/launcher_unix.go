//go:build !windows

package process

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
)

// startWithPty 使用 PTY 启动子进程，以支持交互式应用和窗口调整
func (l *ProcessLauncher) startWithPty() error {
	if l.Verbose {
		log.Printf("以 PTY 模式启动交互式进程: %s %v", l.Command, l.Args)
	}

	var err error
	l.ptyFile, err = pty.Start(l.cmd)
	if err != nil {
		return fmt.Errorf("启动 PTY 失败: %w", err)
	}

	// 处理窗口大小调整
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, l.ptyFile); err != nil {
				log.Printf("调整 PTY 窗口大小失败: %v", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // 初始化窗口大小

	// 将 Stdin 复制到 PTY
	go func() {
		_, _ = io.Copy(l.ptyFile, os.Stdin)
	}()

	// 将 PTY 复制到 Stdout (这会经过我们配置的 MultiWriter，从而记录到日志并过滤 ANSI)
	go func() {
		_, _ = io.Copy(l.Stdout, l.ptyFile)
	}()

	log.Printf("子进程已在 PTY 启动 (PID: %d)", l.cmd.Process.Pid)
	return nil
}

// Wait 等待子进程结束
func (l *ProcessLauncher) Wait() error {
	if l.cmd == nil || l.cmd.Process == nil {
		return fmt.Errorf("进程未启动")
	}
	err := l.cmd.Wait()

	if l.ptyFile != nil {
		l.ptyFile.Close()
	}
	return err
}

// Stop 停止子进程
func (l *ProcessLauncher) Stop() error {
	if l.cmd == nil || l.cmd.Process == nil {
		return nil
	}

	if l.Verbose {
		log.Printf("终止子进程 (PID: %d)", l.cmd.Process.Pid)
	}

	if l.ptyFile != nil {
		l.ptyFile.Close()
	}

	// 尝试优雅地终止进程 (Unix 使用 SIGTERM)
	if err := l.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// 如果发送SIGTERM失败，强制杀死进程
		if err := l.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("终止进程失败: %w", err)
		}
	}

	return nil
}
