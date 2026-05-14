//go:build windows

package cli

import "syscall"

func detachedProcessAttributes() *syscall.SysProcAttr {
	return nil
}
