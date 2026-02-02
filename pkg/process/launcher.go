package process

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"


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
	conpty    interface{} // Windows ConPTY instance
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
