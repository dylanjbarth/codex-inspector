//go:build unix

package process

import (
	"golang.org/x/sys/unix"
	"os"
)

type Lock struct{ f *os.File }

func Acquire(path string, nonblock bool) (*Lock, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if e != nil {
		return nil, e
	}
	flags := unix.LOCK_EX
	if nonblock {
		flags |= unix.LOCK_NB
	}
	if e = unix.Flock(int(f.Fd()), flags); e != nil {
		f.Close()
		return nil, e
	}
	return &Lock{f}, nil
}
func (l *Lock) Close() error { unix.Flock(int(l.f.Fd()), unix.LOCK_UN); return l.f.Close() }
