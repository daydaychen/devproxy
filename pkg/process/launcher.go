package process

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
)

// ProcessLauncher 子进程启动器
type ProcessLauncher struct {
	Command   string
	Args      []string
	ProxyPort int
	Verbose   bool
	cmd       *exec.Cmd
}

// NewProcessLauncher 创建新的进程启动器
func NewProcessLauncher(command string, args []string, proxyPort int, verbose bool) *ProcessLauncher {
	return &ProcessLauncher{
		Command:   command,
		Args:      args,
		ProxyPort: proxyPort,
		Verbose:   verbose,
	}
}

// Start 启动子进程
func (l *ProcessLauncher) Start() error {
	l.cmd = exec.Command(l.Command, l.Args...)

	// 继承当前环境变量，并添加代理配置
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", l.ProxyPort)
	l.cmd.Env = append(os.Environ(),
		fmt.Sprintf("HTTP_PROXY=%s", proxyURL),
		fmt.Sprintf("HTTPS_PROXY=%s", proxyURL),
		fmt.Sprintf("ALL_PROXY=%s", proxyURL),
		"NODE_TLS_REJECT_UNAUTHORIZED=0",
	)

	// 将子进程的输出重定向到当前进程
	l.cmd.Stdout = os.Stdout
	l.cmd.Stderr = os.Stderr
	l.cmd.Stdin = os.Stdin

	// 设置进程组，以便可以一起终止
	l.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if l.Verbose {
		log.Printf("启动进程: %s %v", l.Command, l.Args)
		log.Printf("环境变量: HTTP_PROXY=%s", proxyURL)
	}

	if err := l.cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败: %w", err)
	}

	log.Printf("子进程已启动 (PID: %d)", l.cmd.Process.Pid)
	return nil
}

// Wait 等待子进程结束
func (l *ProcessLauncher) Wait() error {
	if l.cmd == nil || l.cmd.Process == nil {
		return fmt.Errorf("进程未启动")
	}
	return l.cmd.Wait()
}

// Stop 停止子进程
func (l *ProcessLauncher) Stop() error {
	if l.cmd == nil || l.cmd.Process == nil {
		return nil
	}

	if l.Verbose {
		log.Printf("终止子进程 (PID: %d)", l.cmd.Process.Pid)
	}

	// 尝试优雅地终止进程组
	pgid, err := syscall.Getpgid(l.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	}

	// 如果优雅终止失败，强制杀死进程
	if err := l.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("终止进程失败: %w", err)
	}

	return nil
}
