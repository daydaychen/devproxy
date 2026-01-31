package util

import (
	"os"
	"syscall"
)

// HijackStandardStreams 将标准输出和标准错误重定向到指定文件，并返回原始的输出流
// 这可以彻底隔离代理服务器产生的任何杂讯日志，同时保留原始终端流用于渲染子进程 UI
func HijackStandardStreams(f *os.File) (origStdout *os.File, origStderr *os.File, err error) {
	if f == nil {
		return os.Stdout, os.Stderr, nil
	}

	// 1. 备份原始的 FD 1 和 FD 2
	stdoutFd, err := syscall.Dup(int(os.Stdout.Fd()))
	if err != nil {
		return nil, nil, err
	}
	stderrFd, err := syscall.Dup(int(os.Stderr.Fd()))
	if err != nil {
		return nil, nil, err
	}

	origStdout = os.NewFile(uintptr(stdoutFd), "/dev/stdout")
	origStderr = os.NewFile(uintptr(stderrFd), "/dev/stderr")

	// 2. 将 FD 1 和 FD 2 物理重定向到日志文件
	// 这样无论程序中哪里调用 fmt.Printf, log.Printf 甚至底层库直接写 FD 1/2，都会进文件
	if err := syscall.Dup2(int(f.Fd()), int(os.Stdout.Fd())); err != nil {
		return nil, nil, err
	}
	if err := syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd())); err != nil {
		return nil, nil, err
	}

	// 3. 同步更新 Go 的全局变量
	os.Stdout = f
	os.Stderr = f

	return origStdout, origStderr, nil
}

// RedirectStderr 保留旧版 API 以防兼容问题，但建议使用 HijackStandardStreams
func RedirectStderr(f *os.File) error {
	if f == nil {
		return nil
	}
	err := syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
	if err != nil {
		return err
	}
	os.Stderr = f
	return nil
}
