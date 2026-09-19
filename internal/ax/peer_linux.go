package ax

import (
	"net"
	"os"
	"syscall"
)

func sameUID(c *net.UnixConn) bool {
	r, e := c.SyscallConn()
	if e != nil {
		return false
	}
	ok := false
	e = r.Control(func(fd uintptr) {
		v, e := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		ok = e == nil && v.Uid == uint32(os.Getuid())
	})
	return e == nil && ok
}
