package ax

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

func sameUID(c *net.UnixConn) bool {
	raw, e := c.SyscallConn()
	if e != nil {
		return false
	}
	ok := false
	e = raw.Control(func(fd uintptr) {
		var cred struct {
			Version uint32
			UID     uint32
			NGroups int16
			_       int16
			Groups  [16]uint32
		}
		size := uint32(unsafe.Sizeof(cred))
		_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, 0, 1, uintptr(unsafe.Pointer(&cred)), uintptr(unsafe.Pointer(&size)), 0)
		ok = errno == 0 && cred.UID == uint32(os.Getuid())
	})
	return e == nil && ok
}
