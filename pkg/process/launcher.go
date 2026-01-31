package process

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// ProcessLauncher 子进程启动器
type ProcessLauncher struct {
	Command   string
	Args      []string
	ProxyPort int
	Verbose   bool
	Stdout    io.Writer
	Stderr    io.Writer
	Stdin     io.Reader
	cmd       *exec.Cmd
	ptyFile   *os.File
}

// NewProcessLauncher 创建新的进程启动器
func NewProcessLauncher(command string, args []string, proxyPort int, verbose bool) *ProcessLauncher {
	return &ProcessLauncher{
		Command:   command,
		Args:      args,
		ProxyPort: proxyPort,
		Verbose:   verbose,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Stdin:     os.Stdin,
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
		fmt.Sprintf("http_proxy=%s", proxyURL),
		fmt.Sprintf("https_proxy=%s", proxyURL),
		fmt.Sprintf("all_proxy=%s", proxyURL),
		"NODE_TLS_REJECT_UNAUTHORIZED=0",
		"BUN_TLS_REJECT_UNAUTHORIZED=0",
		"npm_config_strict_ssl=false",
		"yarn_strict_ssl=false",
		"STRICT_SSL=false",
		"NODE_EXTRA_CA_CERTS=",
		"BUN_INSTALL_SKIP_TLS_CHECK=1",
		"BUN_CONFIG_SKIP_TLS_VERIFY=true",
	)

	// 检查是否为交互式模式 (Stdin 是否是 TTY)
	if f, ok := l.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return l.startWithPty()
	}

	// 非交互式模式，使用普通方式
	l.cmd.Stdout = l.Stdout
	l.cmd.Stderr = l.Stderr
	l.cmd.Stdin = l.Stdin

	if l.Verbose {
		log.Printf("以普通模式启动进程: %s %v", l.Command, l.Args)
	}

	if err := l.cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败: %w", err)
	}

	log.Printf("子进程已启动 (PID: %d)", l.cmd.Process.Pid)
	return nil
}

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

	// 尝试优雅地终止进程
	if err := l.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// 如果发送SIGTERM失败，强制杀死进程
		if err := l.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("终止进程失败: %w", err)
		}
	}

	return nil
}
