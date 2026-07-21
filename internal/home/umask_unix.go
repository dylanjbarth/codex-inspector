//go:build unix

package home

import "syscall"

func syscallUmask(mask int) int { return syscall.Umask(mask) }
