package process

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/UserExistsError/conpty"
)

// startWithPty 使用 PTY 启动子进程 (Windows ConPTY)
func (l *ProcessLauncher) startWithPty() error {
	if l.Verbose {
		log.Printf("以 ConPTY 模式启动交互式进程: %s %v", l.Command, l.Args)
	}

	// 构造命令行字符串
	// 注意：Windows 命令行参数转义比较复杂，这里做简单处理
	commandLine := l.Command
	for _, arg := range l.Args {
		if strings.Contains(arg, " ") {
			commandLine += fmt.Sprintf(" \"%s\"", arg)
		} else {
			commandLine += " " + arg
		}
	}

	// ConPTY 选项
	opts := []conpty.ConPtyOption{
		conpty.ConPtyEnv(l.cmd.Env),
		conpty.ConPtyWorkDir(l.cmd.Dir),
	}

	cpty, err := conpty.Start(commandLine, opts...)
	if err != nil {
		return fmt.Errorf("启动 ConPTY 失败: %w", err)
	}
	l.conpty = cpty

	// 关联 l.cmd.Process 以便 Wait/Stop 能够某种程度感知 PID
	// 注意：l.cmd.Process 通常由 cmd.Start() 填充，这里我们手动填充
	if proc, err := os.FindProcess(cpty.Pid()); err == nil {
		l.cmd.Process = proc
	}

	// 双向复制流
	// ConPTY -> Stdout
	go func() {
		_, _ = io.Copy(l.Stdout, cpty)
	}()

	// Stdin -> ConPTY
	go func() {
		_, _ = io.Copy(cpty, l.Stdin)
	}()

	// 处理窗口大小 (如果有条件的话，conpty.Resize)
	// 目前 Windows 下获取终端大小变动比较复杂，暂时忽略，或者只初始化一次
	// 实际应用中可能需要监听 Windows 控制台事件

	log.Printf("子进程已在 ConPTY 启动 (PID: %d)", cpty.Pid())
	return nil
}

// Wait 等待子进程结束
func (l *ProcessLauncher) Wait() error {
	if l.conpty != nil {
		// 如果是 ConPTY 启动的
		cpty := l.conpty.(*conpty.ConPty)
		exitCode, err := cpty.Wait(context.Background())
		if l.Verbose {
			log.Printf("ConPTY 进程退出, Code: %d, Err: %v", exitCode, err)
		}
		cpty.Close()
		return err
	}

	// 普通模式
	if l.cmd == nil || l.cmd.Process == nil {
		return fmt.Errorf("进程未启动")
	}
	return l.cmd.Wait()
}

// Stop 停止子进程
func (l *ProcessLauncher) Stop() error {
	if l.conpty != nil {
		// Windows ConPTY 模式
		// 不直接调用 cpty.Close()，因为它会立即关闭管道导致子进程遇到 "read fatal" 错误
		// 而是通过杀掉进程，让 Wait() 检测到退出后进行清理
		if l.cmd.Process != nil {
			if l.Verbose {
				log.Printf("终止子进程 (PID: %d)", l.cmd.Process.Pid)
			}
			return l.cmd.Process.Kill()
		}
		return nil
	}

	// 普通模式
	if l.cmd == nil || l.cmd.Process == nil {
		return nil
	}

	if l.Verbose {
		log.Printf("终止子进程 (PID: %d)", l.cmd.Process.Pid)
	}

	if err := l.cmd.Process.Kill(); err != nil { // Windows通常直接Kill
		return fmt.Errorf("终止进程失败: %w", err)
	}

	return nil
}
