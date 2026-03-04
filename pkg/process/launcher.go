package process

import (
	"fmt"
	"io"
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
	Stdout    io.Writer
	Stderr    io.Writer
	Stdin     io.Reader
	cmd       *exec.Cmd
	cleanupCA func()
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

	// 导出代理证书以便注入到子进程
	caPath, cleanup, err := ExportProxyCA()
	if err != nil {
		if l.Verbose {
			log.Printf("警告: 导出代理CA证书失败: %v, TLS连通性可能受影响", err)
		}
	} else {
		l.cleanupCA = cleanup
		if l.Verbose {
			log.Printf("代理CA证书已导出至: %s", caPath)
		}
	}

	// 继承当前环境变量，并添加代理配置
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", l.ProxyPort)
	envVars := []string{
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
		"BUN_INSTALL_SKIP_TLS_CHECK=1",
		"BUN_CONFIG_SKIP_TLS_VERIFY=true",
	}

	// 如果证书成功导出，则注入证书路径
	// 用于支持严格验证证书的二进制或语言（Rust, Python, Node 纯净版等）
	if caPath != "" {
		envVars = append(envVars,
			fmt.Sprintf("SSL_CERT_FILE=%s", caPath),          // Rust / Go / curl 等通用标准
			fmt.Sprintf("REQUESTS_CA_BUNDLE=%s", caPath),     // Python requests 库
			fmt.Sprintf("NODE_EXTRA_CA_CERTS=%s", caPath),    // Node.js 原生支持
			fmt.Sprintf("CURL_CA_BUNDLE=%s", caPath),         // CURL
		)
	} else {
		envVars = append(envVars, "NODE_EXTRA_CA_CERTS=")
	}

	l.cmd.Env = append(os.Environ(), envVars...)

	// 直接绑定标准输入输出
	// Go 的 exec.Cmd 会正确处理这些流，即使是 TTY
	l.cmd.Stdout = l.Stdout
	l.cmd.Stderr = l.Stderr
	l.cmd.Stdin = l.Stdin

	if l.Verbose {
		log.Printf("启动子进程: %s %v", l.Command, l.Args)
	}

	if err := l.cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败: %w", err)
	}

	log.Printf("子进程已启动 (PID: %d)", l.cmd.Process.Pid)
	return nil
}

// Wait 等待子进程结束
func (l *ProcessLauncher) Wait() error {
	defer l.cleanTempFiles()
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

	// 尝试优雅地终止进程
	// 注意：Windows 上没有 SIGTERM，Signal 会被转为 Kill
	if err := l.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// 如果发送SIGTERM失败，强制杀死进程
		if err := l.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("终止进程失败: %w", err)
		}
	}

	l.cleanTempFiles()

	return nil
}

func (l *ProcessLauncher) cleanTempFiles() {
	if l.cleanupCA != nil {
		l.cleanupCA()
		l.cleanupCA = nil
	}
}
