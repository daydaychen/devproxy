//go:build linux && arm64

package util

import "syscall"

func dup(fd int) (int, error) {
	return syscall.Dup(fd)
}

func dup2(oldfd, newfd int) error {
	return syscall.Dup3(oldfd, newfd, 0)
}
