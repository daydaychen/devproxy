//go:build windows

package util

import "fmt"

func dup(fd int) (int, error) {
	return 0, fmt.Errorf("dup not supported on windows")
}

func dup2(oldfd, newfd int) error {
	return fmt.Errorf("dup2 not supported on windows")
}
